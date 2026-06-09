package state

import (
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"database/sql"
	"encoding/base64"
	"path/filepath"
	"sync"
	"sync/atomic"
	"testing"

	"github.com/alicebob/miniredis/v2"
)

// loadCheckpointFor reaches the unexported per-backend checkpoint loader so a
// test can assert the persisted anchor directly (in-package).
func loadCheckpointFor(ctx context.Context, s Store) (Checkpoint, bool, error) {
	switch v := s.(type) {
	case *sqliteStore:
		return v.loadCheckpoint(ctx)
	case *redisStore:
		return v.loadCheckpoint(ctx)
	default:
		return Checkpoint{}, false, nil
	}
}

// testSigner is a minimal ed25519 CheckpointSigner for the tests: it both signs
// (with the private key) and exposes the public key, matching the production
// intent.Signer shape without importing it.
type testSigner struct {
	priv ed25519.PrivateKey
	pub  ed25519.PublicKey
}

func newTestSigner(t *testing.T) *testSigner {
	t.Helper()
	pub, priv, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	return &testSigner{priv: priv, pub: pub}
}

func (s *testSigner) Sign(payload []byte) string {
	return base64.StdEncoding.EncodeToString(ed25519.Sign(s.priv, payload))
}
func (s *testSigner) PublicKey() ed25519.PublicKey { return s.pub }

// TestVerifyChainGapNoLengthAnchor documents the residual gap that the signed
// checkpoint closes: VerifyChain alone has no length anchor, so a truncation
// that drops the tail (but keeps a valid prefix) still verifies as a chain.
func TestVerifyChainGapNoLengthAnchor(t *testing.T) {
	ctx := context.Background()
	s, _ := OpenSQLite(filepath.Join(t.TempDir(), "gap.db"))
	defer s.Close()
	if _, err := s.AppendAudit(ctx, "intent.applied", "a"); err != nil {
		t.Fatal(err)
	}
	if _, err := s.AppendAudit(ctx, "intent.applied", "b"); err != nil {
		t.Fatal(err)
	}
	full, _ := s.Audit(ctx)
	if len(full) != 2 {
		t.Fatalf("want 2 entries, got %d", len(full))
	}
	// A truncated prefix is itself a perfectly valid hash chain — proving
	// VerifyChain cannot, on its own, distinguish truncation from "less data".
	truncated := full[:1]
	if !VerifyChain(truncated) {
		t.Fatal("expected the truncated prefix to still pass VerifyChain (the gap)")
	}
}

// TestSQLiteCheckpointDetectsTruncation: with a checkpoint signer set, deleting
// the tail of the audit table (a valid prefix chain) must FAIL VerifyAudit
// because the persisted checkpoint count/head no longer match.
func TestSQLiteCheckpointDetectsTruncation(t *testing.T) {
	ctx := context.Background()
	path := filepath.Join(t.TempDir(), "trunc.db")
	signer := newTestSigner(t)
	s, err := OpenSQLite(path, WithCheckpointSigner(signer))
	if err != nil {
		t.Fatal(err)
	}
	for _, d := range []string{"a", "b", "c"} {
		if _, err := s.AppendAudit(ctx, "intent.applied", d); err != nil {
			t.Fatal(err)
		}
	}
	if !s.VerifyAudit(ctx) {
		t.Fatal("intact checkpointed chain must verify")
	}
	s.Close()

	// Truncate the tail directly in the DB, bypassing AppendAudit (so the
	// checkpoint is NOT updated) — the classic tamper.
	raw, err := sql.Open("sqlite", path)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := raw.Exec(`DELETE FROM audit WHERE seq=3`); err != nil {
		t.Fatal(err)
	}
	raw.Close()

	s2, err := OpenSQLite(path, WithCheckpointSigner(signer))
	if err != nil {
		t.Fatal(err)
	}
	defer s2.Close()
	if VerifyChain(mustAudit(t, ctx, s2)) == false {
		t.Fatal("sanity: the surviving prefix is still a valid chain (so only the checkpoint catches it)")
	}
	if s2.VerifyAudit(ctx) {
		t.Fatal("truncated chain must FAIL VerifyAudit via the checkpoint count anchor")
	}
}

// TestSQLiteCheckpointDetectsWholesaleReplacement: replacing the entire audit
// table with a freshly re-chained substitute (valid internal links) must FAIL
// because the substitute's head hash won't match the signed checkpoint.
func TestSQLiteCheckpointDetectsWholesaleReplacement(t *testing.T) {
	ctx := context.Background()
	path := filepath.Join(t.TempDir(), "repl.db")
	signer := newTestSigner(t)
	s, err := OpenSQLite(path, WithCheckpointSigner(signer))
	if err != nil {
		t.Fatal(err)
	}
	for _, d := range []string{"a", "b"} {
		if _, err := s.AppendAudit(ctx, "intent.applied", d); err != nil {
			t.Fatal(err)
		}
	}
	s.Close()

	// Wholesale replacement: wipe the chain and forge a brand-new, internally
	// consistent chain of the SAME length but different content. VerifyChain
	// passes on the forgery; only the signed checkpoint head/sig catches it.
	raw, err := sql.Open("sqlite", path)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := raw.Exec(`DELETE FROM audit`); err != nil {
		t.Fatal(err)
	}
	prev := ""
	for _, d := range []string{"evil1", "evil2"} {
		ts := "2026-01-01T00:00:00Z"
		h := ChainHash(ts, "intent.applied", d, prev)
		if _, err := raw.Exec(`INSERT INTO audit(ts,kind,detail,prev_hash,hash) VALUES(?,?,?,?,?)`,
			ts, "intent.applied", d, prev, h); err != nil {
			t.Fatal(err)
		}
		prev = h
	}
	raw.Close()

	s2, err := OpenSQLite(path, WithCheckpointSigner(signer))
	if err != nil {
		t.Fatal(err)
	}
	defer s2.Close()
	if !VerifyChain(mustAudit(t, ctx, s2)) {
		t.Fatal("sanity: the forged chain is internally valid (so only the checkpoint catches it)")
	}
	if s2.VerifyAudit(ctx) {
		t.Fatal("wholesale-replaced chain must FAIL VerifyAudit via the checkpoint head anchor")
	}
}

// TestSQLiteCheckpointDetectsFullDeletion: deleting EVERY entry while a
// checkpoint says entries existed must FAIL (checkpoint-missing-while-entries
// is symmetric: count>0 checkpoint but zero rows).
func TestSQLiteCheckpointDetectsFullDeletion(t *testing.T) {
	ctx := context.Background()
	path := filepath.Join(t.TempDir(), "del.db")
	signer := newTestSigner(t)
	s, err := OpenSQLite(path, WithCheckpointSigner(signer))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := s.AppendAudit(ctx, "intent.applied", "a"); err != nil {
		t.Fatal(err)
	}
	s.Close()

	raw, _ := sql.Open("sqlite", path)
	if _, err := raw.Exec(`DELETE FROM audit`); err != nil {
		t.Fatal(err)
	}
	raw.Close()

	s2, err := OpenSQLite(path, WithCheckpointSigner(signer))
	if err != nil {
		t.Fatal(err)
	}
	defer s2.Close()
	if s2.VerifyAudit(ctx) {
		t.Fatal("full deletion with a surviving count>0 checkpoint must FAIL VerifyAudit")
	}
}

// TestRedisCheckpointDetectsTruncation: Redis had NO tamper test before this.
// Truncating the audit list (LREM-style tail drop) must FAIL VerifyAudit.
func TestRedisCheckpointDetectsTruncation(t *testing.T) {
	ctx := context.Background()
	mr, err := miniredis.Run()
	if err != nil {
		t.Fatal(err)
	}
	defer mr.Close()
	signer := newTestSigner(t)
	s, err := OpenRedis(mr.Addr(), WithCheckpointSigner(signer))
	if err != nil {
		t.Fatal(err)
	}
	for _, d := range []string{"a", "b", "c"} {
		if _, err := s.AppendAudit(ctx, "intent.applied", d); err != nil {
			t.Fatal(err)
		}
	}
	if !s.VerifyAudit(ctx) {
		t.Fatal("intact checkpointed redis chain must verify")
	}

	// Drop the tail entry directly in the list, bypassing AppendAudit, so the
	// checkpoint is left stale: rebuild the list as just the first two entries.
	full, _ := mr.List(keyAudit)
	if len(full) != 3 {
		t.Fatalf("want 3 raw entries, got %d", len(full))
	}
	mr.Del(keyAudit)
	for _, v := range full[:2] {
		if _, err := mr.Push(keyAudit, v); err != nil {
			t.Fatal(err)
		}
	}

	if !VerifyChain(mustAudit(t, ctx, s)) {
		t.Fatal("sanity: surviving redis prefix is still a valid chain")
	}
	if s.VerifyAudit(ctx) {
		t.Fatal("truncated redis chain must FAIL VerifyAudit via the checkpoint count anchor")
	}
}

// TestRedisCheckpointDetectsWholesaleReplacement: replace the whole list with a
// forged, internally valid chain — only the signed checkpoint head catches it.
func TestRedisCheckpointDetectsWholesaleReplacement(t *testing.T) {
	ctx := context.Background()
	mr, err := miniredis.Run()
	if err != nil {
		t.Fatal(err)
	}
	defer mr.Close()
	signer := newTestSigner(t)
	s, err := OpenRedis(mr.Addr(), WithCheckpointSigner(signer))
	if err != nil {
		t.Fatal(err)
	}
	for _, d := range []string{"a", "b"} {
		if _, err := s.AppendAudit(ctx, "intent.applied", d); err != nil {
			t.Fatal(err)
		}
	}

	mr.Del(keyAudit)
	prev := ""
	for _, d := range []string{"evil1", "evil2"} {
		ts := "2026-01-01T00:00:00Z"
		h := ChainHash(ts, "intent.applied", d, prev)
		rec := `{"ts":"` + ts + `","kind":"intent.applied","detail":"` + d + `","prev_hash":"` + prev + `","hash":"` + h + `"}`
		if _, err := mr.Push(keyAudit, rec); err != nil {
			t.Fatal(err)
		}
		prev = h
	}

	if !VerifyChain(mustAudit(t, ctx, s)) {
		t.Fatal("sanity: forged redis chain is internally valid")
	}
	if s.VerifyAudit(ctx) {
		t.Fatal("wholesale-replaced redis chain must FAIL VerifyAudit via the checkpoint head anchor")
	}
}

// TestCheckpointConcurrentAppendsStayConsistent asserts the checkpoint anchor
// never lags the chain under concurrent appends: after N parallel appends the
// chain still verifies AND the persisted checkpoint count equals exactly N (the
// atomic per-append checkpoint update, not a racy last-writer-wins). Run against
// both backends. Deterministic (count assertion), so plain `go test` validates it.
func TestCheckpointConcurrentAppendsStayConsistent(t *testing.T) {
	run := func(t *testing.T, s Store) {
		ctx := context.Background()
		const n = 24
		// Count only appends that REPORT success: under optimistic-locking
		// contention a backend may legitimately return an error (the caller must
		// handle it), so the invariant we assert is that the checkpoint count
		// equals the number of committed appends — never lags or over-counts.
		var ok int64
		var wg sync.WaitGroup
		for i := 0; i < n; i++ {
			wg.Add(1)
			go func(k int) {
				defer wg.Done()
				if _, err := s.AppendAudit(ctx, "concurrent", "e"); err == nil {
					atomic.AddInt64(&ok, 1)
				}
			}(i)
		}
		wg.Wait()
		es := mustAudit(t, ctx, s)
		committed := int(atomic.LoadInt64(&ok))
		if len(es) != committed {
			t.Fatalf("chain length %d must equal committed appends %d", len(es), committed)
		}
		if !s.VerifyAudit(ctx) {
			t.Fatal("chain+checkpoint must verify after concurrent appends")
		}
		// The signed checkpoint count must match the chain exactly — the anchor
		// never lags the entries it certifies.
		cp, found, err := loadCheckpointFor(ctx, s)
		if err != nil {
			t.Fatal(err)
		}
		if committed > 0 && (!found || cp.Count != committed) {
			t.Fatalf("checkpoint count %d (found=%v) must equal committed appends %d", cp.Count, found, committed)
		}
	}
	t.Run("sqlite", func(t *testing.T) {
		s, err := OpenSQLite(filepath.Join(t.TempDir(), "cc.db"), WithCheckpointSigner(newTestSigner(t)))
		if err != nil {
			t.Fatal(err)
		}
		defer s.Close()
		run(t, s)
	})
	t.Run("redis", func(t *testing.T) {
		mr, err := miniredis.Run()
		if err != nil {
			t.Fatal(err)
		}
		defer mr.Close()
		s, err := OpenRedis(mr.Addr(), WithCheckpointSigner(newTestSigner(t)))
		if err != nil {
			t.Fatal(err)
		}
		defer s.Close()
		run(t, s)
	})
}

func mustAudit(t *testing.T, ctx context.Context, s Store) []AuditEntry {
	t.Helper()
	es, err := s.Audit(ctx)
	if err != nil {
		t.Fatal(err)
	}
	return es
}
