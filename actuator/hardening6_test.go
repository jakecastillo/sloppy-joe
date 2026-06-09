package actuator

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/sloppyjoe/sloppy/core"
)

// TestGitHubTokenError pins the fail-closed token branch: if the broker can't
// resolve the GitHub token, Apply returns OutcomeFailed + an error and never posts
// (parity with the existing Slack token-error test).
func TestGitHubTokenError(t *testing.T) {
	posted := false
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		posted = true
		w.WriteHeader(http.StatusCreated)
	}))
	defer srv.Close()

	a := NewGitHub(srv.URL, func() (string, error) { return "", fmt.Errorf("no token") })
	r, err := a.Apply(context.Background(), core.RemediationIntent{
		ID: "gh-tok", Kind: core.ActionOpenIssue,
		Target: "gpt-4o", Args: map[string]any{"repo": "acme/ops"},
	})
	if err == nil {
		t.Fatal("expected an error when the token cannot be resolved")
	}
	if r.Outcome != core.OutcomeFailed {
		t.Fatalf("expected OutcomeFailed on token error, got %v", r.Outcome)
	}
	if r.Actuator != "github" {
		t.Fatalf("receipt must name the github actuator even on failure, got %q", r.Actuator)
	}
	if posted {
		t.Fatal("Apply must NOT post when the token is unresolved (fail closed before any request)")
	}
}

// TestGitHubApplyServerError pins the non-2xx branch: a 5xx from the GitHub API
// surfaces as OutcomeFailed + a non-nil error (the issue was not opened).
func TestGitHubApplyServerError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer srv.Close()

	a := NewGitHub(srv.URL, func() (string, error) { return "gh-tok", nil })
	r, err := a.Apply(context.Background(), core.RemediationIntent{
		ID: "gh-5xx", Kind: core.ActionOpenIssue,
		Target: "gpt-4o", Args: map[string]any{"repo": "acme/ops"},
	})
	if err == nil || r.Outcome != core.OutcomeFailed {
		t.Fatalf("expected OutcomeFailed+error on 500, got %+v err=%v", r, err)
	}
}

// TestGitHubApplyClientError pins that a 4xx (e.g. 401 bad credentials / 404 repo)
// is also a failure, not a silent success.
func TestGitHubApplyClientError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
	}))
	defer srv.Close()

	a := NewGitHub(srv.URL, func() (string, error) { return "bad-tok", nil })
	r, err := a.Apply(context.Background(), core.RemediationIntent{
		ID: "gh-4xx", Kind: core.ActionOpenIssue,
		Target: "gpt-4o", Args: map[string]any{"repo": "acme/ops"},
	})
	if err == nil || r.Outcome != core.OutcomeFailed {
		t.Fatalf("expected OutcomeFailed+error on 401, got %+v err=%v", r, err)
	}
}

// TestSlackApplyServerError pins Slack's non-2xx branch: a 5xx from the webhook
// surfaces as OutcomeFailed + a non-nil error (the page was not delivered).
func TestSlackApplyServerError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer srv.Close()

	a := NewSlack(func() (string, error) { return srv.URL, nil })
	r, err := a.Apply(context.Background(), core.RemediationIntent{
		ID: "sl-5xx", Kind: core.ActionPage,
		Args: map[string]any{"slack": "#oncall"},
	})
	if err == nil || r.Outcome != core.OutcomeFailed {
		t.Fatalf("expected OutcomeFailed+error on 500, got %+v err=%v", r, err)
	}
}

// TestSlackApplyClientError pins Slack's 4xx branch (e.g. a revoked webhook URL
// returning 404) as a failure, not a silent success.
func TestSlackApplyClientError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNotFound)
	}))
	defer srv.Close()

	a := NewSlack(func() (string, error) { return srv.URL, nil })
	r, err := a.Apply(context.Background(), core.RemediationIntent{
		ID: "sl-4xx", Kind: core.ActionPage,
		Args: map[string]any{"slack": "#oncall"},
	})
	if err == nil || r.Outcome != core.OutcomeFailed {
		t.Fatalf("expected OutcomeFailed+error on 404, got %+v err=%v", r, err)
	}
}
