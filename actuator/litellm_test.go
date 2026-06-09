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
