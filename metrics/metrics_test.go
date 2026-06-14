package metrics

import "testing"

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
