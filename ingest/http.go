// Package ingest exposes an HTTP receiver that feeds signals + usage into the loop.
// This is the v0 ingest surface; an OTLP-collector → /v1/usage shim is a thin adapter.
package ingest

import (
	"encoding/json"
	"errors"
	"net/http"
	"sync"
	"time"

	"github.com/sloppyjoe/sloppy/core"
	"github.com/sloppyjoe/sloppy/engine"
	"github.com/sloppyjoe/sloppy/ledger"
	"github.com/sloppyjoe/sloppy/metrics"
)

// metricUsageRecordFailed counts usage/OTLP datapoints whose ledger.Record failed
// to persist. It turns silently-dropped financial data into an observable signal on
// the /status registry.
const metricUsageRecordFailed = "usage_record_failed"

// Per-handler request body caps (defense against unbounded reads). http.MaxBytesReader
// short-circuits the read and lets the handler return 4xx instead of buffering an
// attacker-controlled body into memory. Signals and single-event usage are small
// JSON; OTLP metrics batches are larger but still bounded.
const (
	maxSignalBytes = 1 << 20  // 1 MiB
	maxUsageBytes  = 1 << 20  // 1 MiB
	maxOTLPBytes   = 16 << 20 // 16 MiB (OTLP batches can be large)
)

// Routes is the set of paths the ingest mux registers, in registration order. It
// is the single source of truth used both to build the mux and by the RBAC guard
// test to assert every registered route has an explicit ee.ScopeForPath mapping
// (an unmapped route must fail closed, not inherit a write scope).
var Routes = []string{
	"/healthz",
	"/v1/signals",
	"/v1/usage",
	"/v1/otlp/metrics",
	"/status",
}

// Server adapts HTTP requests into engine + ledger calls.
type Server struct {
	engine  *engine.Engine
	ledger  *ledger.CostLedger // optional; nil disables /v1/usage
	metrics *metrics.Registry  // optional; powers /status

	// statusMu guards statusBuf, the single long-lived buffer the /status
	// handler reuses across polls via metrics.SnapshotInto so the routinely
	// polled path is allocation-free in steady state. Concurrent /status
	// requests serialize on this mutex while filling and encoding the buffer.
	statusMu  sync.Mutex
	statusBuf map[string]int64
}

// Option configures a Server. Mirrors engine.New's functional-options style so
// optional dependencies are wired the same way across the codebase.
type Option func(*Server)

// WithLedger supplies a cost ledger; nil (the default) disables /v1/usage and
// /v1/otlp/metrics.
func WithLedger(l *ledger.CostLedger) Option { return func(s *Server) { s.ledger = l } }

// WithMetrics attaches a metrics registry to expose at /status.
func WithMetrics(m *metrics.Registry) Option { return func(s *Server) { s.metrics = m } }

// NewServer builds the ingest server. Optional dependencies are opt-in via Options.
func NewServer(e *engine.Engine, opts ...Option) *Server {
	s := &Server{engine: e}
	for _, o := range opts {
		o(s)
	}
	return s
}

// capBody wraps h so the request body is limited to max bytes via
// http.MaxBytesReader. A read past the limit returns an error the handler turns
// into a 4xx, instead of buffering an unbounded body into memory.
func capBody(max int64, h http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		r.Body = http.MaxBytesReader(w, r.Body, max)
		h(w, r)
	}
}

// bodyCapError reports whether err is the MaxBytesReader over-limit error and, if
// so, the 413 status and message a handler should return. Over-limit bodies are a
// client fault (too large), distinct from malformed JSON (400).
func bodyCapError(err error) (status int, msg string, over bool) {
	var mbe *http.MaxBytesError
	if errors.As(err, &mbe) {
		return http.StatusRequestEntityTooLarge, "request body too large", true
	}
	return 0, "", false
}

// Handler returns the configured HTTP mux. Routes must stay in sync with the
// Routes slice (asserted by the RBAC guard test).
func (s *Server) Handler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("/healthz", func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("ok"))
	})
	mux.HandleFunc("/v1/signals", capBody(maxSignalBytes, s.handleSignal))
	mux.HandleFunc("/v1/usage", capBody(maxUsageBytes, s.handleUsage))
	mux.HandleFunc("/v1/otlp/metrics", capBody(maxOTLPBytes, s.handleOTLP))
	mux.HandleFunc("/status", s.handleStatus)
	return mux
}

func (s *Server) handleStatus(w http.ResponseWriter, _ *http.Request) {
	// Reuse one long-lived buffer across polls so the routinely-polled /status
	// path is allocation-free in steady state. SnapshotInto clears and refills
	// the buffer under the registry mutex; statusMu serializes concurrent
	// /status requests sharing the single buffer. A nil registry yields an empty
	// (cleared) buffer, matching the previous empty-map response.
	s.statusMu.Lock()
	defer s.statusMu.Unlock()
	s.statusBuf = s.metrics.SnapshotInto(s.statusBuf)
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(s.statusBuf)
}

type signalResp struct {
	Applied int             `json:"applied"`
	Results []resultSummary `json:"results"`
}

type resultSummary struct {
	Outcome string `json:"outcome"`
	Kind    string `json:"kind"`
	Target  string `json:"target"`
}

func (s *Server) handleSignal(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "POST only", http.StatusMethodNotAllowed)
		return
	}
	var sig core.Signal
	if err := json.NewDecoder(r.Body).Decode(&sig); err != nil {
		if status, msg, over := bodyCapError(err); over {
			http.Error(w, msg, status)
			return
		}
		http.Error(w, "bad signal json: "+err.Error(), http.StatusBadRequest)
		return
	}
	results, err := s.engine.Handle(r.Context(), sig)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	resp := signalResp{}
	for _, res := range results {
		if res.Outcome == engine.OutApplied {
			resp.Applied++
		}
		resp.Results = append(resp.Results, resultSummary{
			Outcome: string(res.Outcome), Kind: string(res.Intent.Kind), Target: res.Intent.Target,
		})
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(resp)
}

type usageReq struct {
	Tenant       string `json:"tenant"`
	Model        string `json:"model"`
	InputTokens  int    `json:"input_tokens"`
	OutputTokens int    `json:"output_tokens"`
	At           string `json:"at"` // optional RFC3339
}

func (s *Server) handleUsage(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "POST only", http.StatusMethodNotAllowed)
		return
	}
	if s.ledger == nil {
		http.Error(w, "ledger disabled", http.StatusServiceUnavailable)
		return
	}
	var u usageReq
	if err := json.NewDecoder(r.Body).Decode(&u); err != nil {
		if status, msg, over := bodyCapError(err); over {
			http.Error(w, msg, status)
			return
		}
		http.Error(w, "bad usage json: "+err.Error(), http.StatusBadRequest)
		return
	}
	at := time.Now().UTC()
	if u.At != "" {
		if parsed, err := time.Parse(time.RFC3339, u.At); err == nil {
			at = parsed
		}
	}
	if err := s.ledger.Record(r.Context(), u.Tenant, u.Model, u.InputTokens, u.OutputTokens, at); err != nil {
		// Single-event contract: still 500, but surface the loss on the metrics
		// registry (handleOTLP shares this counter) instead of only logging via
		// the response body.
		s.metrics.Inc(metricUsageRecordFailed)
		http.Error(w, "record error: "+err.Error(), http.StatusInternalServerError)
		return
	}
	w.WriteHeader(http.StatusAccepted)
}
