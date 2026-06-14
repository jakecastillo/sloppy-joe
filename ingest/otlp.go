package ingest

import (
	"encoding/json"
	"io"
	"net/http"
	"slices"
	"strconv"
	"strings"
	"time"
)

// Minimal OTLP/JSON metrics shapes — enough to extract gen_ai token usage.
type otlpMetricsDoc struct {
	ResourceMetrics []struct {
		ScopeMetrics []struct {
			Metrics []otlpMetric `json:"metrics"`
		} `json:"scopeMetrics"`
	} `json:"resourceMetrics"`
}

type otlpMetric struct {
	Name  string       `json:"name"`
	Sum   *otlpDataSet `json:"sum"`
	Gauge *otlpDataSet `json:"gauge"`
}

type otlpDataSet struct {
	DataPoints []otlpDataPoint `json:"dataPoints"`
}

type otlpDataPoint struct {
	AsInt      string   `json:"asInt"` // OTLP/JSON encodes int64 as a string
	AsDouble   float64  `json:"asDouble"`
	Attributes []otlpKV `json:"attributes"`
}

type otlpKV struct {
	Key   string `json:"key"`
	Value struct {
		StringValue string `json:"stringValue"`
	} `json:"value"`
}

type usageEvent struct {
	tenant string
	model  string
	input  int
	output int
}

// parseOTLPUsage extracts token-usage events from an OTLP/JSON metrics payload.
// Recognizes any metric whose name contains "token", with data-point attributes
// gen_ai.token.type (input|output), tenant, and gen_ai.request.model (or model).
func parseOTLPUsage(body []byte) ([]usageEvent, error) {
	var doc otlpMetricsDoc
	if err := json.Unmarshal(body, &doc); err != nil {
		return nil, err
	}
	var events []usageEvent
	// One attrs map reused across every datapoint: cleared per iteration instead
	// of allocating a fresh map in the innermost loop.
	attrs := make(map[string]string)
	for _, rm := range doc.ResourceMetrics {
		for _, sm := range rm.ScopeMetrics {
			for _, m := range sm.Metrics {
				if !strings.Contains(m.Name, "token") {
					continue
				}
				for _, ds := range []*otlpDataSet{m.Sum, m.Gauge} {
					if ds == nil {
						continue
					}
					// Presize events from this set's datapoint count so the
					// append loop below grows the backing array at most once.
					events = slices.Grow(events, len(ds.DataPoints))
					for _, dp := range ds.DataPoints {
						clear(attrs)
						for _, kv := range dp.Attributes {
							attrs[kv.Key] = kv.Value.StringValue
						}
						ev := usageEvent{
							tenant: attrs["tenant"],
							model:  firstNonEmpty(attrs["gen_ai.request.model"], attrs["model"]),
						}
						switch attrs["gen_ai.token.type"] {
						case "output", "completion":
							ev.output = dpValue(dp)
						default:
							ev.input = dpValue(dp)
						}
						events = append(events, ev)
					}
				}
			}
		}
	}
	return events, nil
}

func dpValue(dp otlpDataPoint) int {
	if dp.AsInt != "" {
		if n, err := strconv.Atoi(dp.AsInt); err == nil {
			return n
		}
	}
	return int(dp.AsDouble)
}

func firstNonEmpty(a, b string) string {
	if a != "" {
		return a
	}
	return b
}

// handleOTLP ingests OTLP/JSON metrics into the cost ledger.
func (s *Server) handleOTLP(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "POST only", http.StatusMethodNotAllowed)
		return
	}
	if s.ledger == nil {
		http.Error(w, "ledger disabled", http.StatusServiceUnavailable)
		return
	}
	body, err := io.ReadAll(r.Body)
	if err != nil {
		if status, msg, over := bodyCapError(err); over {
			http.Error(w, msg, status)
			return
		}
		http.Error(w, "read error", http.StatusBadRequest)
		return
	}
	events, err := parseOTLPUsage(body)
	if err != nil {
		http.Error(w, "bad otlp json: "+err.Error(), http.StatusBadRequest)
		return
	}
	now := time.Now().UTC()
	recorded, failed := 0, 0
	for _, ev := range events {
		// Financial data: a persistence error must NOT be swallowed. Count each
		// datapoint as recorded or failed, surface failures on the metrics
		// registry, and let the status code reflect partial/total loss so the
		// caller (and dashboards) see the outage instead of a false 202.
		if err := s.ledger.Record(r.Context(), ev.tenant, ev.model, ev.input, ev.output, now); err != nil {
			failed++
			s.metrics.Inc(metricUsageRecordFailed)
			continue
		}
		recorded++
	}

	// 202 only when everything persisted. Any loss => non-2xx so the failure is
	// not reported as success: 207 Multi-Status when at least one datapoint
	// persisted (partial), 500 when nothing did (total outage).
	status := http.StatusAccepted
	switch {
	case failed > 0 && recorded > 0:
		status = http.StatusMultiStatus
	case failed > 0:
		status = http.StatusInternalServerError
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(map[string]int{"recorded": recorded, "failed": failed})
}
