package main

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// init_test calls cmdInit directly (not via run) so it does not depend on the
// command switch in main.go, which is edited concurrently.

func TestInitScaffoldNoClobberAndValidates(t *testing.T) {
	dir := t.TempDir()
	cfg := filepath.Join(dir, "sloppy.yaml")

	var out bytes.Buffer
	if rc := cmdInit([]string{"--config", cfg}, &out); rc != 0 {
		t.Fatalf("init rc=%d out=%s", rc, out.String())
	}
	for _, p := range []string{cfg, filepath.Join(dir, ".env.sample"), filepath.Join(dir, "sloppy.key")} {
		if _, err := os.Stat(p); err != nil {
			t.Fatalf("init should have written %s: %v", p, err)
		}
	}

	// The scaffolded config must itself validate (incl. its enabled recipe).
	var v bytes.Buffer
	if rc := cmdConfig([]string{"validate", "--config", cfg}, &v); rc != 0 {
		t.Fatalf("scaffolded config should validate:\n%s", v.String())
	}

	// Re-run without --force: idempotent no-op, exit 0.
	out.Reset()
	if rc := cmdInit([]string{"--config", cfg}, &out); rc != 0 {
		t.Fatalf("re-init should be a no-op exit 0, got rc=%d", rc)
	}
	if !strings.Contains(out.String(), "already initialized") {
		t.Fatalf("re-init should report already initialized:\n%s", out.String())
	}

	// --force regenerates without error.
	out.Reset()
	if rc := cmdInit([]string{"--config", cfg, "--force"}, &out); rc != 0 {
		t.Fatalf("init --force rc=%d out=%s", rc, out.String())
	}
}

func TestInitNonInteractiveInCI(t *testing.T) {
	// init must never block on input even with no TTY and no --yes.
	dir := t.TempDir()
	cfg := filepath.Join(dir, "sloppy.yaml")
	var out bytes.Buffer
	if rc := cmdInit([]string{"--config", cfg}, &out); rc != 0 {
		t.Fatalf("init must succeed non-interactively: rc=%d", rc)
	}
}
