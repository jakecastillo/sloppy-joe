package actuator

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/sloppyjoe/sloppy/core"
)

func TestEnvoyRouteOverrideConformance(t *testing.T) {
	var path string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		path = r.URL.Path
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()
	a := NewEnvoy(srv.URL, func() (string, error) { return "tok", nil })
	Conformance(t, a, core.RemediationIntent{ID: "e1", Target: "gpt-4o", Args: map[string]any{"to": "ollama/llama3"}})
	if path != "/admin/routes/override" {
		t.Fatalf("envoy path: %q", path)
	}
}
