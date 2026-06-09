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

// End-to-end: inject (persisting a signing key + exported .pub) writes a signed,
// applied audit entry; `audit --verify-sigs` then verifies it against the
// persisted public key and exits 0. Tampering a persisted intent field makes
// verification fail and the command exit non-zero.
func TestAuditVerifySigs(t *testing.T) {
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

	var inj bytes.Buffer
	if code := run([]string{"inject", "--now", "--rules", rulesDir, "--db", db, "--key", key, sigPath}, &inj); code != 0 {
		t.Fatalf("inject exit %d: %s", code, inj.String())
	}
	if _, err := os.Stat(key + ".pub"); err != nil {
		t.Fatalf("inject must persist the public key for verification: %v", err)
	}

	// Clean: every persisted signature verifies, exit 0.
	var clean bytes.Buffer
	if code := run([]string{"audit", "--verify-sigs", "--db", db, "--key", key}, &clean); code != 0 {
		t.Fatalf("verify-sigs on a clean store must exit 0, got %d: %s", code, clean.String())
	}
	if !strings.Contains(clean.String(), "verified") || strings.Contains(clean.String(), "failed=0") == false {
		t.Fatalf("expected a verified report with failed=0, got: %s", clean.String())
	}

	// Tamper: rewrite the canonical payload in the applied audit row so the
	// stored signature no longer matches. verify-sigs must report a failure and exit non-zero.
	tamperAppliedCanon(t, db)
	var bad bytes.Buffer
	if code := run([]string{"audit", "--verify-sigs", "--db", db, "--key", key}, &bad); code == 0 {
		t.Fatalf("verify-sigs must exit non-zero when a signature fails; output: %s", bad.String())
	}
}

// tamperAppliedCanon flips a byte inside the base64 `canon=` token of every
// intent.applied audit row, simulating an attacker editing a persisted intent.
func tamperAppliedCanon(t *testing.T, db string) {
	t.Helper()
	dbc, err := sql.Open("sqlite", db+"?_pragma=busy_timeout(5000)&_pragma=journal_mode(WAL)")
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	defer dbc.Close()
	rows, err := dbc.Query(`SELECT seq, detail FROM audit WHERE kind='intent.applied'`)
	if err != nil {
		t.Fatalf("select audit: %v", err)
	}
	type row struct {
		seq    int
		detail string
	}
	var found []row
	for rows.Next() {
		var r row
		if err := rows.Scan(&r.seq, &r.detail); err != nil {
			t.Fatalf("scan: %v", err)
		}
		found = append(found, r)
	}
	rows.Close()
	if len(found) == 0 {
		t.Fatal("no intent.applied rows to tamper")
	}
	for _, r := range found {
		idx := strings.Index(r.detail, "canon=")
		if idx < 0 {
			t.Fatalf("applied detail has no canon= token: %q", r.detail)
		}
		// Flip a character a few bytes into the base64 canon token.
		pos := idx + len("canon=") + 2
		b := []byte(r.detail)
		if b[pos] == 'A' {
			b[pos] = 'B'
		} else {
			b[pos] = 'A'
		}
		if _, err := dbc.Exec(`UPDATE audit SET detail=? WHERE seq=?`, string(b), r.seq); err != nil {
			t.Fatalf("update: %v", err)
		}
	}
}
