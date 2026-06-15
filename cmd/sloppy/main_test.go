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
	// An unknown top-level command must echo the valid command set so the user
	// can recover (mirrors the rules/platform usage lines).
	got := out.String()
	if !strings.Contains(got, "unknown command: frobnicate") {
		t.Fatalf("missing unknown-command line: %q", got)
	}
	if !strings.Contains(got, usageLine) {
		t.Fatalf("unknown command should echo the valid command set: %q", got)
	}
}

func TestRunUnknownConfigSubcommand(t *testing.T) {
	var out bytes.Buffer
	if code := run([]string{"config", "frobnicate"}, &out); code != 2 {
		t.Fatalf("exit %d, want 2", code)
	}
	got := out.String()
	if !strings.Contains(got, "unknown config subcommand: frobnicate") {
		t.Fatalf("missing unknown-subcommand line: %q", got)
	}
	// Must echo the valid config subcommand set.
	for _, want := range []string{"show", "validate", "schema"} {
		if !strings.Contains(got, want) {
			t.Fatalf("unknown config subcommand should list %q: %q", want, got)
		}
	}
}

func TestRunUnknownRecipeSubcommand(t *testing.T) {
	var out bytes.Buffer
	if code := run([]string{"recipe", "frobnicate"}, &out); code != 2 {
		t.Fatalf("exit %d, want 2", code)
	}
	got := out.String()
	if !strings.Contains(got, "unknown recipe subcommand: frobnicate") {
		t.Fatalf("missing unknown-subcommand line: %q", got)
	}
	// Must echo the valid recipe subcommand set.
	for _, want := range []string{"list", "show"} {
		if !strings.Contains(got, want) {
			t.Fatalf("unknown recipe subcommand should list %q: %q", want, got)
		}
	}
}

// A missing --replay fixture must report the fixture path, not a misleading
// "no rules found": the fixture is loaded before the rules.
func TestTestReplayMissingFixture(t *testing.T) {
	dir := t.TempDir()
	missing := filepath.Join(dir, "does-not-exist.jsonl")
	var out bytes.Buffer
	code := run([]string{"test", "--replay", missing, "--rules", filepath.Join(dir, "no-rules-here")}, &out)
	if code == 0 {
		t.Fatalf("missing fixture should be non-zero exit: %q", out.String())
	}
	got := out.String()
	if !strings.Contains(got, "does-not-exist.jsonl") {
		t.Fatalf("error should name the missing fixture, got: %q", got)
	}
	if strings.Contains(got, "no rules found") {
		t.Fatalf("error should be about the fixture, not the rules: %q", got)
	}
}

func TestRecipeListHeader(t *testing.T) {
	dir := t.TempDir()
	var out bytes.Buffer
	if code := run([]string{"recipe", "list", "--config", filepath.Join(dir, "sloppy.yaml")}, &out); code != 0 {
		t.Fatalf("recipe list exit %d: %s", code, out.String())
	}
	got := out.String()
	// One-line header that explains the "available" status.
	if !strings.Contains(got, "recipes") || !strings.Contains(got, "available") {
		t.Fatalf("recipe list should print a header explaining 'available': %q", got)
	}
}

func TestPlatformListHeader(t *testing.T) {
	dir := t.TempDir()
	// A config with at least one platform so the header (not the empty message) prints.
	cfg := filepath.Join(dir, "sloppy.yaml")
	if err := os.WriteFile(cfg, []byte("platforms:\n  github:\n    enabled: true\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	var out bytes.Buffer
	if code := run([]string{"platform", "list", "--config", cfg}, &out); code != 0 {
		t.Fatalf("platform list exit %d: %s", code, out.String())
	}
	got := out.String()
	if !strings.Contains(got, "platforms") {
		t.Fatalf("platform list should print a header: %q", got)
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
