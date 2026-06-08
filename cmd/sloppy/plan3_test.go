package main

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestReplayCmd(t *testing.T) {
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
	fixture := filepath.Join(dir, "f.jsonl")
	if err := os.WriteFile(fixture, []byte(
		`{"id":"s1","type":"cost.budget_burn","subject":{"alias":"gpt-4o"},"data":{"spend_1h_usd":9.0}}`+"\n"+
			`{"id":"s2","type":"cost.budget_burn","data":{"spend_1h_usd":1.0}}`+"\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	var out bytes.Buffer
	if code := run([]string{"test", "--replay", fixture, "--rules", rulesDir}, &out); code != 0 {
		t.Fatalf("exit %d: %s", code, out.String())
	}
	s := out.String()
	if !strings.Contains(s, "would route_override") {
		t.Fatalf("expected s1 to fire: %s", s)
	}
	if !strings.Contains(s, "1 intent(s) would fire") {
		t.Fatalf("expected summary of 1 fire: %s", s)
	}
	if !strings.Contains(s, "(no rule)") {
		t.Fatalf("expected s2 to show no rule: %s", s)
	}
}

func TestDoctorCmd(t *testing.T) {
	dir := t.TempDir()
	rulesDir := filepath.Join(dir, "rules")
	if err := os.MkdirAll(rulesDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(rulesDir, "r.yaml"), []byte(`
on: cost.budget_burn
when: signal.data.spend_1h_usd > 5.0
then: [ { page: { slack: "#x" } } ]
`), 0o644); err != nil {
		t.Fatal(err)
	}
	t.Setenv("SLOPPY_LITELLM_URL", "")
	var out bytes.Buffer
	if code := run([]string{"doctor", "--rules", rulesDir, "--db", filepath.Join(dir, "d.db")}, &out); code != 0 {
		t.Fatalf("doctor exit %d: %s", code, out.String())
	}
	if !strings.Contains(out.String(), "rules") || !strings.Contains(out.String(), "✓") {
		t.Fatalf("doctor output unexpected: %s", out.String())
	}
}
