package actuator

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/sloppyjoe/sloppy/core"
)

func TestCloudflareCapabilities(t *testing.T) {
	a := NewCloudflare("http://example.invalid", func() (string, error) { return "tok", nil })
	caps := a.Capabilities()
	if len(caps) != 2 {
		t.Fatalf("expected exactly throttle_tenant + disable_deployment, got %v", caps)
	}
	got := map[core.ActionKind]bool{}
	for _, k := range caps {
		got[k] = true
	}
	if !got[core.ActionThrottleTenant] || !got[core.ActionDisableDeployment] {
		t.Fatalf("capabilities must be throttle_tenant + disable_deployment, got %v", caps)
	}
	if got[core.ActionRouteOverride] {
		t.Fatalf("cloudflare must NOT advertise route_override, got %v", caps)
	}
}

func TestCloudflareApplyRevertPUT(t *testing.T) {
	var gotMethod, gotPath, gotAuth, gotContentType string
	var gotBody map[string]any
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotMethod = r.Method
		gotPath = r.URL.Path
		gotAuth = r.Header.Get("Authorization")
		gotContentType = r.Header.Get("Content-Type")
		_ = json.NewDecoder(r.Body).Decode(&gotBody)
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"success":true}`))
	}))
	defer srv.Close()

	base := srv.URL + "/accounts/acct-123/ai-gateway/gateways"
	a := NewCloudflare(base, func() (string, error) { return "cf-token", nil })
	i := core.RemediationIntent{
		ID: "int-1", Kind: core.ActionThrottleTenant, Target: "gw-abc",
		Args: map[string]any{"prior_limit": 250},
	}

	// Apply: PUT /accounts/acct-123/ai-gateway/gateways/gw-abc, limit 0.
	rcpt, err := a.Apply(context.Background(), i)
	if err != nil || rcpt.Outcome != core.OutcomeApplied {
		t.Fatalf("apply: %+v err=%v", rcpt, err)
	}
	if gotMethod != http.MethodPut {
		t.Fatalf("expected PUT, got %q", gotMethod)
	}
	if gotPath != "/accounts/acct-123/ai-gateway/gateways/gw-abc" {
		t.Fatalf("bad path, got %q", gotPath)
	}
	if gotAuth != "Bearer cf-token" {
		t.Fatalf("missing/incorrect auth: %q", gotAuth)
	}
	if gotContentType != "application/json" {
		t.Fatalf("expected application/json content-type, got %q", gotContentType)
	}
	if gotBody["rate_limiting_limit"].(float64) != 0 {
		t.Fatalf("apply must pin rate_limiting_limit=0, got %+v", gotBody)
	}
	if gotBody["rate_limiting_technique"] != "fixed" {
		t.Fatalf("expected rate_limiting_technique=fixed, got %+v", gotBody)
	}
	if _, ok := gotBody["rate_limiting_interval"]; !ok {
		t.Fatalf("expected rate_limiting_interval present, got %+v", gotBody)
	}

	// Revert: restore the prior limit from Args.
	rcpt, err = a.Revert(context.Background(), i)
	if err != nil || rcpt.Outcome != core.OutcomeReverted {
		t.Fatalf("revert: %+v err=%v", rcpt, err)
	}
	if gotMethod != http.MethodPut {
		t.Fatalf("revert expected PUT, got %q", gotMethod)
	}
	if gotBody["rate_limiting_limit"].(float64) != 250 {
		t.Fatalf("revert must restore prior_limit=250, got %+v", gotBody)
	}
}

func TestCloudflareRevertDefaultLimit(t *testing.T) {
	var gotBody map[string]any
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewDecoder(r.Body).Decode(&gotBody)
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()
	a := NewCloudflare(srv.URL+"/accounts/a/ai-gateway/gateways", func() (string, error) { return "tok", nil })

	// No prior_limit in Args -> revert falls back to the default limit.
	i := core.RemediationIntent{ID: "d1", Kind: core.ActionDisableDeployment, Target: "gw-z"}
	if _, err := a.Revert(context.Background(), i); err != nil {
		t.Fatal(err)
	}
	if gotBody["rate_limiting_limit"].(float64) != float64(defaultCloudflareLimit) {
		t.Fatalf("revert without prior_limit should use default %d, got %+v", defaultCloudflareLimit, gotBody)
	}
}

func TestCloudflareApplyServerError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer srv.Close()
	a := NewCloudflare(srv.URL+"/accounts/a/ai-gateway/gateways", func() (string, error) { return "tok", nil })
	r, err := a.Apply(context.Background(), core.RemediationIntent{ID: "x", Kind: core.ActionThrottleTenant, Target: "gw"})
	if err == nil || r.Outcome != core.OutcomeFailed {
		t.Fatalf("expected failure on 500, got %+v err=%v", r, err)
	}
}

func TestCloudflareConformance(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()
	a := NewCloudflare(srv.URL+"/accounts/a/ai-gateway/gateways", func() (string, error) { return "tok", nil })
	Conformance(t, a, core.RemediationIntent{ID: "c1", Target: "gw-1"})
}
