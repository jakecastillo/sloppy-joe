// Package metrics is a tiny self-telemetry counter registry.
// (OTLP export is a thin adapter to add later; this is the v0 surface.)
package metrics

import "sync"

// Registry holds named counters.
type Registry struct {
	mu sync.Mutex
	m  map[string]int64
}

// New creates an empty registry.
func New() *Registry { return &Registry{m: map[string]int64{}} }

// Inc adds 1 to a counter.
func (r *Registry) Inc(name string) { r.Add(name, 1) }

// Add adds n to a counter (no-op on a nil registry).
func (r *Registry) Add(name string, n int64) {
	if r == nil {
		return
	}
	r.mu.Lock()
	if r.m == nil {
		r.m = map[string]int64{}
	}
	r.m[name] += n
	r.mu.Unlock()
}

// Snapshot returns a copy of all counters.
func (r *Registry) Snapshot() map[string]int64 {
	r.mu.Lock()
	defer r.mu.Unlock()
	out := make(map[string]int64, len(r.m))
	for k, v := range r.m {
		out[k] = v
	}
	return out
}
