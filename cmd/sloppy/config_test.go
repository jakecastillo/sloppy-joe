package main

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestConfigValidateGoodAndBad(t *testing.T) {
	dir := t.TempDir()
	good := filepath.Join(dir, "good.yaml")
	if err := os.WriteFile(good, []byte("version: 1\nstore: { kind: sqlite }\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	var out bytes.Buffer
	if rc := run([]string{"config", "validate", "--config", good}, &out); rc != 0 {
		t.Fatalf("good config should validate, rc=%d out=%s", rc, out.String())
	}

	bad := filepath.Join(dir, "bad.yaml")
	if err := os.WriteFile(bad, []byte("version: 9\nstore: { kind: mongo }\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	out.Reset()
	if rc := run([]string{"config", "validate", "--config", bad}, &out); rc == 0 {
		t.Fatalf("bad config should fail, out=%s", out.String())
	}
}

func TestConfigShowNeverPrintsResolvedToken(t *testing.T) {
	dir := t.TempDir()
	cfg := filepath.Join(dir, "sloppy.yaml")
	content := "version: 1\n" +
		"platforms:\n" +
		"  litellm: { enabled: true, url: http://localhost:4000, token_env: SLOPPY_TOKEN_LITELLM }\n"
	if err := os.WriteFile(cfg, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
	t.Setenv("SLOPPY_TOKEN_LITELLM", "leaky-secret-value")
	var out bytes.Buffer
	if rc := run([]string{"config", "show", "--config", cfg}, &out); rc != 0 {
		t.Fatalf("config show rc=%d out=%s", rc, out.String())
	}
	if strings.Contains(out.String(), "leaky-secret-value") {
		t.Fatalf("config show leaked the resolved token:\n%s", out.String())
	}
	if !strings.Contains(out.String(), "SLOPPY_TOKEN_LITELLM") {
		t.Fatalf("config show should print the token_env name:\n%s", out.String())
	}
}
