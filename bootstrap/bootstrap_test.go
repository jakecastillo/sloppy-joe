package bootstrap

import (
	"context"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"github.com/sloppyjoe/sloppy/config"
	"github.com/sloppyjoe/sloppy/core"
)

// recordingServer returns an httptest server that flips *hit on any request.
func recordingServer(hit *bool) *httptest.Server {
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		*hit = true
		w.WriteHeader(http.StatusOK)
	}))
}

func TestBuildRegistryWiresGithub(t *testing.T) {
	var hit bool
	srv := recordingServer(&hit)
	defer srv.Close()
	t.Setenv("SLOPPY_TOKEN_GITHUB", "tok")
	eff := config.Resolve(config.File{Platforms: map[string]config.Platform{
		"github": {Enabled: true, BaseURL: srv.URL, TokenEnv: "SLOPPY_TOKEN_GITHUB"},
	}}, true, config.FlagOverrides{}, os.Getenv)
	reg, err := BuildRegistry(eff, io.Discard)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := reg.Apply(context.Background(), core.RemediationIntent{
		ID: "i1", Kind: core.ActionOpenIssue, Target: "gpt-4o", Args: map[string]any{"repo": "o/r"},
	}); err != nil {
		t.Fatalf("apply open_issue: %v", err)
	}
	if !hit {
		t.Fatal("github enabled should wire the github actuator (open_issue routed to it, not Log)")
	}
}

// Enabling the webhook + cloudflare platforms must wire their actuators (route_override
// to webhook, throttle_tenant to cloudflare) with broker-scoped tokens — not fall to Log.
func TestBuildRegistryWiresWebhookAndCloudflare(t *testing.T) {
	var whHit, cfHit bool
	wh := recordingServer(&whHit)
	defer wh.Close()
	cf := recordingServer(&cfHit)
	defer cf.Close()
	t.Setenv("SLOPPY_TOKEN_WEBHOOK", "tok")
	t.Setenv("SLOPPY_TOKEN_CLOUDFLARE", "tok")
	eff := config.Resolve(config.File{Platforms: map[string]config.Platform{
		"webhook":    {Enabled: true, URL: wh.URL, TokenEnv: "SLOPPY_TOKEN_WEBHOOK"},
		"cloudflare": {Enabled: true, URL: cf.URL, TokenEnv: "SLOPPY_TOKEN_CLOUDFLARE"},
	}}, true, config.FlagOverrides{}, os.Getenv)
	reg, err := BuildRegistry(eff, io.Discard)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := reg.Apply(context.Background(), core.RemediationIntent{
		ID: "i1", Kind: core.ActionRouteOverride, Target: "gpt-4o", Args: map[string]any{"to": "ollama/llama3"},
	}); err != nil {
		t.Fatalf("apply route_override: %v", err)
	}
	if !whHit {
		t.Fatal("webhook enabled should wire the webhook actuator (route_override)")
	}
	if _, err := reg.Apply(context.Background(), core.RemediationIntent{
		ID: "i2", Kind: core.ActionThrottleTenant, Target: "acme",
	}); err != nil {
		t.Fatalf("apply throttle_tenant: %v", err)
	}
	if !cfHit {
		t.Fatal("cloudflare enabled should wire the cloudflare actuator (throttle_tenant)")
	}
}

// Backward-compatibility: zero-config + the legacy SLOPPY_LITELLM_URL env must still
// wire the LiteLLM actuator exactly as before the config layer existed.
func TestBuildRegistryTransitionWiresLegacyLitellm(t *testing.T) {
	var hit bool
	srv := recordingServer(&hit)
	defer srv.Close()
	t.Setenv("SLOPPY_LITELLM_URL", srv.URL)
	t.Setenv("SLOPPY_TOKEN_LITELLM", "tok")
	eff := config.Resolve(config.Defaults(), false, config.FlagOverrides{}, os.Getenv)
	reg, err := BuildRegistry(eff, io.Discard)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := reg.Apply(context.Background(), core.RemediationIntent{
		ID: "i1", Kind: core.ActionRouteOverride, Target: "gpt-4o", Args: map[string]any{"to": "ollama/llama3"},
	}); err != nil {
		t.Fatalf("apply route_override: %v", err)
	}
	if !hit {
		t.Fatal("legacy SLOPPY_LITELLM_URL should wire the litellm actuator (transition compat)")
	}
}

func TestBuildEngineSmoke(t *testing.T) {
	dir := t.TempDir()
	rulesDir := filepath.Join(dir, "rules")
	if err := os.MkdirAll(rulesDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(rulesDir, "r.yaml"),
		[]byte("on: x\nwhen: \"true\"\nthen: [ { page: {} } ]\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	eff := config.Resolve(config.File{
		Rules:  []string{rulesDir},
		Store:  config.StoreConfig{Kind: "sqlite", Path: filepath.Join(dir, "s.db")},
		Engine: config.EngineConfig{SigningKey: filepath.Join(dir, "k.key")},
	}, true, config.FlagOverrides{}, func(string) string { return "" })
	e, l, m, cleanup, err := BuildEngine(eff, io.Discard, slog.New(slog.DiscardHandler))
	if err != nil {
		t.Fatal(err)
	}
	defer cleanup()
	if e == nil || l == nil || m == nil {
		t.Fatal("BuildEngine returned a nil component")
	}
}
