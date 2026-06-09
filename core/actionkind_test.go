package core

import "testing"

// TestKnownActionKindsTable pins the exact set of governed action kinds. If a new
// ActionKind constant is added, it MUST be listed in KnownActionKinds() (and thus
// accepted by rule validation); this table is the single source of truth that
// forces that wiring, so a kind can never be defined-but-unvalidatable.
func TestKnownActionKindsTable(t *testing.T) {
	want := []ActionKind{
		ActionRouteOverride,
		ActionOpenIssue,
		ActionPage,
		ActionThrottleTenant,
		ActionDisableDeployment,
	}
	got := KnownActionKinds()
	if len(got) != len(want) {
		t.Fatalf("KnownActionKinds count = %d, want %d (%v)", len(got), len(want), got)
	}
	for i, k := range want {
		if got[i] != k {
			t.Fatalf("KnownActionKinds[%d] = %q, want %q", i, got[i], k)
		}
	}
}

func TestIsKnownActionKind(t *testing.T) {
	for _, k := range KnownActionKinds() {
		if !IsKnownActionKind(k) {
			t.Errorf("IsKnownActionKind(%q) = false, want true", k)
		}
	}
	for _, bad := range []ActionKind{"", "route_overide", "delete_everything", "ROUTE_OVERRIDE"} {
		if IsKnownActionKind(bad) {
			t.Errorf("IsKnownActionKind(%q) = true, want false", bad)
		}
	}
}
