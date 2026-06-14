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
