package actuator

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/sloppyjoe/sloppy/core"
)

func TestPortkeyCapabilities(t *testing.T) {
	a := NewPortkey("http://example.invalid", func() (string, error) { return "tok", nil })
	caps := a.Capabilities()
	if len(caps) != 2 {
		t.Fatalf("expected exactly route_override + disable_deployment, got %v", caps)
	}
	got := map[core.ActionKind]bool{}
	for _, k := range caps {
		got[k] = true
	}
	if !got[core.ActionRouteOverride] || !got[core.ActionDisableDeployment] {
		t.Fatalf("capabilities must be route_override + disable_deployment, got %v", caps)
	}
	if got[core.ActionThrottleTenant] {
		t.Fatalf("portkey must NOT advertise throttle_tenant, got %v", caps)
	}
}

func TestPortkeyApplyRevertPUT(t *testing.T) {
	var gotMethod, gotPath, gotKey, gotAuth, gotContentType string
	var gotBody map[string]any
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotMethod = r.Method
		gotPath = r.URL.Path
		gotKey = r.Header.Get("x-portkey-api-key")
		gotAuth = r.Header.Get("Authorization")
		gotContentType = r.Header.Get("Content-Type")
		_ = json.NewDecoder(r.Body).Decode(&gotBody)
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"success":true}`))
	}))
	defer srv.Close()

	base := srv.URL + "/v1/configs"
	a := NewPortkey(base, func() (string, error) { return "pk-token", nil })
	priorConfig := map[string]any{"strategy": map[string]any{"mode": "loadbalance"}}
	i := core.RemediationIntent{
		ID: "int-1", Kind: core.ActionRouteOverride, Target: "cfg-abc",
		Args: map[string]any{"to": "gpt-4o-mini", "prior_config": priorConfig},
	}

	// Apply: PUT /v1/configs/cfg-abc, status inactive, routing overridden to args["to"].
	rcpt, err := a.Apply(context.Background(), i)
	if err != nil || rcpt.Outcome != core.OutcomeApplied {
		t.Fatalf("apply: %+v err=%v", rcpt, err)
	}
	if gotMethod != http.MethodPut {
		t.Fatalf("expected PUT, got %q", gotMethod)
	}
	if gotPath != "/v1/configs/cfg-abc" {
		t.Fatalf("bad path, got %q", gotPath)
	}
	if gotKey != "pk-token" {
		t.Fatalf("missing/incorrect x-portkey-api-key: %q", gotKey)
	}
	if gotAuth != "" {
		t.Fatalf("portkey must NOT send Authorization: Bearer, got %q", gotAuth)
	}
	if gotContentType != "application/json" {
		t.Fatalf("expected application/json content-type, got %q", gotContentType)
	}
	if gotBody["status"] != portkeyStatusInactive {
		t.Fatalf("apply must pin status=%q, got %+v", portkeyStatusInactive, gotBody)
	}
	if _, ok := gotBody["config"].(map[string]any); !ok {
		t.Fatalf("apply must carry an overridden config object, got %+v", gotBody)
	}
	if rcpt.After != "gpt-4o-mini" {
		t.Fatalf("receipt After should record route destination, got %+v", rcpt)
	}

	// Revert: restore the prior config from Args and status=active.
	rcpt, err = a.Revert(context.Background(), i)
	if err != nil || rcpt.Outcome != core.OutcomeReverted {
		t.Fatalf("revert: %+v err=%v", rcpt, err)
	}
	if gotMethod != http.MethodPut {
		t.Fatalf("revert expected PUT, got %q", gotMethod)
	}
	if gotBody["status"] != portkeyStatusActive {
		t.Fatalf("revert must restore status=%q, got %+v", portkeyStatusActive, gotBody)
	}
	rc, ok := gotBody["config"].(map[string]any)
	if !ok {
		t.Fatalf("revert must restore prior config object, got %+v", gotBody)
	}
	strategy, ok := rc["strategy"].(map[string]any)
	if !ok || strategy["mode"] != "loadbalance" {
		t.Fatalf("revert must restore prior_config verbatim, got %+v", rc)
	}
}

func TestPortkeyRevertWithoutPriorConfig(t *testing.T) {
	var gotBody map[string]any
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewDecoder(r.Body).Decode(&gotBody)
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()
	a := NewPortkey(srv.URL+"/v1/configs", func() (string, error) { return "tok", nil })

	// No prior_config in Args -> revert still reactivates (status=active) without
	// clobbering the config (omitted, so the stored config is left untouched).
	i := core.RemediationIntent{ID: "d1", Kind: core.ActionDisableDeployment, Target: "cfg-z"}
	if _, err := a.Revert(context.Background(), i); err != nil {
		t.Fatal(err)
	}
	if gotBody["status"] != portkeyStatusActive {
		t.Fatalf("revert without prior_config should still set status=active, got %+v", gotBody)
	}
	if _, ok := gotBody["config"]; ok {
		t.Fatalf("revert without prior_config must omit config (no clobber), got %+v", gotBody)
	}
}

func TestPortkeyApplyServerError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer srv.Close()
	a := NewPortkey(srv.URL+"/v1/configs", func() (string, error) { return "tok", nil })
	r, err := a.Apply(context.Background(), core.RemediationIntent{ID: "x", Kind: core.ActionRouteOverride, Target: "cfg"})
	if err == nil || r.Outcome != core.OutcomeFailed {
		t.Fatalf("expected failure on 500, got %+v err=%v", r, err)
	}
}

func TestPortkeyConformance(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()
	a := NewPortkey(srv.URL+"/v1/configs", func() (string, error) { return "tok", nil })
	Conformance(t, a, core.RemediationIntent{ID: "c1", Target: "cfg-1"})
}
