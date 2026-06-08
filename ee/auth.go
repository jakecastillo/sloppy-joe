// Package ee holds optional enterprise features (the open-core boundary).
// v0: API-key → scope RBAC for the HTTP API.
package ee

import (
	"net/http"
	"os"
	"strings"
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

// ScopeForPath maps a request path to the scope it requires ("" = public).
func ScopeForPath(path string) string {
	switch path {
	case "/healthz":
		return "" // liveness is public
	case "/status":
		return "status:read"
	default:
		return "ingest:write"
	}
}

// HasScope reports whether the api key grants scope (the public scope is always granted).
func (a *Authorizer) HasScope(apiKey, scope string) bool {
	if scope == "" {
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
			http.Error(w, "forbidden: requires scope "+scope, http.StatusForbidden)
			return
		}
		next.ServeHTTP(w, r)
	})
}
