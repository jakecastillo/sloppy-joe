package state

import (
	"fmt"
	"sync"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
)

// storeContract exercises the Store contract; run against every backend.
func storeContract(t *testing.T, s Store) {
	t.Helper()

	// Idempotency.
	if a, _ := s.IsIntentApplied("i1"); a {
		t.Fatal("i1 should be new")
	}
	if err := s.MarkIntentApplied("i1"); err != nil {
		t.Fatal(err)
	}
	if a, _ := s.IsIntentApplied("i1"); !a {
		t.Fatal("i1 should be applied")
	}
	if err := s.MarkIntentApplied("i1"); err != nil {
		t.Fatalf("re-mark must be idempotent: %v", err)
	}

	// Hash-chained audit.
	if _, err := s.AppendAudit("intent.applied", "a"); err != nil {
		t.Fatal(err)
	}
	if _, err := s.AppendAudit("intent.reverted", "b"); err != nil {
		t.Fatal(err)
	}
	if !s.VerifyAudit() {
		t.Fatal("chain should verify")
	}
	es, _ := s.Audit()
	if len(es) != 2 {
		t.Fatalf("want 2 audit entries, got %d", len(es))
	}
	if es[1].PrevHash != es[0].Hash {
		t.Fatal("audit chain link broken")
	}

	// Pending reverts.
	base := time.Unix(1749340800, 0).UTC()
	if err := s.ScheduleRevert(PendingRevert{IntentID: "r1", Kind: "route_override", Target: "m", ArgsJSON: `{}`, DueAt: base.Add(30 * time.Minute)}); err != nil {
		t.Fatal(err)
	}
	if d, _ := s.DueReverts(base); len(d) != 0 {
		t.Fatalf("nothing due at base, got %d", len(d))
	}
	d, _ := s.DueReverts(base.Add(time.Hour))
	if len(d) != 1 || d[0].IntentID != "r1" || d[0].Target != "m" {
		t.Fatalf("expected r1 due, got %+v", d)
	}
	if err := s.MarkReverted("r1"); err != nil {
		t.Fatal(err)
	}
	if d, _ := s.DueReverts(base.Add(time.Hour)); len(d) != 0 {
		t.Fatalf("reverted entry should be gone, got %d", len(d))
	}

	// Concurrency: parallel appends must keep the tamper-evident chain intact.
	var wg sync.WaitGroup
	for i := 0; i < 20; i++ {
		wg.Add(1)
		go func(n int) {
			defer wg.Done()
			_, _ = s.AppendAudit("concurrent", fmt.Sprintf("e%d", n))
		}(i)
	}
	wg.Wait()
	if !s.VerifyAudit() {
		t.Fatal("audit chain corrupt after concurrent appends")
	}
}

func TestSQLiteContract(t *testing.T) {
	s, err := OpenSQLite(t.TempDir() + "/c.db")
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	storeContract(t, s)
}

func TestRedisContract(t *testing.T) {
	mr, err := miniredis.Run()
	if err != nil {
		t.Fatal(err)
	}
	defer mr.Close()
	s, err := OpenRedis(mr.Addr())
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	storeContract(t, s)
}
