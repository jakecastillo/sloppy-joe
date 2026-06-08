package ingest

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestParseOTLPUsageUnit(t *testing.T) {
	body := []byte(`{"resourceMetrics":[{"scopeMetrics":[{"metrics":[
	  {"name":"gen_ai.client.token.usage","sum":{"dataPoints":[
	    {"asInt":"500","attributes":[
	      {"key":"gen_ai.token.type","value":{"stringValue":"input"}},
	      {"key":"tenant","value":{"stringValue":"t1"}},
	      {"key":"gen_ai.request.model","value":{"stringValue":"m1"}}]}]}}]}]}]}`)
	ev, err := parseOTLPUsage(body)
	if err != nil {
		t.Fatal(err)
	}
	if len(ev) != 1 || ev[0].tenant != "t1" || ev[0].model != "m1" || ev[0].input != 500 || ev[0].output != 0 {
		t.Fatalf("bad parse: %+v", ev)
	}
}

func TestOTLPMetricsEndpointFeedsLedger(t *testing.T) {
	s, l := testServer(t)
	srv := httptest.NewServer(s.Handler())
	defer srv.Close()
	body := `{"resourceMetrics":[{"scopeMetrics":[{"metrics":[
	  {"name":"gen_ai.client.token.usage","sum":{"dataPoints":[
	    {"asInt":"1000","attributes":[
	      {"key":"gen_ai.token.type","value":{"stringValue":"input"}},
	      {"key":"tenant","value":{"stringValue":"acme"}},
	      {"key":"gen_ai.request.model","value":{"stringValue":"gpt-4o"}}]},
	    {"asInt":"1000","attributes":[
	      {"key":"gen_ai.token.type","value":{"stringValue":"output"}},
	      {"key":"tenant","value":{"stringValue":"acme"}},
	      {"key":"gen_ai.request.model","value":{"stringValue":"gpt-4o"}}]}]}}]}]}]}`
	resp, err := http.Post(srv.URL+"/v1/otlp/metrics", "application/json", strings.NewReader(body))
	if err != nil || resp.StatusCode != http.StatusAccepted {
		t.Fatalf("otlp post: %v code=%v", err, resp.StatusCode)
	}
	// testServer price book: gpt-4o input 5/1k + output 15/1k → 1000+1000 toks = $20.
	if got := l.Spend("acme", time.Hour, time.Now().UTC().Add(time.Second)); got < 19.9 {
		t.Fatalf("ledger should reflect ~$20 from OTLP, got %v", got)
	}
}
