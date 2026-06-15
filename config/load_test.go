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

// rawDBNoise is the raw SQLite/OS text that must never reach the user.
var rawDBNoise = []string{
	"(14)", "(26)", "unable to open database file", "file is not a database",
	"GetFileAttributesEx", "system cannot find", "no such file or directory",
}

func assertNoRawNoise(t *testing.T, msg string, noise []string) {
	t.Helper()
	for _, raw := range noise {
		if strings.Contains(msg, raw) {
			t.Errorf("error should not leak raw text %q: %q", raw, msg)
		}
	}
}

func TestLoadRulesMissingPathFriendly(t *testing.T) {
	missing := filepath.Join(t.TempDir(), "nope")
	_, err := LoadRules(missing)
	if err == nil {
		t.Fatal("missing rules path should error")
	}
	msg := err.Error()
	if !strings.Contains(msg, missing) {
		t.Errorf("error should name the missing path %q: %q", missing, msg)
	}
	if !strings.Contains(msg, "--rules") {
		t.Errorf("error should mention the --rules remedy: %q", msg)
	}
	assertNoRawNoise(t, msg, []string{"GetFileAttributesEx", "system cannot find", "no such file or directory"})
}

func TestLoadSignalMissingPathFriendly(t *testing.T) {
	missing := filepath.Join(t.TempDir(), "nope.json")
	_, err := LoadSignal(missing)
	if err == nil {
		t.Fatal("missing signal path should error")
	}
	msg := err.Error()
	if !strings.Contains(msg, missing) || !strings.Contains(msg, "signal file") {
		t.Errorf("error should name the missing signal file: %q", msg)
	}
	assertNoRawNoise(t, msg, []string{"open ", "GetFileAttributesEx", "system cannot find", "no such file or directory"})
}

func TestLoadSignalMalformedJSONNamesFile(t *testing.T) {
	dir := t.TempDir()
	p := filepath.Join(dir, "sig.json")
	writeFile(t, p, "{not valid json")
	_, err := LoadSignal(p)
	if err == nil {
		t.Fatal("malformed signal JSON should error")
	}
	msg := err.Error()
	if !strings.Contains(msg, p) {
		t.Errorf("error should name the malformed file %q: %q", p, msg)
	}
	if !strings.Contains(msg, "malformed JSON") {
		t.Errorf("error should say malformed JSON: %q", msg)
	}
}

func TestLoadSignalsJSONLMissingPathFriendly(t *testing.T) {
	missing := filepath.Join(t.TempDir(), "nope.jsonl")
	_, err := LoadSignalsJSONL(missing)
	if err == nil {
		t.Fatal("missing jsonl path should error")
	}
	msg := err.Error()
	if !strings.Contains(msg, missing) || !strings.Contains(msg, "signal file") {
		t.Errorf("error should name the missing signal file: %q", msg)
	}
	assertNoRawNoise(t, msg, []string{"open ", "GetFileAttributesEx", "system cannot find", "no such file or directory"})
}

func TestOpenStoreMissingParentDirFriendly(t *testing.T) {
	// Parent directory does not exist: sqlite won't create intermediate dirs.
	dbPath := filepath.Join(t.TempDir(), "no-such-dir", "state.db")
	_, err := OpenStore("sqlite", dbPath, "")
	if err == nil {
		t.Fatal("opening a db under a missing parent dir should error")
	}
	msg := err.Error()
	if !strings.Contains(msg, dbPath) {
		t.Errorf("error should name the db path %q: %q", dbPath, msg)
	}
	if !strings.Contains(msg, "parent directory") {
		t.Errorf("error should blame the missing parent directory: %q", msg)
	}
	assertNoRawNoise(t, msg, rawDBNoise)
}

func TestOpenStoreGarbageDBFriendly(t *testing.T) {
	// A non-sloppy file sitting at the db path: sqlite reports SQLITE_NOTADB.
	dbPath := filepath.Join(t.TempDir(), "garbage.db")
	writeFile(t, dbPath, "this is plainly not a sqlite database, just text")
	_, err := OpenStore("sqlite", dbPath, "")
	if err == nil {
		t.Fatal("opening a garbage file as a db should error")
	}
	msg := err.Error()
	if !strings.Contains(msg, dbPath) {
		t.Errorf("error should name the db path %q: %q", dbPath, msg)
	}
	if !strings.Contains(msg, "not a sloppy state database") {
		t.Errorf("error should explain the file is not a sloppy db: %q", msg)
	}
	assertNoRawNoise(t, msg, rawDBNoise)
}
