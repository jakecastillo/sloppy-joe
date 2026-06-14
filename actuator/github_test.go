package actuator

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/sloppyjoe/sloppy/core"
)

func TestGitHubOpenIssue(t *testing.T) {
	var body map[string]any
	var path string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		path = r.URL.Path
		_ = json.NewDecoder(r.Body).Decode(&body)
		w.WriteHeader(http.StatusCreated)
		_, _ = w.Write([]byte(`{"number":42}`))
	}))
	defer srv.Close()
	a := NewGitHub(srv.URL, func() (string, error) { return "gh-tok", nil })
	i := core.RemediationIntent{
		ID: "int-2", Kind: core.ActionOpenIssue, RuleSHA: "sha9",
		Target: "gpt-4o", Args: map[string]any{"repo": "acme/ops"},
	}
	r, err := a.Apply(context.Background(), i)
	if err != nil || r.Outcome != core.OutcomeApplied {
		t.Fatalf("apply: %+v err=%v", r, err)
	}
	if path != "/repos/acme/ops/issues" {
		t.Fatalf("bad path: %q", path)
	}
	if body["title"] == nil {
		t.Fatal("issue must have a title")
	}
	if !strings.Contains(body["body"].(string), "sha9") {
		t.Fatalf("issue body must carry rule SHA provenance: %v", body["body"])
	}
}
