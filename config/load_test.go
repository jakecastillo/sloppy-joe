package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func writeFile(t *testing.T, path, content string) {
	t.Helper()
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

func TestLoadRulesMixedExtensions(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, filepath.Join(dir, "a.yaml"), "on: x\nwhen: \"true\"\nthen: [ { page: {} } ]\n")
	writeFile(t, filepath.Join(dir, "b.yml"), "on: y\nwhen: \"true\"\nthen: [ { page: {} } ]\n")
	writeFile(t, filepath.Join(dir, "note.txt"), "ignored")
	rs, err := LoadRules(dir)
	if err != nil {
		t.Fatal(err)
	}
	if len(rs) != 2 {
		t.Fatalf("want 2 rules (yaml+yml, not txt), got %d", len(rs))
	}
}

func TestLoadRulesEmptyDir(t *testing.T) {
	_, err := LoadRules(t.TempDir())
	if err == nil || !strings.Contains(err.Error(), "no rules found") {
		t.Fatalf("want 'no rules found', got %v", err)
	}
}

func TestLoadRulesMalformedNamesFile(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, filepath.Join(dir, "broken.yaml"), "on: x\nwhen:\nthen: not-a-list\n")
	_, err := LoadRules(dir)
	if err == nil || !strings.Contains(err.Error(), "broken.yaml") {
		t.Fatalf("error should name the offending file, got %v", err)
	}
}

func TestLoadSignalsJSONLBadLineNumber(t *testing.T) {
	dir := t.TempDir()
	p := filepath.Join(dir, "f.jsonl")
	writeFile(t, p, `{"type":"a"}`+"\n"+`{bad json}`+"\n"+`{"type":"c"}`+"\n")
	_, err := LoadSignalsJSONL(p)
	if err == nil || !strings.Contains(err.Error(), "line 2") {
		t.Fatalf("error should name line 2, got %v", err)
	}
}

func TestLoadSignalsJSONLEmpty(t *testing.T) {
	dir := t.TempDir()
	p := filepath.Join(dir, "empty.jsonl")
	writeFile(t, p, "\n  \n")
	_, err := LoadSignalsJSONL(p)
	if err == nil || !strings.Contains(err.Error(), "no signals") {
		t.Fatalf("want 'no signals', got %v", err)
	}
}

func TestOpenStore(t *testing.T) {
	s, err := OpenStore("sqlite", filepath.Join(t.TempDir(), "s.db"), "")
	if err != nil {
		t.Fatalf("sqlite: %v", err)
	}
	s.Close()
	if _, err := OpenStore("redis", "", ""); err == nil {
		t.Fatal("redis without address should error")
	}
	if _, err := OpenStore("bogus", "x", ""); err == nil {
		t.Fatal("unknown store should error")
	}
}
