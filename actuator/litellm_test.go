package actuator

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/sloppyjoe/sloppy/core"
)

func TestLiteLLMRouteOverrideApplyRevert(t *testing.T) {
	var gotPath, gotAuth string
	var gotBody map[string]any
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		gotAuth = r.Header.Get("Authorization")
		_ = json.NewDecoder(r.Body).Decode(&gotBody)
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"ok":true}`))
	}))
	defer srv.Close()

	a := NewLiteLLM(srv.URL, func() (string, error) { return "admin-xyz", nil })
	i := core.RemediationIntent{
		ID: "int-1", Kind: core.ActionRouteOverride, Target: "gpt-4o",
		Args: map[string]any{"to": "ollama/llama3"},
	}

	rcpt, err := a.Apply(context.Background(), i)
	if err != nil || rcpt.Outcome != core.OutcomeApplied {
		t.Fatalf("apply: %+v err=%v", rcpt, err)
	}
	if gotAuth != "Bearer admin-xyz" {
		t.Fatalf("missing/incorrect auth: %q", gotAuth)
	}
	if gotPath != "/model/update" || gotBody["model_name"] != "gpt-4o" {
		t.Fatalf("bad request path=%q body=%+v", gotPath, gotBody)
	}
	lp, _ := gotBody["litellm_params"].(map[string]any)
	if lp == nil || lp["model"] != "ollama/llama3" {
		t.Fatalf("litellm_params.model should be the override dest, got %+v", gotBody["litellm_params"])
	}

	rcpt, err = a.Revert(context.Background(), i)
	if err != nil || rcpt.Outcome != core.OutcomeReverted {
		t.Fatalf("revert: %+v err=%v", rcpt, err)
	}
	lp, _ = gotBody["litellm_params"].(map[string]any)
	if lp["model"] != "gpt-4o" {
		t.Fatalf("revert should restore self-route, got %+v", gotBody["litellm_params"])
	}
}

func TestLiteLLMApplyServerError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer srv.Close()
	a := NewLiteLLM(srv.URL, func() (string, error) { return "tok", nil })
	r, err := a.Apply(context.Background(), core.RemediationIntent{ID: "x", Kind: core.ActionRouteOverride, Target: "m", Args: map[string]any{"to": "n"}})
	if err == nil || r.Outcome != core.OutcomeFailed {
		t.Fatalf("expected failure on 500, got %+v err=%v", r, err)
	}
}

func TestLiteLLMThrottleAndDisable(t *testing.T) {
	var gotPath string
	var gotBody map[string]any
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		_ = json.NewDecoder(r.Body).Decode(&gotBody)
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()
	a := NewLiteLLM(srv.URL, func() (string, error) { return "tok", nil })

	// throttle_tenant -> /key/update, rpm_limit 0 on apply, -1 (unlimited) on revert.
	thr := core.RemediationIntent{ID: "t1", Kind: core.ActionThrottleTenant, Target: "acme"}
	if _, err := a.Apply(context.Background(), thr); err != nil {
		t.Fatal(err)
	}
	if gotPath != "/key/update" || gotBody["key"] != "acme" || gotBody["rpm_limit"].(float64) != 0 {
		t.Fatalf("throttle apply: path=%s body=%+v", gotPath, gotBody)
	}
	if _, err := a.Revert(context.Background(), thr); err != nil {
		t.Fatal(err)
	}
	if gotBody["rpm_limit"].(float64) != -1 {
		t.Fatalf("throttle revert should restore (rpm_limit -1), got %+v", gotBody)
	}

	// disable_deployment -> /model/update, model_info.disabled true on apply, false on revert.
	dis := core.RemediationIntent{ID: "d1", Kind: core.ActionDisableDeployment, Target: "gpt-4o"}
	if _, err := a.Apply(context.Background(), dis); err != nil {
		t.Fatal(err)
	}
	mi, _ := gotBody["model_info"].(map[string]any)
	if gotPath != "/model/update" || mi == nil || mi["disabled"] != true {
		t.Fatalf("disable apply: path=%s body=%+v", gotPath, gotBody)
	}
	if _, err := a.Revert(context.Background(), dis); err != nil {
		t.Fatal(err)
	}
	mi, _ = gotBody["model_info"].(map[string]any)
	if mi["disabled"] != false {
		t.Fatalf("disable revert should re-enable, got %+v", gotBody)
	}
}

func TestLiteLLMConformance(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()
	a := NewLiteLLM(srv.URL, func() (string, error) { return "tok", nil })
	Conformance(t, a, core.RemediationIntent{ID: "c1", Target: "gpt-4o", Args: map[string]any{"to": "ollama/llama3"}})
}
