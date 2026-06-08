package state

import (
	"path/filepath"
	"testing"
	"time"
)

func TestPendingRevertsLifecycle(t *testing.T) {
	db := filepath.Join(t.TempDir(), "r.db")
	s, err := OpenSQLite(db)
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()

	base := time.Unix(1749340800, 0).UTC()
	if err := s.ScheduleRevert(PendingRevert{IntentID: "i1", Kind: "route_override", Target: "gpt-4o", ArgsJSON: `{"to":"gpt-4o"}`, DueAt: base.Add(30 * time.Minute)}); err != nil {
		t.Fatalf("schedule: %v", err)
	}
	// Idempotent on intent id.
	if err := s.ScheduleRevert(PendingRevert{IntentID: "i1", DueAt: base.Add(30 * time.Minute)}); err != nil {
		t.Fatalf("re-schedule must be idempotent: %v", err)
	}

	// Not due yet.
	if due, _ := s.DueReverts(base); len(due) != 0 {
		t.Fatalf("nothing should be due at base, got %d", len(due))
	}
	// Due after ttl.
	due, _ := s.DueReverts(base.Add(31 * time.Minute))
	if len(due) != 1 || due[0].IntentID != "i1" || due[0].Target != "gpt-4o" {
		t.Fatalf("expected i1 due, got %+v", due)
	}
	// Reverting removes it.
	if err := s.MarkReverted("i1"); err != nil {
		t.Fatalf("mark reverted: %v", err)
	}
	if due, _ := s.DueReverts(base.Add(time.Hour)); len(due) != 0 {
		t.Fatalf("reverted entry should be gone, got %d", len(due))
	}
}
