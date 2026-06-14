package main

import (
	"bytes"
	"context"
	"io"
	"log/slog"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/sloppyjoe/sloppy/actuator"
	"github.com/sloppyjoe/sloppy/bootstrap"
	"github.com/sloppyjoe/sloppy/config"
	"github.com/sloppyjoe/sloppy/core"
	"github.com/sloppyjoe/sloppy/ee"
	"github.com/sloppyjoe/sloppy/engine"
	"github.com/sloppyjoe/sloppy/intent"
	"github.com/sloppyjoe/sloppy/metrics"
	"github.com/sloppyjoe/sloppy/rules"
	"github.com/sloppyjoe/sloppy/state"
)

// scanFailStore satisfies state.Store via a real backend but fails the two scan
// reads the ticker drives — DueReverts (ProcessDueReverts) and PruneUsage — to
// simulate a store blip during the background revert scan.
type scanFailStore struct {
	state.Store
}

func (scanFailStore) DueReverts(context.Context, time.Time) ([]state.PendingRevert, error) {
	return nil, errScan
}
func (scanFailStore) PruneUsage(context.Context, time.Time) error { return errScan }

var errScan = errScanError("store unavailable")

type errScanError string

func (e errScanError) Error() string { return string(e) }

// The revert ticker must not silently swallow scan errors: a failed
// ProcessDueReverts and a failed PruneUsage each log a Warn and bump
// revert_scan_failed, so a store outage during the safety-net scan is visible.
func TestRevertScanErrorsLoggedAndCounted(t *testing.T) {
	inner, err := state.OpenSQLite(t.TempDir() + "/scan.db")
	if err != nil {
		t.Fatal(err)
	}
	defer inner.Close()
	st := scanFailStore{Store: inner}

	rs, _ := rules.ParseRules([]byte(`
on: cost.budget_burn
when: signal.data.spend_1h_usd > 5.0
then: [ { route_override: { alias: gpt-4o, to: ollama/llama3 } } ]
`))
	rec, _ := rules.NewReconciler(rs)
	reg := actuator.NewRegistry()
	reg.Register(&actuator.Fake{})
	signer, _ := intent.NewEd25519Signer()
	m := metrics.New()
	e := engine.New(rec, reg, st, signer, engine.WithMetrics(m))

	var buf bytes.Buffer
	logger := slog.New(slog.NewTextHandler(&buf, &slog.HandlerOptions{Level: slog.LevelWarn}))

	runRevertScan(context.Background(), e, m, logger, time.Unix(1749340800, 0).UTC())

	out := buf.String()
	if !strings.Contains(out, "process due reverts") {
		t.Fatalf("expected a Warn log for the ProcessDueReverts error, got: %q", out)
	}
	if !strings.Contains(out, "prune usage") {
		t.Fatalf("expected a Warn log for the PruneUsage error, got: %q", out)
	}
	if m.Snapshot()["revert_scan_failed"] != 2 {
		t.Fatalf("revert_scan_failed must count both scan failures, got %d", m.Snapshot()["revert_scan_failed"])
	}
}

// A clean scan with work done logs the revert count and bumps nothing failure-side.
func TestRevertScanSuccessLogsCount(t *testing.T) {
	base := time.Unix(1749340800, 0).UTC()
	inner, err := state.OpenSQLite(t.TempDir() + "/ok.db")
	if err != nil {
		t.Fatal(err)
	}
	defer inner.Close()

	rs, _ := rules.ParseRules([]byte(`
on: cost.budget_burn
when: signal.data.spend_1h_usd > 5.0
then: [ { route_override: { alias: gpt-4o, to: ollama/llama3, ttl: 30m } } ]
`))
	rec, _ := rules.NewReconciler(rs)
	reg := actuator.NewRegistry()
	reg.Register(&actuator.Fake{})
	signer, _ := intent.NewEd25519Signer()
	m := metrics.New()
	e := engine.New(rec, reg, inner, signer, engine.WithMetrics(m),
		engine.WithClock(func() time.Time { return base }))

	sig := core.Signal{
		Type: "cost.budget_burn", CorrelationKey: "acme:cost",
		Subject: core.Subject{Alias: "gpt-4o"}, Data: map[string]any{"spend_1h_usd": 9.0},
	}
	if _, err := e.Handle(context.Background(), sig); err != nil {
		t.Fatal(err)
	}

	var buf bytes.Buffer
	logger := slog.New(slog.NewTextHandler(&buf, &slog.HandlerOptions{Level: slog.LevelInfo}))
	runRevertScan(context.Background(), e, m, logger, base.Add(31*time.Minute))

	if !strings.Contains(buf.String(), "reverted expired intents") {
		t.Fatalf("expected reverted-count log, got: %q", buf.String())
	}
	if m.Snapshot()["revert_scan_failed"] != 0 {
		t.Fatalf("clean scan must not bump revert_scan_failed, got %d", m.Snapshot()["revert_scan_failed"])
	}
}

// The bind guard refuses to expose an unauthenticated control plane on a
// network-reachable address: a non-loopback (incl. wildcard) bind without --auth
// is an error; loopback binds and authenticated public binds are allowed.
func TestBindGuard(t *testing.T) {
	cases := []struct {
		addr      string
		auth      bool
		wantError bool
	}{
		{":8723", false, true},            // wildcard, no auth => refuse
		{"0.0.0.0:8723", false, true},     // explicit wildcard, no auth => refuse
		{"[::]:8723", false, true},        // IPv6 wildcard, no auth => refuse
		{"192.168.1.5:8723", false, true}, // LAN ip, no auth => refuse
		{":8723", true, false},            // wildcard WITH auth => ok
		{"0.0.0.0:8723", true, false},     // wildcard WITH auth => ok
		{"127.0.0.1:8723", false, false},  // loopback, no auth => ok
		{"localhost:8723", false, false},  // loopback name, no auth => ok
		{"[::1]:8723", false, false},      // IPv6 loopback, no auth => ok
	}
	for _, c := range cases {
		err := checkBindAuth(c.addr, c.auth)
		if c.wantError && err == nil {
			t.Fatalf("addr=%q auth=%v: expected a bind-guard error, got nil", c.addr, c.auth)
		}
		if !c.wantError && err != nil {
			t.Fatalf("addr=%q auth=%v: expected no error, got %v", c.addr, c.auth, err)
		}
	}
}

// Startup must LOUDLY distinguish the three auth states so an operator never
// silently runs an open control plane (or an auth-on-but-no-keys lockout).
func TestAuthStateStartupLog(t *testing.T) {
	cases := []struct {
		name     string
		enabled  bool
		keyCount int
		want     string
	}{
		{"auth-off", false, 0, "auth disabled"},
		{"auth-on-with-keys", true, 3, "auth enabled"},
		{"auth-on-empty-keys", true, 0, "auth enabled but NO api keys"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			var buf bytes.Buffer
			logger := slog.New(slog.NewTextHandler(&buf, &slog.HandlerOptions{Level: slog.LevelInfo}))
			logAuthState(logger, c.enabled, c.keyCount)
			out := buf.String()
			if !strings.Contains(out, c.want) {
				t.Fatalf("auth log for %s missing %q, got: %q", c.name, c.want, out)
			}
		})
	}

	// auth-off and auth-on-empty-keys are operational hazards: they must log at WARN.
	var buf bytes.Buffer
	logger := slog.New(slog.NewTextHandler(&buf, &slog.HandlerOptions{Level: slog.LevelWarn}))
	logAuthState(logger, false, 0)
	if !strings.Contains(buf.String(), "auth disabled") {
		t.Fatalf("auth-off must log at WARN (loud), got: %q", buf.String())
	}
	buf.Reset()
	logAuthState(logger, true, 0)
	if !strings.Contains(buf.String(), "NO api keys") {
		t.Fatalf("auth-on-empty-keys must log at WARN (loud), got: %q", buf.String())
	}
}

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
	eff := config.Resolve(config.File{
		Rules:  []string{rulesDir},
		Store:  config.StoreConfig{Kind: "sqlite", Path: filepath.Join(dir, "d.db")},
		Engine: config.EngineConfig{SigningKey: filepath.Join(dir, "k.key")},
	}, true, config.FlagOverrides{}, func(string) string { return "" })
	e, l, m, cleanup, err := bootstrap.BuildEngine(eff, io.Discard, slog.New(slog.DiscardHandler))
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
	go func() { done <- serve(ctx, ln, e, l, m, nil, time.Hour, slog.New(slog.DiscardHandler)) }()

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

// When srv.Serve returns a non-ErrServerClosed error, serve() must return AND its
// background goroutines (ticker + shutdown watcher) must exit on their own — the
// shutdown goroutine no longer parks forever on a ctx that will never be cancelled.
// Forcing the error: close the listener before serve() calls srv.Serve, so Serve
// fails accepting on a closed listener.
func TestServeErrorNoGoroutineLeak(t *testing.T) {
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
	eff := config.Resolve(config.File{
		Rules:  []string{rulesDir},
		Store:  config.StoreConfig{Kind: "sqlite", Path: filepath.Join(dir, "d.db")},
		Engine: config.EngineConfig{SigningKey: filepath.Join(dir, "k.key")},
	}, true, config.FlagOverrides{}, func(string) string { return "" })
	e, l, m, cleanup, err := bootstrap.BuildEngine(eff, io.Discard, slog.New(slog.DiscardHandler))
	if err != nil {
		t.Fatal(err)
	}
	defer cleanup()

	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	// Closing the listener makes srv.Serve(ln) return an accept error that is NOT
	// http.ErrServerClosed, so serve() returns that error.
	if err := ln.Close(); err != nil {
		t.Fatal(err)
	}

	// Let any startup goroutines settle, then snapshot the baseline.
	for i := 0; i < 50 && runtime.NumGoroutine() > 0; i++ {
		runtime.Gosched()
	}
	before := runtime.NumGoroutine()

	// A long revert interval guarantees the ticker exits via serveCtx, never a tick.
	// The parent ctx is NEVER cancelled: the only thing that can unpark the
	// goroutines is serve()'s defer cancel() on its own return.
	serveErr := serve(context.Background(), ln, e, l, m, nil, time.Hour, slog.New(slog.DiscardHandler))
	if serveErr == nil {
		t.Fatal("serve must return the non-ErrServerClosed Serve error, got nil")
	}

	// Poll for the goroutine count to return to baseline. If the shutdown goroutine
	// (or ticker) leaked, it would stay parked and the count would never drop back.
	deadline := time.Now().Add(2 * time.Second)
	for runtime.NumGoroutine() > before && time.Now().Before(deadline) {
		runtime.Gosched()
		time.Sleep(5 * time.Millisecond)
	}
	if got := runtime.NumGoroutine(); got > before {
		buf := make([]byte, 1<<16)
		n := runtime.Stack(buf, true)
		t.Fatalf("serve leaked goroutines: before=%d after=%d (serve err=%v)\n%s", before, got, serveErr, buf[:n])
	}
}

func TestServeWithAuth(t *testing.T) {
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
	eff := config.Resolve(config.File{
		Rules:  []string{rulesDir},
		Store:  config.StoreConfig{Kind: "sqlite", Path: filepath.Join(dir, "d.db")},
		Engine: config.EngineConfig{SigningKey: filepath.Join(dir, "k.key")},
	}, true, config.FlagOverrides{}, func(string) string { return "" })
	e, l, m, cleanup, err := bootstrap.BuildEngine(eff, io.Discard, slog.New(slog.DiscardHandler))
	if err != nil {
		t.Fatal(err)
	}
	defer cleanup()
	authz := ee.NewAuthorizer(map[string][]string{"secret": {"ingest:write"}})

	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() { done <- serve(ctx, ln, e, l, m, authz, time.Hour, slog.New(slog.DiscardHandler)) }()
	base := "http://" + ln.Addr().String()

	var resp *http.Response
	for i := 0; i < 100; i++ {
		resp, err = http.Get(base + "/healthz") // public even with auth on
		if err == nil {
			break
		}
		time.Sleep(10 * time.Millisecond)
	}
	if err != nil || resp.StatusCode != http.StatusOK {
		t.Fatalf("healthz should be public: %v", err)
	}

	body := `{"type":"cost.budget_burn","correlation_key":"acme:cost","subject":{"alias":"gpt-4o"},"data":{"spend_1h_usd":9.0}}`
	resp, err = http.Post(base+"/v1/signals", "application/json", strings.NewReader(body))
	if err != nil || resp.StatusCode != http.StatusForbidden {
		t.Fatalf("unauthenticated signal should be 403, got %v %d", err, resp.StatusCode)
	}

	req, _ := http.NewRequest("POST", base+"/v1/signals", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-API-Key", "secret")
	resp, err = http.DefaultClient.Do(req)
	if err != nil || resp.StatusCode != http.StatusOK {
		t.Fatalf("authenticated signal should be 200, got %v %d", err, resp.StatusCode)
	}

	cancel()
	select {
	case <-done:
	case <-time.After(5 * time.Second):
		t.Fatal("serve did not shut down")
	}
}
