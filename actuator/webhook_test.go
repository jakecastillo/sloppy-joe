package actuator

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/sloppyjoe/sloppy/core"
)

func TestWebhookRouteOverride(t *testing.T) {
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

	a := NewWebhook(srv.URL, func() (string, error) { return "admin-xyz", nil })
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
	if gotPath != "/route" {
		t.Fatalf("expected path /route, got %q", gotPath)
	}
	if gotBody["model"] != "gpt-4o" {
		t.Fatalf("body.model should be the target, got %+v", gotBody["model"])
	}
	if gotBody["to"] != "ollama/llama3" {
		t.Fatalf("body.to should be the override dest, got %+v", gotBody["to"])
	}

	rcpt, err = a.Revert(context.Background(), i)
	if err != nil || rcpt.Outcome != core.OutcomeReverted {
		t.Fatalf("revert: %+v err=%v", rcpt, err)
	}
	if gotPath != "/route" {
		t.Fatalf("revert path should be /route, got %q", gotPath)
	}
	if gotBody["to"] != "gpt-4o" {
		t.Fatalf("revert should restore self-route (body.to=target), got %+v", gotBody["to"])
	}
}

func TestWebhookConformance(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()
	a := NewWebhook(srv.URL, func() (string, error) { return "tok", nil })
	Conformance(t, a, core.RemediationIntent{ID: "c1", Target: "gpt-4o", Args: map[string]any{"to": "ollama/llama3"}})
}
