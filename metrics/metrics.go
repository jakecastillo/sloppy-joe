// Package metrics is a tiny self-telemetry counter registry.
// (OTLP export is a thin adapter to add later; this is the v0 surface.)
package metrics

import "sync"

// Registry holds named counters.
type Registry struct {
	mu sync.Mutex
	m  map[string]int64
}

// emptySnapshot is a shared, never-mutated empty map returned by Snapshot when
// the registry holds no counters, so the routinely-polled /status endpoint does
// not churn a fresh allocation per call. Callers only read from snapshots.
var emptySnapshot = map[string]int64{}

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

// Get returns the value of a single counter (0 if absent) with a single mutex
// read and no map allocation. It matches Snapshot()[name] without copying the
// whole registry, for the common single-key read on the polled /status path.
func (r *Registry) Get(name string) int64 {
	if r == nil {
		return 0
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.m[name]
}

// Snapshot returns a copy of all counters. When the registry is empty it
// returns a shared empty map without allocating; otherwise callers get an
// independent copy.
func (r *Registry) Snapshot() map[string]int64 {
	r.mu.Lock()
	defer r.mu.Unlock()
	if len(r.m) == 0 {
		return emptySnapshot
	}
	out := make(map[string]int64, len(r.m))
	for k, v := range r.m {
		out[k] = v
	}
	return out
}
