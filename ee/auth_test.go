package ee

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

func okHandler() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("ok"))
	})
}

func TestMiddlewareScopeEnforcement(t *testing.T) {
	a := NewAuthorizer(map[string][]string{"k1": {"ingest:write"}, "k2": {"status:read"}})
	srv := httptest.NewServer(a.Middleware(okHandler()))
	defer srv.Close()

	cases := []struct {
		path, key string
		want      int
	}{
		{"/healthz", "", http.StatusOK},           // public
		{"/v1/signals", "", http.StatusForbidden}, // no key
		{"/v1/signals", "k1", http.StatusOK},      // has ingest:write
		{"/v1/signals", "k2", http.StatusForbidden},
		{"/status", "k2", http.StatusOK}, // has status:read
		{"/status", "k1", http.StatusForbidden},
	}
	for _, c := range cases {
		req, _ := http.NewRequest("GET", srv.URL+c.path, nil)
		if c.key != "" {
			req.Header.Set("X-API-Key", c.key)
		}
		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Fatal(err)
		}
		resp.Body.Close()
		if resp.StatusCode != c.want {
			t.Fatalf("%s key=%q: want %d got %d", c.path, c.key, c.want, resp.StatusCode)
		}
	}
}

// Fail-closed default: an unmapped route resolves to the deny sentinel, which no
// key can satisfy — replacing the old fail-open ingest:write default that handed
// every unknown route (including future billing writes) to any ingest:write key.
func TestScopeForPathFailsClosedForUnmappedRoute(t *testing.T) {
	if got := ScopeForPath("/v1/billing/charge"); got != ScopeDeny {
		t.Fatalf("unmapped route must map to ScopeDeny, got %q", got)
	}
	if got := ScopeForPath("/totally/unknown"); got != ScopeDeny {
		t.Fatalf("unknown route must map to ScopeDeny, got %q", got)
	}
	// Known routes keep their grantable scopes.
	for path, want := range map[string]string{
		"/healthz":         ScopePublic,
		"/status":          ScopeStatusRead,
		"/v1/signals":      ScopeIngestWrite,
		"/v1/usage":        ScopeIngestWrite,
		"/v1/otlp/metrics": ScopeIngestWrite,
	} {
		if got := ScopeForPath(path); got != want {
			t.Fatalf("ScopeForPath(%q)=%q, want %q", path, got, want)
		}
	}
}

// No key — not even one granted the literal sentinel value — can satisfy ScopeDeny.
func TestDenyScopeUnsatisfiable(t *testing.T) {
	a := NewAuthorizer(map[string][]string{
		"super":  {ScopeIngestWrite, ScopeStatusRead},
		"sneaky": {ScopeDeny}, // even a key literally granted "deny:all" is denied
	})
	if a.HasScope("super", ScopeDeny) {
		t.Fatal("a fully-scoped key must NOT satisfy ScopeDeny")
	}
	if a.HasScope("sneaky", ScopeDeny) {
		t.Fatal("ScopeDeny is hard-denied even if literally present in a key set")
	}
}

// An unmapped route is 403 for every caller, regardless of key, through the mux.
func TestMiddlewareDeniesUnmappedRoute(t *testing.T) {
	a := NewAuthorizer(map[string][]string{"k1": {ScopeIngestWrite, ScopeStatusRead}})
	srv := httptest.NewServer(a.Middleware(okHandler()))
	defer srv.Close()

	req, _ := http.NewRequest("POST", srv.URL+"/v1/billing/charge", nil)
	req.Header.Set("X-API-Key", "k1")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusForbidden {
		t.Fatalf("unmapped billing write must be 403 even for a write key, got %d", resp.StatusCode)
	}
}

func TestKeyCount(t *testing.T) {
	if (*Authorizer)(nil).KeyCount() != 0 {
		t.Fatal("nil authorizer must report 0 keys")
	}
	a := NewAuthorizer(map[string][]string{"a": {ScopeStatusRead}, "b": {ScopeIngestWrite}})
	if a.KeyCount() != 2 {
		t.Fatalf("KeyCount=%d, want 2", a.KeyCount())
	}
	if NewAuthorizer(nil).KeyCount() != 0 {
		t.Fatal("empty authorizer must report 0 keys")
	}
}

func TestLoadFromEnv(t *testing.T) {
	t.Setenv("SLOPPY_API_KEYS", "k1=ingest:write,status:read;k2=status:read")
	a := LoadFromEnv()
	if !a.HasScope("k1", "ingest:write") || !a.HasScope("k1", "status:read") {
		t.Fatal("k1 should have both scopes")
	}
	if a.HasScope("k2", "ingest:write") {
		t.Fatal("k2 must not have ingest:write")
	}
	if a.HasScope("unknown", "status:read") {
		t.Fatal("unknown key must be denied")
	}
	// Bearer form also works.
	if !a.HasScope("k2", "status:read") {
		t.Fatal("k2 should have status:read")
	}
}
