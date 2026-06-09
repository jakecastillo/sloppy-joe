package main

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestRulesValidate(t *testing.T) {
	dir := t.TempDir()

	good := filepath.Join(dir, "good")
	if err := os.MkdirAll(good, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(good, "r.yaml"), []byte(`
on: cost.budget_burn
when: signal.data.spend_1h_usd > 5.0
then: [ { route_override: { alias: gpt-4o, to: ollama/llama3 } } ]
with: { intent_budget: "3/h" }
`), 0o644); err != nil {
		t.Fatal(err)
	}
	var out bytes.Buffer
	if code := run([]string{"rules", "validate", good}, &out); code != 0 {
		t.Fatalf("valid dir should exit 0, got %d: %s", code, out.String())
	}
	if !strings.Contains(out.String(), "valid") {
		t.Fatalf("expected 'valid' in output: %s", out.String())
	}

	bad := filepath.Join(dir, "bad")
	if err := os.MkdirAll(bad, 0o755); err != nil {
		t.Fatal(err)
	}
	// Malformed CEL `when` — parses as YAML, fails to compile.
	if err := os.WriteFile(filepath.Join(bad, "r.yaml"), []byte("on: cost.budget_burn\nwhen: signal.data.spend_1h_usd >\nthen: [ { route_override: {} } ]\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	var out2 bytes.Buffer
	if code := run([]string{"rules", "validate", bad}, &out2); code == 0 {
		t.Fatalf("invalid dir should exit non-zero: %s", out2.String())
	}
}
