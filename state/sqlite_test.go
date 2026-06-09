package state

import (
	"context"
	"database/sql"
	"path/filepath"
	"testing"
	"time"
)

func TestSQLiteUsagePrune(t *testing.T) {
	ctx := context.Background()
	s, _ := OpenSQLite(filepath.Join(t.TempDir(), "u.db"))
	defer s.Close()
	base := time.Unix(1749340800, 0).UTC()
	_ = s.RecordUsage(ctx, "acme", "m", 1.0, base.Add(-2*time.Hour)) // old
	_ = s.RecordUsage(ctx, "acme", "m", 2.0, base)                   // recent
	if spend, _ := s.SpendSince(ctx, "acme", base.Add(-3*time.Hour)); spend != 3.0 {
		t.Fatalf("pre-prune want 3.0, got %v", spend)
	}
	if err := s.PruneUsage(ctx, base.Add(-time.Hour)); err != nil {
		t.Fatal(err)
	}
	if spend, _ := s.SpendSince(ctx, "acme", base.Add(-3*time.Hour)); spend != 2.0 {
		t.Fatalf("post-prune want 2.0 (old pruned), got %v", spend)
	}
}

func TestSQLiteIdempotencyAndAuditPersist(t *testing.T) {
	ctx := context.Background()
	db := filepath.Join(t.TempDir(), "test.db")
	s, err := OpenSQLite(db)
	if err != nil {
		t.Fatalf("open: %v", err)
	}

	if applied, _ := s.IsIntentApplied(ctx, "int-1"); applied {
		t.Fatal("int-1 should be new")
	}
	if err := s.MarkIntentApplied(ctx, "int-1"); err != nil {
		t.Fatalf("mark: %v", err)
	}
	if applied, _ := s.IsIntentApplied(ctx, "int-1"); !applied {
		t.Fatal("int-1 should be applied after mark")
	}
	if err := s.MarkIntentApplied(ctx, "int-1"); err != nil {
		t.Fatalf("re-mark must be idempotent: %v", err)
	}

	if _, err := s.AppendAudit(ctx, "intent.applied", "reroute acme"); err != nil {
		t.Fatalf("audit: %v", err)
	}
	if _, err := s.AppendAudit(ctx, "intent.reverted", "restore acme"); err != nil {
		t.Fatalf("audit: %v", err)
	}
	if !s.VerifyAudit(ctx) {
		t.Fatal("fresh chain must verify")
	}
	s.Close()

	s2, err := OpenSQLite(db)
	if err != nil {
		t.Fatalf("reopen: %v", err)
	}
	defer s2.Close()
	if !s2.VerifyAudit(ctx) {
		t.Fatal("persisted chain must verify after reopen")
	}
	entries, _ := s2.Audit(ctx)
	if len(entries) != 2 {
		t.Fatalf("want 2 audit entries, got %d", len(entries))
	}
}

func TestSQLiteAuditTamperDetected(t *testing.T) {
	ctx := context.Background()
	path := filepath.Join(t.TempDir(), "t.db")
	s, _ := OpenSQLite(path)
	if _, err := s.AppendAudit(ctx, "intent.applied", "legit"); err != nil {
		t.Fatal(err)
	}
	s.Close()

	// Tamper directly in the DB, bypassing the chain logic.
	raw, err := sql.Open("sqlite", path)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := raw.Exec(`UPDATE audit SET detail='evil' WHERE seq=1`); err != nil {
		t.Fatal(err)
	}
	raw.Close()

	s2, _ := OpenSQLite(path)
	defer s2.Close()
	if s2.VerifyAudit(ctx) {
		t.Fatal("tampered chain must fail verify")
	}
}
