package main

import (
	"context"
	"io"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestServeHealthSignalAndStatus(t *testing.T) {
	dir := t.TempDir()
	rulesDir := filepath.Join(dir, "rules")
	if err := os.MkdirAll(rulesDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(rulesDir, "c.yaml"), []byte(`
on: cost.budget_burn
when: signal.data.spend_1h_usd > 5.0
then: [ { route_override: { alias: gpt-4o, to: ollama/llama3 } } ]
`), 0o644); err != nil {
		t.Fatal(err)
	}
	e, l, m, cleanup, err := buildEngine(rulesDir, filepath.Join(dir, "d.db"), "", filepath.Join(dir, "k.key"), false, io.Discard)
	if err != nil {
		t.Fatal(err)
	}
	defer cleanup()

	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() { done <- serve(ctx, ln, e, l, m, time.Hour, io.Discard) }()

	base := "http://" + ln.Addr().String()
	var resp *http.Response
	for i := 0; i < 100; i++ {
		resp, err = http.Get(base + "/healthz")
		if err == nil {
			break
		}
		time.Sleep(10 * time.Millisecond)
	}
	if err != nil || resp.StatusCode != http.StatusOK {
		t.Fatalf("healthz never came up: %v", err)
	}

	body := `{"type":"cost.budget_burn","correlation_key":"acme:cost","subject":{"alias":"gpt-4o"},"data":{"spend_1h_usd":9.0}}`
	resp, err = http.Post(base+"/v1/signals", "application/json", strings.NewReader(body))
	if err != nil || resp.StatusCode != http.StatusOK {
		t.Fatalf("post signal: %v", err)
	}
	out, _ := io.ReadAll(resp.Body)
	if !strings.Contains(string(out), `"applied":1`) {
		t.Fatalf("expected applied:1, got %s", out)
	}

	// /status reflects self-metrics.
	resp, err = http.Get(base + "/status")
	if err != nil || resp.StatusCode != http.StatusOK {
		t.Fatalf("status: %v", err)
	}
	st, _ := io.ReadAll(resp.Body)
	if !strings.Contains(string(st), `"signals_handled":1`) || !strings.Contains(string(st), `"intents_applied":1`) {
		t.Fatalf("status metrics unexpected: %s", st)
	}

	cancel()
	select {
	case <-done:
	case <-time.After(5 * time.Second):
		t.Fatal("serve did not shut down on cancel")
	}
}
