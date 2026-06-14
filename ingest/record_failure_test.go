package ingest

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/sloppyjoe/sloppy/actuator"
	"github.com/sloppyjoe/sloppy/engine"
	"github.com/sloppyjoe/sloppy/intent"
	"github.com/sloppyjoe/sloppy/ledger"
	"github.com/sloppyjoe/sloppy/metrics"
	"github.com/sloppyjoe/sloppy/rules"
	"github.com/sloppyjoe/sloppy/state"
)

// flakyUsageStore is a ledger.UsageStore whose RecordUsage fails for any tenant
// in failTenants. It lets ingest tests force ledger.Record errors deterministically
// (whole-batch or per-datapoint) without a real backing store.
type flakyUsageStore struct {
	failTenants map[string]bool
}

func (f *flakyUsageStore) RecordUsage(_ context.Context, tenant, _ string, _ float64, _ time.Time) error {
	if f.failTenants[tenant] {
		return errFakeRecord
	}
	return nil
}

func (f *flakyUsageStore) SpendSince(_ context.Context, _ string, _ time.Time) (float64, error) {
	return 0, nil
}

var errFakeRecord = errRecord("boom: usage store down")

type errRecord string

func (e errRecord) Error() string { return string(e) }

// failServer builds an ingest Server whose ledger persists through a flakyUsageStore
// configured to fail for the given tenants, with a metrics registry attached so tests
// can assert counters.
func failServer(t *testing.T, failTenants ...string) (*Server, *metrics.Registry) {
	t.Helper()
	rs, err := rules.ParseRules([]byte(`
on: cost.budget_burn
when: signal.data.spend_1h_usd > 5.0
then: [ { route_override: { alias: gpt-4o, to: ollama/llama3 } } ]
`))
	if err != nil {
		t.Fatal(err)
	}
	rec, _ := rules.NewReconciler(rs)
	st, err := state.OpenSQLite(t.TempDir() + "/i.db")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { st.Close() })
	reg := actuator.NewRegistry()
	reg.Register(&actuator.Log{W: io.Discard})
	signer, _ := intent.NewEd25519Signer()

	fail := map[string]bool{}
	for _, tn := range failTenants {
		fail[tn] = true
	}
	l := ledger.New(
		ledger.PriceBook{"gpt-4o": {InputPer1K: 5, OutputPer1K: 15}},
		&flakyUsageStore{failTenants: fail},
	)
	e := engine.New(rec, reg, st, signer, engine.WithLedger(l))
	m := metrics.New()
	return NewServer(e, WithLedger(l), WithMetrics(m)), m
}

func otlpDatapoint(tenant, ttype string, asInt string) string {
	return `{"asInt":"` + asInt + `","attributes":[
		{"key":"gen_ai.token.type","value":{"stringValue":"` + ttype + `"}},
		{"key":"tenant","value":{"stringValue":"` + tenant + `"}},
		{"key":"gen_ai.request.model","value":{"stringValue":"gpt-4o"}}]}`
}

func otlpBody(dps ...string) string {
	return `{"resourceMetrics":[{"scopeMetrics":[{"metrics":[
		{"name":"gen_ai.client.token.usage","sum":{"dataPoints":[` +
		strings.Join(dps, ",") + `]}}]}]}]}`
}

// TestOTLPAllFailNonSuccess: every datapoint fails to persist -> non-2xx, failed
// count == number of events, recorded == 0, and usage_record_failed metric reflects it.
func TestOTLPAllFailNonSuccess(t *testing.T) {
	s, m := failServer(t, "acme")
	srv := httptest.NewServer(s.Handler())
	defer srv.Close()

	body := otlpBody(
		otlpDatapoint("acme", "input", "1000"),
		otlpDatapoint("acme", "output", "1000"),
	)
	resp, err := http.Post(srv.URL+"/v1/otlp/metrics", "application/json", strings.NewReader(body))
	if err != nil {
		t.Fatal(err)
	}
	if resp.StatusCode/100 == 2 {
		t.Fatalf("expected non-2xx on total failure, got %d", resp.StatusCode)
	}
	var out struct {
		Recorded int `json:"recorded"`
		Failed   int `json:"failed"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		t.Fatal(err)
	}
	if out.Recorded != 0 || out.Failed != 2 {
		t.Fatalf("want recorded=0 failed=2, got %+v", out)
	}
	if got := m.Snapshot()["usage_record_failed"]; got != 2 {
		t.Fatalf("want usage_record_failed=2, got %d", got)
	}
}

// TestOTLPPartialFailure: some datapoints persist, some fail -> 207 Multi-Status,
// correct recorded/failed split, and the metric counts only the failures.
func TestOTLPPartialFailure(t *testing.T) {
	s, m := failServer(t, "broke")
	srv := httptest.NewServer(s.Handler())
	defer srv.Close()

	body := otlpBody(
		otlpDatapoint("acme", "input", "1000"),  // persists
		otlpDatapoint("broke", "input", "1000"), // fails
		otlpDatapoint("acme", "output", "1000"), // persists
	)
	resp, err := http.Post(srv.URL+"/v1/otlp/metrics", "application/json", strings.NewReader(body))
	if err != nil {
		t.Fatal(err)
	}
	if resp.StatusCode != http.StatusMultiStatus {
		t.Fatalf("expected 207 on partial failure, got %d", resp.StatusCode)
	}
	var out struct {
		Recorded int `json:"recorded"`
		Failed   int `json:"failed"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		t.Fatal(err)
	}
	if out.Recorded != 2 || out.Failed != 1 {
		t.Fatalf("want recorded=2 failed=1, got %+v", out)
	}
	if got := m.Snapshot()["usage_record_failed"]; got != 1 {
		t.Fatalf("want usage_record_failed=1, got %d", got)
	}
}

// TestOTLPHappyPath: all datapoints persist -> 202 Accepted, recorded == len(events),
// failed == 0, no failure metric.
func TestOTLPHappyPath(t *testing.T) {
	s, m := failServer(t) // no failing tenants
	srv := httptest.NewServer(s.Handler())
	defer srv.Close()

	body := otlpBody(
		otlpDatapoint("acme", "input", "1000"),
		otlpDatapoint("acme", "output", "1000"),
	)
	resp, err := http.Post(srv.URL+"/v1/otlp/metrics", "application/json", strings.NewReader(body))
	if err != nil {
		t.Fatal(err)
	}
	if resp.StatusCode != http.StatusAccepted {
		t.Fatalf("expected 202 on happy path, got %d", resp.StatusCode)
	}
	var out struct {
		Recorded int `json:"recorded"`
		Failed   int `json:"failed"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		t.Fatal(err)
	}
	if out.Recorded != 2 || out.Failed != 0 {
		t.Fatalf("want recorded=2 failed=0, got %+v", out)
	}
	if got := m.Snapshot()["usage_record_failed"]; got != 0 {
		t.Fatalf("want usage_record_failed=0, got %d", got)
	}
}

// TestUsageFailIncrementsMetric: handleUsage keeps its single-event 500 contract but
// must also surface the failure via usage_record_failed (was a silent metric gap).
func TestUsageFailIncrementsMetric(t *testing.T) {
	s, m := failServer(t, "acme")
	srv := httptest.NewServer(s.Handler())
	defer srv.Close()

	body := `{"tenant":"acme","model":"gpt-4o","input_tokens":1000,"output_tokens":1000}`
	resp, err := http.Post(srv.URL+"/v1/usage", "application/json", strings.NewReader(body))
	if err != nil {
		t.Fatal(err)
	}
	if resp.StatusCode != http.StatusInternalServerError {
		t.Fatalf("expected 500 on usage record failure, got %d", resp.StatusCode)
	}
	if got := m.Snapshot()["usage_record_failed"]; got != 1 {
		t.Fatalf("want usage_record_failed=1, got %d", got)
	}
}
