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
// independent copy. It delegates to SnapshotInto with a fresh map, so each
// caller keeps an independent result.
func (r *Registry) Snapshot() map[string]int64 {
	out := r.SnapshotInto(make(map[string]int64))
	if len(out) == 0 {
		// Preserve the shared, never-mutated empty map for the routinely-polled
		// empty case so Snapshot stays allocation-free there.
		return emptySnapshot
	}
	return out
}

// SnapshotInto clears dst and refills it with the current counters under the
// existing mutex, returning the same content Snapshot() would. It lets a
// long-lived caller (the routinely-polled /status handler) reuse one buffer
// across calls instead of churning a fresh map per request, so the steady state
// is allocation-free. dst may be nil, in which case a fresh map is allocated.
// Callers only read from the returned map; it is the same instance as dst (when
// non-nil), so the caller can reuse it on the next call.
func (r *Registry) SnapshotInto(dst map[string]int64) map[string]int64 {
	if dst == nil {
		dst = map[string]int64{}
	}
	for k := range dst {
		delete(dst, k)
	}
	if r == nil {
		return dst
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	for k, v := range r.m {
		dst[k] = v
	}
	return dst
}
