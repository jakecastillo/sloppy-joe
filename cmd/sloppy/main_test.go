package main

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestRunVersion(t *testing.T) {
	var out bytes.Buffer
	if code := run([]string{"version"}, &out); code != 0 {
		t.Fatalf("exit %d", code)
	}
	if !strings.Contains(out.String(), "sloppy") {
		t.Fatalf("version output missing: %q", out.String())
	}
}

func TestRunUnknown(t *testing.T) {
	var out bytes.Buffer
	if code := run([]string{"frobnicate"}, &out); code == 0 {
		t.Fatal("unknown command should be non-zero exit")
	}
}

func TestRunHelp(t *testing.T) {
	for _, arg := range []string{"help", "-h", "--help"} {
		t.Run(arg, func(t *testing.T) {
			var out bytes.Buffer
			if code := run([]string{arg}, &out); code != 0 {
				t.Fatalf("%q: exit %d, want 0: %s", arg, code, out.String())
			}
			if got := out.String(); !strings.Contains(got, "usage: sloppy") {
				t.Fatalf("%q: missing usage: %q", arg, got)
			}
			if strings.Contains(out.String(), "unknown command") {
				t.Fatalf("%q: should not print 'unknown command': %q", arg, out.String())
			}
		})
	}
}

func TestInjectThenAudit(t *testing.T) {
	dir := t.TempDir()
	rulesDir := filepath.Join(dir, "rules")
	if err := os.MkdirAll(rulesDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(rulesDir, "cost.yaml"), []byte(`
on: cost.budget_burn
when: signal.data.spend_1h_usd > 5.0
then: [ { route_override: { alias: gpt-4o, to: ollama/llama3, ttl: 30m } } ]
with: { dry_run: false }
`), 0o644); err != nil {
		t.Fatal(err)
	}
	sigPath := filepath.Join(dir, "sig.json")
	if err := os.WriteFile(sigPath, []byte(`{"type":"cost.budget_burn","correlation_key":"acme:cost","subject":{"alias":"gpt-4o"},"data":{"spend_1h_usd":9.0}}`), 0o644); err != nil {
		t.Fatal(err)
	}
	db := filepath.Join(dir, "s.db")

	var out bytes.Buffer
	if code := run([]string{"inject", "--rules", rulesDir, "--db", db, sigPath}, &out); code != 0 {
		t.Fatalf("inject exit nonzero: %s", out.String())
	}
	if !strings.Contains(out.String(), "applied") {
		t.Fatalf("expected 'applied' in output: %s", out.String())
	}

	// Replay → idempotent skip.
	var out2 bytes.Buffer
	run([]string{"inject", "--rules", rulesDir, "--db", db, sigPath}, &out2)
	if !strings.Contains(out2.String(), "skipped") {
		t.Fatalf("expected idempotent skip on replay: %s", out2.String())
	}

	// Audit shows entries + verified chain.
	var out3 bytes.Buffer
	if code := run([]string{"audit", "tail", "--db", db}, &out3); code != 0 {
		t.Fatalf("audit exit nonzero: %s", out3.String())
	}
	if !strings.Contains(out3.String(), "verified") {
		t.Fatalf("audit should verify chain: %s", out3.String())
	}
}
