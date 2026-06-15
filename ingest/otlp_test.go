package ingest

import (
	"context"
	"errors"
	"fmt"
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

// TestParseOTLPUsageMultiDatapoint pins the exact parse output for a batch with
// several datapoints (input/output types, multiple tenants/models, the `model`
// fallback key, and a non-token metric that must be skipped). It guards the
// allocation-reduction refactor — reusing the attrs map and presizing events
// must not change the produced events.
func TestParseOTLPUsageMultiDatapoint(t *testing.T) {
	body := []byte(`{"resourceMetrics":[{"scopeMetrics":[{"metrics":[
	  {"name":"gen_ai.client.token.usage","sum":{"dataPoints":[
	    {"asInt":"100","attributes":[
	      {"key":"gen_ai.token.type","value":{"stringValue":"input"}},
	      {"key":"tenant","value":{"stringValue":"t1"}},
	      {"key":"gen_ai.request.model","value":{"stringValue":"m1"}}]},
	    {"asInt":"200","attributes":[
	      {"key":"gen_ai.token.type","value":{"stringValue":"output"}},
	      {"key":"tenant","value":{"stringValue":"t1"}},
	      {"key":"gen_ai.request.model","value":{"stringValue":"m1"}}]}]}},
	  {"name":"gen_ai.server.token.usage","gauge":{"dataPoints":[
	    {"asDouble":300,"attributes":[
	      {"key":"gen_ai.token.type","value":{"stringValue":"completion"}},
	      {"key":"tenant","value":{"stringValue":"t2"}},
	      {"key":"model","value":{"stringValue":"m2"}}]},
	    {"asInt":"400","attributes":[
	      {"key":"tenant","value":{"stringValue":"t2"}},
	      {"key":"model","value":{"stringValue":"m2"}}]}]}},
	  {"name":"latency.seconds","sum":{"dataPoints":[
	    {"asInt":"999","attributes":[
	      {"key":"tenant","value":{"stringValue":"ignore"}}]}]}}]}]}]}`)
	got, err := parseOTLPUsage(body)
	if err != nil {
		t.Fatal(err)
	}
	want := []usageEvent{
		{tenant: "t1", model: "m1", input: 100},
		{tenant: "t1", model: "m1", output: 200},
		{tenant: "t2", model: "m2", output: 300},
		{tenant: "t2", model: "m2", input: 400},
	}
	if len(got) != len(want) {
		t.Fatalf("event count: got %d want %d (%+v)", len(got), len(want), got)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("event %d: got %+v want %+v", i, got[i], want[i])
		}
	}
}

// otlpBatchWithDataPoints builds an OTLP/JSON metrics body whose single
// token-named sum metric carries n input datapoints. Used to drive the
// per-batch event cap without hand-writing huge JSON literals.
func otlpBatchWithDataPoints(n int) []byte {
	var b strings.Builder
	b.WriteString(`{"resourceMetrics":[{"scopeMetrics":[{"metrics":[{"name":"gen_ai.client.token.usage","sum":{"dataPoints":[`)
	for i := 0; i < n; i++ {
		if i > 0 {
			b.WriteByte(',')
		}
		fmt.Fprintf(&b, `{"asInt":"1","attributes":[`+
			`{"key":"gen_ai.token.type","value":{"stringValue":"input"}},`+
			`{"key":"tenant","value":{"stringValue":"t"}},`+
			`{"key":"gen_ai.request.model","value":{"stringValue":"m"}}]}`)
	}
	b.WriteString(`]}}]}]}]}`)
	return []byte(b.String())
}

// TestParseOTLPUsageAtCap pins under-cap behavior: a batch with exactly
// maxOTLPEvents datapoints is fully extracted, unchanged, with no error.
func TestParseOTLPUsageAtCap(t *testing.T) {
	ev, err := parseOTLPUsage(otlpBatchWithDataPoints(maxOTLPEvents))
	if err != nil {
		t.Fatalf("at-cap batch must not error: %v", err)
	}
	if len(ev) != maxOTLPEvents {
		t.Fatalf("event count: got %d want %d", len(ev), maxOTLPEvents)
	}
	// Each datapoint priced exactly as before (asInt 1, input type).
	if ev[0].tenant != "t" || ev[0].model != "m" || ev[0].input != 1 || ev[0].output != 0 {
		t.Fatalf("first event mispriced: %+v", ev[0])
	}
}

// TestParseOTLPUsageOverCap pins over-cap behavior: one datapoint past the cap
// trips the bounded error with no panic, no partial slice, and no oversized
// allocation (the returned slice is nil, not a cap+1-length working set).
func TestParseOTLPUsageOverCap(t *testing.T) {
	ev, err := parseOTLPUsage(otlpBatchWithDataPoints(maxOTLPEvents + 1))
	if !errors.Is(err, errOTLPEventCap) {
		t.Fatalf("over-cap batch must return errOTLPEventCap, got err=%v", err)
	}
	if ev != nil {
		t.Fatalf("over-cap batch must not return a partial/oversized slice, got len=%d", len(ev))
	}
}

// TestOTLPMetricsEndpointEventCap413 confirms the handler surfaces the event
// cap as a 413, mirroring the maxOTLPBytes body cap (not a 400 for bad JSON).
func TestOTLPMetricsEndpointEventCap413(t *testing.T) {
	s, _ := testServer(t)
	srv := httptest.NewServer(s.Handler())
	defer srv.Close()
	body := otlpBatchWithDataPoints(maxOTLPEvents + 1)
	resp, err := http.Post(srv.URL+"/v1/otlp/metrics", "application/json", strings.NewReader(string(body)))
	if err != nil {
		t.Fatalf("otlp post: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusRequestEntityTooLarge {
		t.Fatalf("over-cap batch: got status %d want %d", resp.StatusCode, http.StatusRequestEntityTooLarge)
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
	got, _ := l.Spend(context.Background(), "acme", time.Hour, time.Now().UTC().Add(time.Second))
	if got < 19.9 {
		t.Fatalf("ledger should reflect ~$20 from OTLP, got %v", got)
	}
}
