package ingest

import (
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
	"github.com/sloppyjoe/sloppy/rules"
	"github.com/sloppyjoe/sloppy/state"
)

func testServer(t *testing.T) (*Server, *ledger.CostLedger) {
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
	l := ledger.New(ledger.PriceBook{"gpt-4o": {InputPer1K: 5, OutputPer1K: 15}})
	e := engine.New(rec, reg, st, signer, engine.WithLedger(l))
	return NewServer(e, l), l
}

func TestIngestHealth(t *testing.T) {
	s, _ := testServer(t)
	srv := httptest.NewServer(s.Handler())
	defer srv.Close()
	resp, err := http.Get(srv.URL + "/healthz")
	if err != nil || resp.StatusCode != http.StatusOK {
		t.Fatalf("healthz: %v code=%v", err, resp.StatusCode)
	}
}

func TestIngestSignalFiresLoop(t *testing.T) {
	s, _ := testServer(t)
	srv := httptest.NewServer(s.Handler())
	defer srv.Close()
	body := `{"type":"cost.budget_burn","correlation_key":"acme:cost","subject":{"alias":"gpt-4o"},"data":{"spend_1h_usd":9.0}}`
	resp, err := http.Post(srv.URL+"/v1/signals", "application/json", strings.NewReader(body))
	if err != nil || resp.StatusCode != http.StatusOK {
		t.Fatalf("post signal: %v code=%v", err, resp.StatusCode)
	}
	out, _ := io.ReadAll(resp.Body)
	if !strings.Contains(string(out), `"applied":1`) {
		t.Fatalf("expected applied:1, got %s", out)
	}
}

func TestIngestUsageFeedsLedger(t *testing.T) {
	s, l := testServer(t)
	srv := httptest.NewServer(s.Handler())
	defer srv.Close()
	at := time.Now().UTC().Format(time.RFC3339)
	body := `{"tenant":"acme","model":"gpt-4o","input_tokens":1000,"output_tokens":1000,"at":"` + at + `"}`
	resp, err := http.Post(srv.URL+"/v1/usage", "application/json", strings.NewReader(body))
	if err != nil || resp.StatusCode != http.StatusAccepted {
		t.Fatalf("post usage: %v code=%v", err, resp.StatusCode)
	}
	if got := l.Spend("acme", time.Hour, time.Now().UTC().Add(time.Second)); got < 19.9 {
		t.Fatalf("ledger should reflect ~$20 spend, got %v", got)
	}
}
