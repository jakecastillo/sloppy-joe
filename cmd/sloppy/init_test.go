package main

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/sloppyjoe/sloppy/doctor"
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

// TestInitCreatesRulesDir asserts the scaffold's `rules: [./rules]` resolves: init
// creates the directory plus a commented sample, and doctor's rules check passes on
// it (an empty rules dir is informational, not a failure).
func TestInitCreatesRulesDir(t *testing.T) {
	dir := t.TempDir()
	cfg := filepath.Join(dir, "sloppy.yaml")

	var out bytes.Buffer
	if rc := cmdInit([]string{"--config", cfg}, &out); rc != 0 {
		t.Fatalf("init rc=%d out=%s", rc, out.String())
	}

	rulesDir := filepath.Join(dir, "rules")
	info, err := os.Stat(rulesDir)
	if err != nil || !info.IsDir() {
		t.Fatalf("init should create %s as a directory: err=%v", rulesDir, err)
	}
	// The starter sample must NOT be a loadable rule file (so the dir stays "empty"
	// from the loader's view and doctor doesn't trip on an unparseable sample).
	if _, err := os.Stat(filepath.Join(rulesDir, "example.yaml.sample")); err != nil {
		t.Fatalf("init should drop a commented sample: %v", err)
	}

	// Doctor's rules check on the fresh scaffold dir must pass.
	if c := doctor.CheckRules(rulesDir); !c.OK {
		t.Fatalf("doctor rules check should pass on fresh scaffold: %+v", c)
	}
}

// TestInitScaffoldDoctorLiteLLMDisabled guards the other half of the bead: the
// scaffold ships litellm disabled, so doctor's litellm probe must be informational
// (OK) even though the scaffold URL points at a port nothing is listening on.
func TestInitScaffoldDoctorLiteLLMDisabled(t *testing.T) {
	// The scaffold disables litellm with a localhost URL; mirror that here.
	if c := doctor.CheckLiteLLM(false, "http://localhost:4000"); !c.OK {
		t.Fatalf("disabled litellm must not fail doctor on the fresh scaffold: %+v", c)
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
