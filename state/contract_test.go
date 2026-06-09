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
	ctx := t.Context()

	// Idempotency.
	if a, _ := s.IsIntentApplied(ctx, "i1"); a {
		t.Fatal("i1 should be new")
	}
	if err := s.MarkIntentApplied(ctx, "i1"); err != nil {
		t.Fatal(err)
	}
	if a, _ := s.IsIntentApplied(ctx, "i1"); !a {
		t.Fatal("i1 should be applied")
	}
	if err := s.MarkIntentApplied(ctx, "i1"); err != nil {
		t.Fatalf("re-mark must be idempotent: %v", err)
	}

	// Hash-chained audit.
	if _, err := s.AppendAudit(ctx, "intent.applied", "a"); err != nil {
		t.Fatal(err)
	}
	if _, err := s.AppendAudit(ctx, "intent.reverted", "b"); err != nil {
		t.Fatal(err)
	}
	if !s.VerifyAudit(ctx) {
		t.Fatal("chain should verify")
	}
	es, _ := s.Audit(ctx)
	if len(es) != 2 {
		t.Fatalf("want 2 audit entries, got %d", len(es))
	}
	if es[1].PrevHash != es[0].Hash {
		t.Fatal("audit chain link broken")
	}

	// Pending reverts.
	base := time.Unix(1749340800, 0).UTC()
	if err := s.ScheduleRevert(ctx, PendingRevert{IntentID: "r1", Kind: "route_override", Target: "m", ArgsJSON: `{}`, DueAt: base.Add(30 * time.Minute)}); err != nil {
		t.Fatal(err)
	}
	if d, _ := s.DueReverts(ctx, base); len(d) != 0 {
		t.Fatalf("nothing due at base, got %d", len(d))
	}
	d, _ := s.DueReverts(ctx, base.Add(time.Hour))
	if len(d) != 1 || d[0].IntentID != "r1" || d[0].Target != "m" {
		t.Fatalf("expected r1 due, got %+v", d)
	}
	if err := s.MarkReverted(ctx, "r1"); err != nil {
		t.Fatal(err)
	}
	if d, _ := s.DueReverts(ctx, base.Add(time.Hour)); len(d) != 0 {
		t.Fatalf("reverted entry should be gone, got %d", len(d))
	}

	// Rule-action budget accounting.
	if err := s.RecordAction(ctx, "ruleA", base); err != nil {
		t.Fatal(err)
	}
	if err := s.RecordAction(ctx, "ruleA", base.Add(time.Second)); err != nil {
		t.Fatal(err)
	}
	if n, _ := s.CountActions(ctx, "ruleA", base); n != 2 {
		t.Fatalf("want 2 actions since base, got %d", n)
	}
	if n, _ := s.CountActions(ctx, "ruleA", base.Add(time.Hour)); n != 0 {
		t.Fatalf("want 0 actions in the future, got %d", n)
	}

	// On-clear outstanding tracking.
	if err := s.RecordOutstanding(ctx, "ruleA|acme", PendingRevert{IntentID: "o1", Kind: "route_override", Target: "m", ArgsJSON: "{}"}); err != nil {
		t.Fatal(err)
	}
	if outs, _ := s.Outstanding(ctx, "ruleA|acme"); len(outs) != 1 || outs[0].IntentID != "o1" {
		t.Fatalf("outstanding: %+v", outs)
	}
	if err := s.ClearOutstanding(ctx, "ruleA|acme"); err != nil {
		t.Fatal(err)
	}
	if outs, _ := s.Outstanding(ctx, "ruleA|acme"); len(outs) != 0 {
		t.Fatalf("cleared outstanding should be empty: %+v", outs)
	}

	// Cost-ledger usage accounting.
	if err := s.RecordUsage(ctx, "acme", "gpt-4o", 5.0, base); err != nil {
		t.Fatal(err)
	}
	if err := s.RecordUsage(ctx, "acme", "gpt-4o", 7.5, base.Add(time.Minute)); err != nil {
		t.Fatal(err)
	}
	if spend, _ := s.SpendSince(ctx, "acme", base); spend != 12.5 {
		t.Fatalf("want spend 12.5, got %v", spend)
	}
	if spend, _ := s.SpendSince(ctx, "other", base); spend != 0 {
		t.Fatalf("other tenant should be 0, got %v", spend)
	}
	if spend, _ := s.SpendSince(ctx, "acme", base.Add(time.Hour)); spend != 0 {
		t.Fatalf("future window should be 0, got %v", spend)
	}
	if err := s.PruneUsage(ctx, base.Add(time.Hour)); err != nil {
		t.Fatalf("PruneUsage: %v", err) // no-op on Redis; deletes pre-cutoff rows on SQLite
	}

	// Concurrency: parallel appends must keep the tamper-evident chain intact.
	var wg sync.WaitGroup
	for i := 0; i < 20; i++ {
		wg.Add(1)
		go func(n int) {
			defer wg.Done()
			_, _ = s.AppendAudit(ctx, "concurrent", fmt.Sprintf("e%d", n))
		}(i)
	}
	wg.Wait()
	if !s.VerifyAudit(ctx) {
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
