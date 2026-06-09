package main

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// The flagship one-shot demo: a `for:` rule is pending without --now, and fires
// (writing a verifiable audit chain) with --now.
func TestInjectNowBypassesForWindow(t *testing.T) {
	dir := t.TempDir()
	rulesDir := filepath.Join(dir, "rules")
	if err := os.MkdirAll(rulesDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(rulesDir, "cost.yaml"), []byte(`
on: cost.budget_burn
when: signal.data.spend_1h_usd > 5.0
for: 5m
then: [ { route_override: { alias: gpt-4o, to: ollama/llama3 } } ]
`), 0o644); err != nil {
		t.Fatal(err)
	}
	sigPath := filepath.Join(dir, "sig.json")
	if err := os.WriteFile(sigPath, []byte(`{"type":"cost.budget_burn","correlation_key":"acme:cost","subject":{"alias":"gpt-4o"},"data":{"spend_1h_usd":9.0}}`), 0o644); err != nil {
		t.Fatal(err)
	}
	db := filepath.Join(dir, "s.db")

	// Without --now: a for: rule can't be satisfied by a single one-shot inject.
	var pend bytes.Buffer
	run([]string{"inject", "--rules", rulesDir, "--db", db, sigPath}, &pend)
	if strings.Contains(pend.String(), "applied") {
		t.Fatalf("for: rule should be pending without --now, got: %s", pend.String())
	}

	// With --now: it fires and audits.
	var out bytes.Buffer
	if code := run([]string{"inject", "--now", "--rules", rulesDir, "--db", db, sigPath}, &out); code != 0 {
		t.Fatalf("inject --now exit %d: %s", code, out.String())
	}
	s := out.String()
	if !strings.Contains(s, "applied") || strings.Contains(s, "pending_for_window") {
		t.Fatalf("expected applied (not pending) with --now, got: %s", s)
	}
	if !strings.Contains(s, "route_override target=gpt-4o") {
		t.Fatalf("expected route_override in output, got: %s", s)
	}

	var audit bytes.Buffer
	run([]string{"audit", "tail", "--db", db}, &audit)
	if !strings.Contains(audit.String(), "verified") || strings.Contains(audit.String(), "(0 entries)") {
		t.Fatalf("audit should show a verified non-empty chain, got: %s", audit.String())
	}
}
