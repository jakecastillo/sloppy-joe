package actuator

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/sloppyjoe/sloppy/core"
)

func TestSlackPage(t *testing.T) {
	hit := false
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hit = true
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()
	// The webhook URL is resolved through a TokenFunc (the broker), never inline.
	a := NewSlack(func() (string, error) { return srv.URL, nil })
	r, err := a.Apply(context.Background(), core.RemediationIntent{ID: "int-3", Kind: core.ActionPage,
		Args: map[string]any{"slack": "#oncall"}})
	if err != nil || r.Outcome != core.OutcomeApplied || !hit {
		t.Fatalf("slack apply failed: %+v err=%v hit=%v", r, err, hit)
	}
}

func TestSlackTokenError(t *testing.T) {
	// If the broker can't resolve the webhook, Apply fails closed (no post).
	a := NewSlack(func() (string, error) { return "", fmt.Errorf("no token") })
	r, err := a.Apply(context.Background(), core.RemediationIntent{ID: "int-4", Kind: core.ActionPage,
		Args: map[string]any{"slack": "#oncall"}})
	if err == nil || r.Outcome != core.OutcomeFailed {
		t.Fatalf("expected failure when token unresolved: %+v err=%v", r, err)
	}
}
