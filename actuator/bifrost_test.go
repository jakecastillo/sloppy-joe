package actuator

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/sloppyjoe/sloppy/core"
)

func TestBifrostRouteOverrideConformance(t *testing.T) {
	var path, auth string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		path = r.URL.Path
		auth = r.Header.Get("Authorization")
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()
	a := NewBifrost(srv.URL, func() (string, error) { return "tok", nil })
	Conformance(t, a, core.RemediationIntent{ID: "b1", Target: "gpt-4o", Args: map[string]any{"to": "ollama/llama3"}})
	if path != "/api/route/override" {
		t.Fatalf("bifrost path: %q", path)
	}
	if auth != "Bearer tok" {
		t.Fatalf("bifrost auth: %q", auth)
	}
}
