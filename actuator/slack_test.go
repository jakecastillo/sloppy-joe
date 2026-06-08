package actuator

import (
	"context"
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
	a := NewSlack(srv.URL)
	r, err := a.Apply(context.Background(), core.RemediationIntent{ID: "int-3", Kind: core.ActionPage,
		Args: map[string]any{"slack": "#oncall"}})
	if err != nil || r.Outcome != core.OutcomeApplied || !hit {
		t.Fatalf("slack apply failed: %+v err=%v hit=%v", r, err, hit)
	}
}
