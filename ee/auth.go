// Package ee holds optional enterprise features (the open-core boundary).
// v0: API-key → scope RBAC for the HTTP API.
package ee

import (
	"net/http"
	"os"
	"strings"
)

// Scope constants name the grantable RBAC scopes plus the deny sentinel.
const (
	// ScopeIngestWrite gates the mutating ingest surface (signals + usage/OTLP).
	ScopeIngestWrite = "ingest:write"
	// ScopeStatusRead gates the read-only /status metrics surface.
	ScopeStatusRead = "status:read"
	// ScopePublic ("") marks a route as unauthenticated (e.g. /healthz liveness).
	ScopePublic = ""
	// ScopeDeny is the fail-closed sentinel returned for any route ScopeForPath does
	// not explicitly map. It is intentionally unrepresentable as a granted scope: the
	// ':' guarantees it cannot be produced by LoadFromEnv parsing (which would split
	// it), and HasScope hard-denies it regardless of what a key set contains. An
	// unmapped route is therefore reachable by NO api key — never the old
	// fail-open ingest:write default.
	ScopeDeny = "deny:all"
)

// Authorizer enforces API-key → scope RBAC.
type Authorizer struct {
	keys map[string]map[string]bool // apiKey -> set of scopes
}

// NewAuthorizer builds an authorizer from key -> scopes.
func NewAuthorizer(keys map[string][]string) *Authorizer {
	m := map[string]map[string]bool{}
	for k, scopes := range keys {
		set := map[string]bool{}
		for _, s := range scopes {
			set[s] = true
		}
		m[k] = set
	}
	return &Authorizer{keys: m}
}

// LoadFromEnv parses SLOPPY_API_KEYS="key1=ingest:write,status:read;key2=status:read".
func LoadFromEnv() *Authorizer {
	keys := map[string][]string{}
	for _, entry := range strings.Split(os.Getenv("SLOPPY_API_KEYS"), ";") {
		entry = strings.TrimSpace(entry)
		if entry == "" {
			continue
		}
		k, scopes, ok := strings.Cut(entry, "=")
		if !ok {
			continue
		}
		var ss []string
		for _, s := range strings.Split(scopes, ",") {
			if s = strings.TrimSpace(s); s != "" {
				ss = append(ss, s)
			}
		}
		keys[strings.TrimSpace(k)] = ss
	}
	return NewAuthorizer(keys)
}

// ScopeForPath maps a request path to the scope it requires. It is the single
// scope source of truth for the HTTP API and is fail-closed: only explicitly
// listed routes get a grantable scope; everything else (including any future
// billing/write route added to the mux but forgotten here) returns ScopeDeny so
// it is reachable by no api key. Public routes return ScopePublic ("").
func ScopeForPath(path string) string {
	switch path {
	case "/healthz":
		return ScopePublic // liveness is public
	case "/status":
		return ScopeStatusRead
	case "/v1/signals", "/v1/usage", "/v1/otlp/metrics":
		return ScopeIngestWrite
	default:
		return ScopeDeny
	}
}

// KeyCount returns how many api keys are configured. A zero count while auth is
// enabled means every protected route is unreachable (auth-on-empty-keys), which
// the daemon logs loudly at startup.
func (a *Authorizer) KeyCount() int {
	if a == nil {
		return 0
	}
	return len(a.keys)
}

// HasScope reports whether the api key grants scope (the public scope is always
// granted). The ScopeDeny sentinel is hard-denied: no key can ever satisfy it,
// even if a malformed key set somehow contained the literal value.
func (a *Authorizer) HasScope(apiKey, scope string) bool {
	if scope == ScopeDeny {
		return false
	}
	if scope == ScopePublic {
		return true
	}
	set, ok := a.keys[apiKey]
	if !ok {
		return false
	}
	return set[scope]
}

func apiKeyFrom(r *http.Request) string {
	if k := r.Header.Get("X-API-Key"); k != "" {
		return k
	}
	if a := r.Header.Get("Authorization"); strings.HasPrefix(a, "Bearer ") {
		return strings.TrimPrefix(a, "Bearer ")
	}
	return ""
}

// Middleware enforces scope-per-path RBAC; /healthz stays public.
func (a *Authorizer) Middleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		scope := ScopeForPath(r.URL.Path)
		if !a.HasScope(apiKeyFrom(r), scope) {
			if scope == ScopeDeny {
				// Fail-closed: route has no scope mapping, so it is reachable by no key.
				http.Error(w, "forbidden: route not authorized", http.StatusForbidden)
				return
			}
			http.Error(w, "forbidden: requires scope "+scope, http.StatusForbidden)
			return
		}
		next.ServeHTTP(w, r)
	})
}
