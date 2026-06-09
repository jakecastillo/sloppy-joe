package ingest

import (
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/sloppyjoe/sloppy/actuator"
	"github.com/sloppyjoe/sloppy/engine"
	"github.com/sloppyjoe/sloppy/intent"
	"github.com/sloppyjoe/sloppy/rules"
	"github.com/sloppyjoe/sloppy/state"
)

// ledgerlessServer builds an ingest Server with NO ledger attached, so the
// usage/OTLP handlers exercise their `s.ledger == nil` 503 branch.
func ledgerlessServer(t *testing.T) *Server {
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
	e := engine.New(rec, reg, st, signer)
	return NewServer(e, nil) // nil ledger -> /v1/usage and /v1/otlp/metrics disabled
}

// TestIngestRejectsNonPostMethods pins the 405 branch on every mutating handler:
// a GET to a POST-only ingest endpoint must be 405 Method Not Allowed, not silently
// accepted or 404'd.
func TestIngestRejectsNonPostMethods(t *testing.T) {
	s, _ := testServer(t)
	srv := httptest.NewServer(s.Handler())
	defer srv.Close()

	for _, path := range []string{"/v1/signals", "/v1/usage", "/v1/otlp/metrics"} {
		resp, err := http.Get(srv.URL + path)
		if err != nil {
			t.Fatalf("%s: get error: %v", path, err)
		}
		resp.Body.Close()
		if resp.StatusCode != http.StatusMethodNotAllowed {
			t.Fatalf("%s: GET must be 405, got %d", path, resp.StatusCode)
		}
	}
}

// TestUsageLedgerDisabledReturns503 pins that POSTing usage when no ledger is wired
// returns 503 Service Unavailable (the feature is off), not a 500 or a false 202.
func TestUsageLedgerDisabledReturns503(t *testing.T) {
	s := ledgerlessServer(t)
	srv := httptest.NewServer(s.Handler())
	defer srv.Close()

	body := `{"tenant":"acme","model":"gpt-4o","input_tokens":1000,"output_tokens":1000}`
	resp, err := http.Post(srv.URL+"/v1/usage", "application/json", strings.NewReader(body))
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusServiceUnavailable {
		t.Fatalf("usage with no ledger must be 503, got %d", resp.StatusCode)
	}
}

// TestOTLPLedgerDisabledReturns503 is the OTLP counterpart of the 503 branch.
func TestOTLPLedgerDisabledReturns503(t *testing.T) {
	s := ledgerlessServer(t)
	srv := httptest.NewServer(s.Handler())
	defer srv.Close()

	resp, err := http.Post(srv.URL+"/v1/otlp/metrics", "application/json", strings.NewReader(`{}`))
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusServiceUnavailable {
		t.Fatalf("otlp with no ledger must be 503, got %d", resp.StatusCode)
	}
}

// TestUsageBadJSONReturns400 pins the within-limit malformed-JSON branch on the
// usage handler (a parse fault is a 400, distinct from the 413 over-limit fault and
// the 503 disabled fault).
func TestUsageBadJSONReturns400(t *testing.T) {
	s, _ := testServer(t)
	srv := httptest.NewServer(s.Handler())
	defer srv.Close()

	resp, err := http.Post(srv.URL+"/v1/usage", "application/json", strings.NewReader("{not json"))
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("malformed usage json must be 400, got %d", resp.StatusCode)
	}
}

// TestOTLPBadJSONReturns400 pins the OTLP malformed-JSON branch (parseOTLPUsage
// unmarshal error) as a 400.
func TestOTLPBadJSONReturns400(t *testing.T) {
	s, _ := testServer(t)
	srv := httptest.NewServer(s.Handler())
	defer srv.Close()

	resp, err := http.Post(srv.URL+"/v1/otlp/metrics", "application/json", strings.NewReader("{not json"))
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("malformed otlp json must be 400, got %d", resp.StatusCode)
	}
}

// TestOTLPEmptyBatchIs202 pins the boundary: a well-formed OTLP doc with zero usage
// datapoints records nothing and fails nothing, so the handler returns 202 (the
// loop reflects no loss). This guards the recorded==0 && failed==0 path from being
// mistaken for the total-outage 500 path.
func TestOTLPEmptyBatchIs202(t *testing.T) {
	s, _ := testServer(t)
	srv := httptest.NewServer(s.Handler())
	defer srv.Close()

	resp, err := http.Post(srv.URL+"/v1/otlp/metrics", "application/json", strings.NewReader(`{"resourceMetrics":[]}`))
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusAccepted {
		t.Fatalf("empty OTLP batch (no datapoints) must be 202, got %d", resp.StatusCode)
	}
}
