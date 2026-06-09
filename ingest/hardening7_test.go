package ingest

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/sloppyjoe/sloppy/ee"
)

// Guard: every route the ingest mux registers must have an explicit, non-deny
// ee.ScopeForPath mapping. A route added to the mux but forgotten in ScopeForPath
// would otherwise fail closed (ScopeDeny) — this test makes that omission a build
// failure instead of a silent unreachable endpoint, and keeps ScopeForPath the
// single scope source without exporting a route table into ee.
func TestEveryRouteHasExplicitScope(t *testing.T) {
	for _, path := range Routes {
		if scope := ee.ScopeForPath(path); scope == ee.ScopeDeny {
			t.Fatalf("route %q registered by the ingest mux has no explicit scope (got ScopeDeny); add it to ee.ScopeForPath", path)
		}
	}
	// And a route NOT registered must fail closed.
	if ee.ScopeForPath("/v1/route-the-mux-does-not-have") != ee.ScopeDeny {
		t.Fatal("an unregistered route must map to ScopeDeny (fail closed)")
	}
}

// Over-limit bodies on the mutating ingest handlers must be rejected (413), not
// buffered unbounded into memory.
func TestBodyCapsRejectOversizeRequests(t *testing.T) {
	s, _ := testServer(t)
	srv := httptest.NewServer(s.Handler())
	defer srv.Close()

	cases := []struct {
		path string
		size int
	}{
		{"/v1/signals", maxSignalBytes + 1024},
		{"/v1/usage", maxUsageBytes + 1024},
		{"/v1/otlp/metrics", maxOTLPBytes + 1024},
	}
	for _, c := range cases {
		// A valid-looking JSON prefix followed by junk padding so the read, not the
		// parse, is what trips: the body exceeds the cap before decoding completes.
		body := `{"junk":"` + strings.Repeat("A", c.size) + `"}`
		resp, err := http.Post(srv.URL+c.path, "application/json", strings.NewReader(body))
		if err != nil {
			t.Fatalf("%s: post error: %v", c.path, err)
		}
		resp.Body.Close()
		if resp.StatusCode != http.StatusRequestEntityTooLarge {
			t.Fatalf("%s: oversize body must be 413, got %d", c.path, resp.StatusCode)
		}
	}
}

// Within-limit malformed JSON stays a 400 (client parse fault), distinct from the
// 413 over-limit fault — the cap must not turn ordinary bad requests into 413.
func TestBodyCapWithinLimitStillParses(t *testing.T) {
	s, _ := testServer(t)
	srv := httptest.NewServer(s.Handler())
	defer srv.Close()

	resp, err := http.Post(srv.URL+"/v1/signals", "application/json", strings.NewReader("{not json"))
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("small malformed body must be 400, got %d", resp.StatusCode)
	}
}
