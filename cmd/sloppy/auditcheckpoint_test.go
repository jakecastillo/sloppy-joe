package main

import (
	"bytes"
	"database/sql"
	"os"
	"path/filepath"
	"strings"
	"testing"

	_ "modernc.org/sqlite"
)

// End-to-end: `sloppy audit` is the operator/CI tamper-check surface (the daemon
// never calls VerifyAudit at runtime). It MUST enforce the signed checkpoint, so
// that an attacker who strips the audit_checkpoint row over a non-empty, otherwise
// re-chained chain is reported as TAMPERED with a non-zero exit — not "verified ✓".
//
// This exercises the actual CLI path (run -> cmdAudit), unlike the in-package
// state tests which reopen the store WITH a signer.
func TestAuditCommandDetectsCheckpointStrip(t *testing.T) {
	dir := t.TempDir()
	rulesDir := filepath.Join(dir, "rules")
	if err := os.MkdirAll(rulesDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(rulesDir, "cost.yaml"), []byte(`
on: cost.budget_burn
when: signal.data.spend_1h_usd > 5.0
then: [ { route_override: { alias: gpt-4o, to: ollama/llama3 } } ]
`), 0o644); err != nil {
		t.Fatal(err)
	}
	sigPath := filepath.Join(dir, "sig.json")
	if err := os.WriteFile(sigPath, []byte(`{"type":"cost.budget_burn","correlation_key":"acme:cost","subject":{"alias":"gpt-4o"},"data":{"spend_1h_usd":9.0}}`), 0o644); err != nil {
		t.Fatal(err)
	}
	db := filepath.Join(dir, "s.db")
	key := filepath.Join(dir, "sloppy.key")

	// inject --now opens the store WITH a checkpoint signer, so it persists a
	// signed length+head checkpoint alongside the applied audit entry.
	var inj bytes.Buffer
	if code := run([]string{"inject", "--now", "--rules", rulesDir, "--db", db, "--key", key, sigPath}, &inj); code != 0 {
		t.Fatalf("inject exit %d: %s", code, inj.String())
	}

	// Clean store: audit reports verified and exits 0.
	var clean bytes.Buffer
	if code := run([]string{"audit", "--db", db, "--key", key}, &clean); code != 0 {
		t.Fatalf("audit on a clean checkpointed store must exit 0, got %d: %s", code, clean.String())
	}
	if !strings.Contains(clean.String(), "verified") {
		t.Fatalf("expected clean audit to report verified, got: %s", clean.String())
	}

	// Sanity: there must actually be a checkpoint to strip (otherwise the test
	// proves nothing about checkpoint enforcement).
	if !checkpointRowExists(t, db) {
		t.Fatal("expected inject to have persisted an audit_checkpoint row")
	}

	// Tamper: strip the signed checkpoint row. The audit hash chain itself is left
	// intact and valid, so chain-only verification would still pass — only checkpoint
	// enforcement catches this. The daemon never re-verifies, so the CLI is the gate.
	stripCheckpointRow(t, db)
	if checkpointRowExists(t, db) {
		t.Fatal("strip did not remove the audit_checkpoint row")
	}

	var bad bytes.Buffer
	code := run([]string{"audit", "--db", db, "--key", key}, &bad)
	if code == 0 {
		t.Fatalf("audit must exit non-zero when the signed checkpoint is stripped; output: %s", bad.String())
	}
	if !strings.Contains(bad.String(), "TAMPERED") {
		t.Fatalf("audit must report TAMPERED on a stripped checkpoint, got: %s", bad.String())
	}
}

// checkpointRowExists reports whether the singleton audit_checkpoint row is present.
func checkpointRowExists(t *testing.T, db string) bool {
	t.Helper()
	dbc, err := sql.Open("sqlite", db+"?_pragma=busy_timeout(5000)&_pragma=journal_mode(WAL)")
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	defer dbc.Close()
	var n int
	if err := dbc.QueryRow(`SELECT COUNT(*) FROM audit_checkpoint WHERE id=1`).Scan(&n); err != nil {
		t.Fatalf("count checkpoint: %v", err)
	}
	return n == 1
}

// stripCheckpointRow deletes the signed checkpoint anchor, simulating an attacker
// who removes the one artifact they cannot forge without the signing key.
func stripCheckpointRow(t *testing.T, db string) {
	t.Helper()
	dbc, err := sql.Open("sqlite", db+"?_pragma=busy_timeout(5000)&_pragma=journal_mode(WAL)")
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	defer dbc.Close()
	if _, err := dbc.Exec(`DELETE FROM audit_checkpoint`); err != nil {
		t.Fatalf("delete checkpoint: %v", err)
	}
}
