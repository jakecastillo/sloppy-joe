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
