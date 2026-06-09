package config

import (
	"path/filepath"
	"testing"
)

func TestLoadFileMissingReturnsDefaults(t *testing.T) {
	f, existed, err := LoadFile(filepath.Join(t.TempDir(), "nope.yaml"))
	if err != nil {
		t.Fatal(err)
	}
	if existed {
		t.Fatal("missing file should report existed=false")
	}
	if f.Version != 1 || f.Store.Kind != "sqlite" || f.Server.Addr != ":8723" {
		t.Fatalf("defaults not applied: %+v", f)
	}
	if len(f.Rules) != 1 || f.Rules[0] != "rules" {
		t.Fatalf("default rules wrong: %v", f.Rules)
	}
	if f.Engine.FailMode.Default != "closed" || f.Engine.FailMode.Notify != "open" {
		t.Fatalf("default fail_mode wrong: %+v", f.Engine.FailMode)
	}
}

func TestLoadFileParsesAndFillsDefaults(t *testing.T) {
	p := filepath.Join(t.TempDir(), "sloppy.yaml")
	writeFile(t, p, "version: 1\n"+
		"store: { kind: redis, redis_addr: localhost:6379 }\n"+
		"platforms:\n"+
		"  slack: { enabled: true, channel: \"#ops\", token_env: SLOPPY_TOKEN_SLACK }\n")
	f, existed, err := LoadFile(p)
	if err != nil {
		t.Fatal(err)
	}
	if !existed {
		t.Fatal("existed should be true for a present file")
	}
	if f.Store.Kind != "redis" || f.Store.RedisAddr != "localhost:6379" {
		t.Fatalf("store not parsed: %+v", f.Store)
	}
	if f.Server.Addr != ":8723" { // default filled even though file omitted it
		t.Fatalf("default addr not filled: %q", f.Server.Addr)
	}
	if sp := f.Platforms["slack"]; !sp.Enabled || sp.TokenEnv != "SLOPPY_TOKEN_SLACK" {
		t.Fatalf("slack platform: %+v", sp)
	}
}

func TestLoadFileRejectsUnknownKey(t *testing.T) {
	p := filepath.Join(t.TempDir(), "sloppy.yaml")
	writeFile(t, p, "version: 1\nstoree: { kind: sqlite }\n")
	if _, _, err := LoadFile(p); err == nil {
		t.Fatal("unknown key should error under strict decoding")
	}
}
