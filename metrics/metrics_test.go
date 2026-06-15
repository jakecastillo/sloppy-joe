package metrics

import (
	"reflect"
	"testing"
)

func TestRegistryCounts(t *testing.T) {
	r := New()
	r.Inc("a")
	r.Inc("a")
	r.Add("b", 5)
	s := r.Snapshot()
	if s["a"] != 2 || s["b"] != 5 {
		t.Fatalf("bad snapshot: %+v", s)
	}
}

func TestNilRegistrySafe(t *testing.T) {
	var r *Registry
	r.Inc("x") // must not panic
	r.Add("y", 3)
}

func TestZeroValueRegistryUsable(t *testing.T) {
	var r Registry // zero value, nil map
	r.Inc("a")     // must not panic
	r.Add("a", 2)
	r.Add("b", 5)
	s := r.Snapshot()
	if s["a"] != 3 || s["b"] != 5 {
		t.Fatalf("bad snapshot: %+v", s)
	}
}

func TestGetMatchesSnapshot(t *testing.T) {
	r := New()
	r.Inc("a")
	r.Inc("a")
	r.Add("b", 5)
	snap := r.Snapshot()
	for _, name := range []string{"a", "b", "missing"} {
		if got, want := r.Get(name), snap[name]; got != want {
			t.Fatalf("Get(%q)=%d, Snapshot()[%q]=%d", name, got, name, want)
		}
	}
}

func TestGetNilRegistry(t *testing.T) {
	var r *Registry
	if got := r.Get("x"); got != 0 { // must not panic, returns 0
		t.Fatalf("nil Get=%d, want 0", got)
	}
}

func TestEmptySnapshotNoAlloc(t *testing.T) {
	r := New()
	if allocs := testing.AllocsPerRun(100, func() {
		_ = r.Snapshot()
	}); allocs != 0 {
		t.Fatalf("empty Snapshot allocated %v times, want 0", allocs)
	}
}

func TestGetNoAlloc(t *testing.T) {
	r := New()
	r.Inc("a")
	if allocs := testing.AllocsPerRun(100, func() {
		_ = r.Get("a")
	}); allocs != 0 {
		t.Fatalf("Get allocated %v times, want 0", allocs)
	}
}

func TestSnapshotIntoRefills(t *testing.T) {
	r := New()
	r.Inc("a")
	r.Inc("a")
	r.Add("b", 5)
	want := r.Snapshot()

	dst := map[string]int64{}
	got := r.SnapshotInto(dst)
	// SnapshotInto returns the same instance it was handed.
	if !sameMap(got, dst) {
		t.Fatalf("SnapshotInto returned a different map than dst")
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("SnapshotInto=%+v, Snapshot()=%+v", got, want)
	}
}

func TestSnapshotIntoClearsStaleKeys(t *testing.T) {
	r := New()
	r.Add("b", 5)

	// Populate dst with stale keys (including one that is not a current counter)
	// to prove the reused buffer is cleared, not merged.
	dst := map[string]int64{"a": 99, "stale": 7}
	got := r.SnapshotInto(dst)
	want := map[string]int64{"b": 5}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("SnapshotInto reused buffer=%+v, want %+v", got, want)
	}

	// Reuse again after the registry empties out: every key must be cleared.
	r2 := New()
	if cleared := r2.SnapshotInto(got); len(cleared) != 0 {
		t.Fatalf("SnapshotInto on empty registry left %d keys: %+v", len(cleared), cleared)
	}
}

func TestSnapshotIntoNilDst(t *testing.T) {
	r := New()
	r.Add("a", 3)
	got := r.SnapshotInto(nil)
	if want := (map[string]int64{"a": 3}); !reflect.DeepEqual(got, want) {
		t.Fatalf("SnapshotInto(nil)=%+v, want %+v", got, want)
	}
}

func TestSnapshotIntoNilRegistry(t *testing.T) {
	var r *Registry
	dst := map[string]int64{"stale": 1}
	got := r.SnapshotInto(dst) // must not panic; clears dst
	if len(got) != 0 {
		t.Fatalf("nil SnapshotInto left %d keys: %+v", len(got), got)
	}
}

func TestSnapshotReturnsIndependentMap(t *testing.T) {
	r := New()
	r.Add("a", 1)

	s1 := r.Snapshot()
	s2 := r.Snapshot()
	// Two non-empty snapshots must be independent instances.
	if sameMap(s1, s2) {
		t.Fatalf("Snapshot returned the same map instance across calls")
	}
	// Mutating one (callers only read, but prove isolation) must not affect the
	// registry or another snapshot.
	s1["a"] = 999
	if r.Get("a") != 1 {
		t.Fatalf("mutating a snapshot changed the registry")
	}
	if s2["a"] != 1 {
		t.Fatalf("snapshots share backing storage: s2[a]=%d", s2["a"])
	}
}

func TestSnapshotIntoReuseNoAlloc(t *testing.T) {
	r := New()
	r.Inc("a")
	r.Add("b", 5)
	dst := map[string]int64{}
	// Warm the buffer once so its capacity covers the steady-state key set.
	dst = r.SnapshotInto(dst)
	if allocs := testing.AllocsPerRun(100, func() {
		dst = r.SnapshotInto(dst)
	}); allocs != 0 {
		t.Fatalf("reused SnapshotInto allocated %v times, want 0", allocs)
	}
}

// sameMap reports whether a and b are the same underlying map instance.
func sameMap(a, b map[string]int64) bool {
	return reflect.ValueOf(a).Pointer() == reflect.ValueOf(b).Pointer()
}
