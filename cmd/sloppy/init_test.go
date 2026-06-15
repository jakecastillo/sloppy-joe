package main

import (
	"bytes"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/sloppyjoe/sloppy/config"
	"github.com/sloppyjoe/sloppy/core"
	"github.com/sloppyjoe/sloppy/doctor"
	"github.com/sloppyjoe/sloppy/ledger"
	"github.com/sloppyjoe/sloppy/replay"
	"github.com/sloppyjoe/sloppy/rules"
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
// creates the directory with an ACTIVE starter.yaml (so the install is not a
// no-rule-fired dead-end) plus a commented *.yaml.sample authoring template the
// loader ignores, and doctor's rules check passes on it.
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
	// An ACTIVE starter rule must be present so minute one fires something.
	if _, err := os.Stat(filepath.Join(rulesDir, "starter.yaml")); err != nil {
		t.Fatalf("init should write an active starter.yaml: %v", err)
	}
	// The commented sample must NOT be a loadable rule file (the loader skips
	// non-.yaml/.yml so doctor doesn't trip on the unparseable template).
	if _, err := os.Stat(filepath.Join(rulesDir, "example.yaml.sample")); err != nil {
		t.Fatalf("init should drop a commented sample: %v", err)
	}

	// Doctor's rules check on the fresh scaffold dir must pass.
	if c := doctor.CheckRules(rulesDir); !c.OK {
		t.Fatalf("doctor rules check should pass on fresh scaffold: %+v", c)
	}
}

// TestInitStarterRuleFiresImmediately is the core of the dead-end fix: the active
// starter rule init scaffolds must actually fire on a matching cost.budget_burn
// signal (instead of the old "no rule fired" cold start). It loads the scaffolded
// rules dir, validates the starter rule, and replays the shipped example signal
// through the reconciler to confirm at least one intent matches.
func TestInitStarterRuleFiresImmediately(t *testing.T) {
	dir := t.TempDir()
	cfg := filepath.Join(dir, "sloppy.yaml")

	var out bytes.Buffer
	if rc := cmdInit([]string{"--config", cfg}, &out); rc != 0 {
		t.Fatalf("init rc=%d out=%s", rc, out.String())
	}
	rulesDir := filepath.Join(dir, "rules")

	// The scaffolded rules dir must load at least one ACTIVE rule, and each must
	// validate (good CEL predicate + known action kinds).
	rs, err := config.LoadRules(rulesDir)
	if err != nil {
		t.Fatalf("scaffolded rules dir should load an active rule: %v", err)
	}
	if len(rs) == 0 {
		t.Fatalf("scaffolded rules dir loaded 0 rules — minute one is still a dead-end")
	}
	for i, r := range rs {
		if err := rules.Validate(r); err != nil {
			t.Fatalf("scaffolded rule %d should validate: %v", i, err)
		}
	}

	// Replay the shipped cost-spike signal through the reconciler: the starter rule
	// must produce at least one intent, proving the cold start now fires.
	rec, err := rules.NewReconciler(rs)
	if err != nil {
		t.Fatalf("reconciler: %v", err)
	}
	sig := core.Signal{
		Type:           "cost.budget_burn",
		CorrelationKey: "acme:cost",
		Subject:        core.Subject{Alias: "gpt-4o"},
		Data:           map[string]any{"spend_1h_usd": 9.0},
	}
	res := replay.Run(rec, []core.Signal{sig})
	fired := 0
	for _, r := range res {
		fired += len(r.Intents)
	}
	if fired == 0 {
		t.Fatalf("starter rule should fire on the cost-spike signal but produced 0 intents")
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

// TestInitWritesPricebookSample asserts init drops a pricebook.yaml.sample beside the
// config (so the cost-guard recipe has non-zero spend to guard against), that it parses
// as a valid price book, that a re-run does not clobber an operator's edits, and that it
// is written with 0o644 perms.
func TestInitWritesPricebookSample(t *testing.T) {
	dir := t.TempDir()
	cfg := filepath.Join(dir, "sloppy.yaml")
	pb := filepath.Join(dir, "pricebook.yaml.sample")

	var out bytes.Buffer
	if rc := cmdInit([]string{"--config", cfg}, &out); rc != 0 {
		t.Fatalf("init rc=%d out=%s", rc, out.String())
	}

	// The sample must exist and parse as a real price book with at least one model.
	b, err := os.ReadFile(pb)
	if err != nil {
		t.Fatalf("init should write %s: %v", pb, err)
	}
	book, err := ledger.LoadPriceBook(b)
	if err != nil {
		t.Fatalf("pricebook sample must parse as a price book: %v", err)
	}
	if len(book) == 0 {
		t.Fatalf("pricebook sample should list at least one model")
	}

	// 0o644 perms. Windows does not honor the group/other bits, so assert the full
	// mode only off Windows and just the owner-readable/writable bits on Windows.
	info, err := os.Stat(pb)
	if err != nil {
		t.Fatalf("stat %s: %v", pb, err)
	}
	if runtime.GOOS == "windows" {
		if info.Mode().Perm()&0o600 != 0o600 {
			t.Fatalf("pricebook sample should be owner read+write, got %v", info.Mode().Perm())
		}
	} else if info.Mode().Perm() != 0o644 {
		t.Fatalf("pricebook sample should be 0o644, got %v", info.Mode().Perm())
	}

	// Operator edits a copy; a re-run (and --force) must not clobber it.
	const edited = "my-model:\n  input_per_1k: 1.0\n  output_per_1k: 2.0\n"
	if err := os.WriteFile(pb, []byte(edited), 0o644); err != nil {
		t.Fatalf("rewrite sample: %v", err)
	}
	out.Reset()
	if rc := cmdInit([]string{"--config", cfg, "--force"}, &out); rc != 0 {
		t.Fatalf("init --force rc=%d out=%s", rc, out.String())
	}
	got, err := os.ReadFile(pb)
	if err != nil {
		t.Fatalf("read sample after re-run: %v", err)
	}
	if string(got) != edited {
		t.Fatalf("pricebook sample was clobbered on re-run:\n%s", string(got))
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
