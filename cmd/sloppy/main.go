// Command sloppy is the Sloppy Joe CLI.
package main

import (
	"context"
	"flag"
	"fmt"
	"io"
	"os"
	"sort"
	"strings"

	sloppyjoe "github.com/sloppyjoe/sloppy"
	"github.com/sloppyjoe/sloppy/actuator"
	"github.com/sloppyjoe/sloppy/bootstrap"
	"github.com/sloppyjoe/sloppy/config"
	"github.com/sloppyjoe/sloppy/doctor"
	"github.com/sloppyjoe/sloppy/engine"
	"github.com/sloppyjoe/sloppy/intent"
	"github.com/sloppyjoe/sloppy/recipe"
	"github.com/sloppyjoe/sloppy/replay"
	"github.com/sloppyjoe/sloppy/rules"
	"github.com/sloppyjoe/sloppy/state"
)

func run(args []string, out io.Writer) int {
	if len(args) == 0 {
		fmt.Fprintln(out, "usage: sloppy <init|version|inject|rules|audit|test|doctor|config|platform|recipe>")
		return 2
	}
	switch args[0] {
	case "init":
		return cmdInit(args[1:], out)
	case "version":
		fmt.Fprintf(out, "sloppy %s\n", sloppyjoe.Version)
		return 0
	case "inject":
		return cmdInject(args[1:], out)
	case "rules":
		return cmdRules(args[1:], out)
	case "audit":
		return cmdAudit(args[1:], out)
	case "test":
		return cmdTest(args[1:], out)
	case "doctor":
		return cmdDoctor(args[1:], out)
	case "config":
		return cmdConfig(args[1:], out)
	case "platform":
		return cmdPlatform(args[1:], out)
	case "recipe":
		return cmdRecipe(args[1:], out)
	default:
		fmt.Fprintf(out, "unknown command: %s\n", args[0])
		return 2
	}
}

func cmdInject(args []string, out io.Writer) int {
	fs := flag.NewFlagSet("inject", flag.ContinueOnError)
	fs.SetOutput(out)
	rulesPath := fs.String("rules", "rules", "rules dir or file")
	dbPath := fs.String("db", "sloppy.db", "sqlite db path")
	cfgPath := fs.String("config", "sloppy.yaml", "path to sloppy.yaml (platform wiring)")
	keyPath := fs.String("key", "sloppy.key", "ed25519 signing key file (created if absent; exports <key>.pub)")
	now := fs.Bool("now", false, "fire matching rules immediately, bypassing for: windows (one-shot)")
	if err := fs.Parse(args); err != nil {
		return 2
	}
	rest := fs.Args()
	if len(rest) != 1 {
		fmt.Fprintln(out, "usage: sloppy inject [--rules dir] [--db path] [--key file] <signal.json>")
		return 2
	}
	sig, err := config.LoadSignal(rest[0])
	if err != nil {
		fmt.Fprintf(out, "error: %v\n", err)
		return 1
	}
	rs, err := config.LoadRules(*rulesPath)
	if err != nil {
		fmt.Fprintf(out, "error: %v\n", err)
		return 1
	}
	rec, err := rules.NewReconciler(rs)
	if err != nil {
		fmt.Fprintf(out, "error: %v\n", err)
		return 1
	}
	st, err := state.OpenSQLite(*dbPath)
	if err != nil {
		fmt.Fprintf(out, "error: %v\n", err)
		return 1
	}
	defer st.Close()
	// Persist the signing key (and export its public key) so the signatures this
	// writes to the audit chain are later verifiable via `audit --verify-sigs`.
	signer, err := intent.LoadOrCreateSigner(*keyPath)
	if err != nil {
		fmt.Fprintf(out, "error: %v\n", err)
		return 1
	}
	var opts []engine.Option
	if *now {
		opts = append(opts, engine.WithImmediate())
	}
	reg, err := registryFromConfig(*cfgPath, out)
	if err != nil {
		fmt.Fprintf(out, "error: %v\n", err)
		return 1
	}
	e := engine.New(rec, reg, st, signer, opts...)
	results, _ := e.Handle(context.Background(), sig)
	if len(results) == 0 {
		fmt.Fprintln(out, "🥪 no rule fired for this signal")
		return 0
	}
	for _, r := range results {
		fmt.Fprintf(out, "%-18s %s target=%s\n", r.Outcome, r.Intent.Kind, r.Intent.Target)
	}
	return 0
}

func cmdAudit(args []string, out io.Writer) int {
	if len(args) > 0 && args[0] == "tail" {
		args = args[1:]
	}
	fs := flag.NewFlagSet("audit", flag.ContinueOnError)
	fs.SetOutput(out)
	dbPath := fs.String("db", "sloppy.db", "sqlite db path")
	keyPath := fs.String("key", "sloppy.key", "signing key whose <key>.pub verifies signatures (--verify-sigs)")
	pubPath := fs.String("pubkey", "", "explicit public key file (defaults to <key>.pub)")
	verifySigs := fs.Bool("verify-sigs", false, "verify each persisted intent signature against the public key")
	if err := fs.Parse(args); err != nil {
		return 2
	}
	st, err := state.OpenSQLite(*dbPath)
	if err != nil {
		fmt.Fprintf(out, "error: %v\n", err)
		return 1
	}
	defer st.Close()
	entries, err := st.Audit(context.Background())
	if err != nil {
		fmt.Fprintf(out, "error: %v\n", err)
		return 1
	}
	for _, e := range entries {
		fmt.Fprintf(out, "%4d  %-16s  %s\n", e.Seq, e.Kind, e.Detail)
	}
	status := "verified ✓"
	if !st.VerifyAudit(context.Background()) {
		status = "TAMPERED ✗"
	}
	fmt.Fprintf(out, "chain: %s (%d entries)\n", status, len(entries))
	if *verifySigs {
		return verifyAuditSigs(out, entries, *keyPath, *pubPath)
	}
	return 0
}

// verifyAuditSigs loads the persisted public key and verifies every signed audit
// entry's signature against the recomputed canonical bytes. It reports
// verified/failed counts and returns a non-zero exit code if any signature fails
// to verify (or the chain has tampered hashes), so it can gate CI.
func verifyAuditSigs(out io.Writer, entries []state.AuditEntry, keyPath, pubPath string) int {
	if pubPath == "" {
		pubPath = intent.PublicKeyPath(keyPath)
	}
	pub, err := intent.LoadVerifierKey(pubPath)
	if err != nil {
		fmt.Fprintf(out, "error: %v\n", err)
		return 1
	}
	verified, failed := 0, 0
	for _, e := range entries {
		ok, found := intent.VerifyAuditDetail(pub, e.Detail)
		if !found {
			continue // not a signed entry (e.g. intent.reverted) — nothing to check
		}
		if ok {
			verified++
			continue
		}
		failed++
		fmt.Fprintf(out, "✗ seq %d (%s): signature verification FAILED\n", e.Seq, e.Kind)
	}
	fmt.Fprintf(out, "signatures: %d verified, failed=%d\n", verified, failed)
	if failed > 0 {
		return 1
	}
	return 0
}

// registryFromConfig builds the actuator registry from sloppy.yaml (zero-config if
// absent) plus the environment, via the shared bootstrap builder — the same wiring
// sloppyd uses, so the CLI and daemon agree on which platforms are enabled.
func registryFromConfig(cfgPath string, out io.Writer) (*actuator.Registry, error) {
	f, existed, err := config.LoadFile(cfgPath)
	if err != nil {
		return nil, err
	}
	eff := config.Resolve(f, existed, config.FlagOverrides{}, os.Getenv)
	return bootstrap.BuildRegistry(eff, out)
}

// cmdTest deterministically replays a JSONL fixture of signals against the rules
// and prints what WOULD fire — no actuation, no state writes. A CI gate.
func cmdTest(args []string, out io.Writer) int {
	fs := flag.NewFlagSet("test", flag.ContinueOnError)
	fs.SetOutput(out)
	rulesPath := fs.String("rules", "rules", "rules dir or file")
	fixture := fs.String("replay", "", "JSONL fixture of signals to replay")
	if err := fs.Parse(args); err != nil {
		return 2
	}
	if *fixture == "" {
		fmt.Fprintln(out, "usage: sloppy test --replay <fixture.jsonl> [--rules dir]")
		return 2
	}
	rs, err := config.LoadRules(*rulesPath)
	if err != nil {
		fmt.Fprintf(out, "error: %v\n", err)
		return 1
	}
	rec, err := rules.NewReconciler(rs)
	if err != nil {
		fmt.Fprintf(out, "error: %v\n", err)
		return 1
	}
	sigs, err := config.LoadSignalsJSONL(*fixture)
	if err != nil {
		fmt.Fprintf(out, "error: %v\n", err)
		return 1
	}
	fired := 0
	for _, r := range replay.Run(rec, sigs) {
		if !r.Matched {
			fmt.Fprintf(out, "%-14s (no rule)\n", r.SignalID)
			continue
		}
		for _, f := range r.Intents {
			fmt.Fprintf(out, "%-14s would %s target=%s (rule %s)\n", r.SignalID, f.Kind, f.Target, f.Rule)
			fired++
		}
	}
	fmt.Fprintf(out, "replay: %d signal(s), %d intent(s) would fire\n", len(sigs), fired)
	return 0
}

// cmdDoctor runs connectivity/capability checks.
func cmdDoctor(args []string, out io.Writer) int {
	fs := flag.NewFlagSet("doctor", flag.ContinueOnError)
	fs.SetOutput(out)
	rulesPath := fs.String("rules", "rules", "rules dir or file")
	dbPath := fs.String("db", "sloppy.db", "sqlite db path")
	cfgPath := fs.String("config", "sloppy.yaml", "path to sloppy.yaml")
	if err := fs.Parse(args); err != nil {
		return 2
	}
	f, existed, _ := config.LoadFile(*cfgPath)
	eff := config.Resolve(f, existed, config.FlagOverrides{}, os.Getenv)
	litellmURL := ""
	if p, ok := eff.Platforms["litellm"]; ok {
		litellmURL = p.URL
	}
	checks := []doctor.Check{
		doctor.CheckRules(*rulesPath),
		doctor.CheckDB(*dbPath),
		doctor.CheckLedger(*dbPath),
		doctor.CheckPlatforms(eff),
		doctor.CheckLiteLLM(litellmURL),
	}
	if reg, err := bootstrap.BuildRegistry(eff, out); err == nil {
		checks = append(checks, doctor.CheckActuators(reg.Kinds()))
	}
	allOK := true
	for _, c := range checks {
		mark := "✓"
		if !c.OK {
			mark = "✗"
			allOK = false
		}
		fmt.Fprintf(out, "[%s] %-10s %s\n", mark, c.Name, c.Detail)
	}
	if !allOK {
		return 1
	}
	return 0
}

func cmdRules(args []string, out io.Writer) int {
	if len(args) == 0 || args[0] != "validate" {
		fmt.Fprintln(out, "usage: sloppy rules validate [dir|file]")
		return 2
	}
	path := "rules"
	if rest := args[1:]; len(rest) >= 1 {
		path = rest[0]
	}
	rs, err := config.LoadRules(path)
	if err != nil {
		fmt.Fprintf(out, "✗ %v\n", err)
		return 1
	}
	failed := 0
	for i, r := range rs {
		if err := rules.Validate(r); err != nil {
			fmt.Fprintf(out, "✗ rule %d (sha %s): %v\n", i+1, r.SHA, err)
			failed++
		}
	}
	if failed > 0 {
		fmt.Fprintf(out, "%d of %d rule(s) invalid\n", failed, len(rs))
		return 1
	}
	fmt.Fprintf(out, "✓ %d rule(s) valid\n", len(rs))
	return 0
}

// cmdConfig is the read-only config surface: `config show` renders the effective
// merged config (never resolving secrets), `config validate` is an offline CI gate.
func cmdConfig(args []string, out io.Writer) int {
	if len(args) == 0 {
		fmt.Fprintln(out, "usage: sloppy config <show|validate|schema> [--config sloppy.yaml]")
		return 2
	}
	sub := args[0]
	fs := flag.NewFlagSet("config "+sub, flag.ContinueOnError)
	fs.SetOutput(out)
	cfgPath := fs.String("config", "sloppy.yaml", "path to sloppy.yaml")
	prov := fs.Bool("provenance", false, "annotate `config show` with value sources")
	if err := fs.Parse(args[1:]); err != nil {
		return 2
	}
	switch sub {
	case "show":
		f, existed, err := config.LoadFile(*cfgPath)
		if err != nil {
			fmt.Fprintf(out, "error: %v\n", err)
			return 1
		}
		eff := config.Resolve(f, existed, config.FlagOverrides{}, os.Getenv)
		if err := config.RenderEffective(out, eff, *prov); err != nil {
			fmt.Fprintf(out, "error: %v\n", err)
			return 1
		}
		return 0
	case "validate":
		f, existed, err := config.LoadFile(*cfgPath)
		if err != nil {
			fmt.Fprintf(out, "✗ %v\n", err)
			return 1
		}
		probs := config.Validate(f)
		for _, p := range probs {
			fmt.Fprintf(out, "✗ %s\n", p)
		}
		// Render enabled recipes too, so bad params / templates fail the gate.
		eff := config.Resolve(f, existed, config.FlagOverrides{}, os.Getenv)
		if _, rerr := bootstrap.RenderRecipes(eff); rerr != nil {
			fmt.Fprintf(out, "✗ recipes: %v\n", rerr)
			probs = append(probs, config.Problem{Path: "recipes", Msg: rerr.Error()})
		}
		if len(probs) > 0 {
			fmt.Fprintf(out, "%d problem(s)\n", len(probs))
			return 1
		}
		fmt.Fprintln(out, "✓ config valid")
		return 0
	case "schema":
		b, err := config.JSONSchema()
		if err != nil {
			fmt.Fprintf(out, "error: %v\n", err)
			return 1
		}
		fmt.Fprintln(out, string(b))
		return 0
	default:
		fmt.Fprintf(out, "unknown config subcommand: %s\n", sub)
		return 2
	}
}

// cmdPlatform lists configured platforms: enabled/disabled, whether a token is
// present (name/presence only, never the value), and experimental status.
func cmdPlatform(args []string, out io.Writer) int {
	if len(args) == 0 || args[0] != "list" {
		fmt.Fprintln(out, "usage: sloppy platform list [--config sloppy.yaml]")
		return 2
	}
	fs := flag.NewFlagSet("platform list", flag.ContinueOnError)
	fs.SetOutput(out)
	cfgPath := fs.String("config", "sloppy.yaml", "path to sloppy.yaml")
	if err := fs.Parse(args[1:]); err != nil {
		return 2
	}
	f, existed, err := config.LoadFile(*cfgPath)
	if err != nil {
		fmt.Fprintf(out, "error: %v\n", err)
		return 1
	}
	eff := config.Resolve(f, existed, config.FlagOverrides{}, os.Getenv)
	names := make([]string, 0, len(eff.Platforms))
	for n := range eff.Platforms {
		names = append(names, n)
	}
	sort.Strings(names)
	if len(names) == 0 {
		fmt.Fprintln(out, "no platforms configured (Log actuator only)")
		return 0
	}
	for _, n := range names {
		p := eff.Platforms[n]
		status := "disabled"
		if p.Enabled {
			status = "enabled"
		}
		tok := "no-token"
		if p.TokenEnv != "" && (os.Getenv(p.TokenEnv) != "" || os.Getenv(p.TokenEnv+"_FILE") != "") {
			tok = "token-present"
		}
		exp := ""
		if p.Experimental {
			exp = " (experimental)"
		}
		fmt.Fprintf(out, "%-9s %-9s %s%s\n", n, status, tok, exp)
	}
	return 0
}

// cmdRecipe lists the curated recipes (with enabled status) and renders a recipe to
// the exact rule it produces (+ its content-hash SHA), all read-only.
func cmdRecipe(args []string, out io.Writer) int {
	if len(args) == 0 {
		fmt.Fprintln(out, "usage: sloppy recipe <list|show> [name] [--config sloppy.yaml]")
		return 2
	}
	sub := args[0]
	rest := args[1:]
	name := ""
	if sub == "show" && len(rest) > 0 && !strings.HasPrefix(rest[0], "-") {
		name = rest[0]
		rest = rest[1:]
	}
	fs := flag.NewFlagSet("recipe "+sub, flag.ContinueOnError)
	fs.SetOutput(out)
	cfgPath := fs.String("config", "sloppy.yaml", "path to sloppy.yaml")
	if err := fs.Parse(rest); err != nil {
		return 2
	}
	f, existed, err := config.LoadFile(*cfgPath)
	if err != nil {
		fmt.Fprintf(out, "error: %v\n", err)
		return 1
	}
	eff := config.Resolve(f, existed, config.FlagOverrides{}, os.Getenv)
	switch sub {
	case "list":
		for _, n := range recipe.Names() {
			status := "available"
			if rc, ok := eff.Recipes[n]; ok {
				status = "disabled"
				if rc.Enabled {
					status = "enabled"
				}
			}
			fmt.Fprintf(out, "%-15s %-10s %s\n", n, status, recipe.Summary(n))
		}
		return 0
	case "show":
		if name == "" {
			fmt.Fprintln(out, "usage: sloppy recipe show <name> [--config sloppy.yaml]")
			return 2
		}
		if !recipe.Known(name) {
			fmt.Fprintf(out, "unknown recipe %q (have: %s)\n", name, strings.Join(recipe.Names(), ", "))
			return 1
		}
		text, sha, err := bootstrap.RenderRecipeText(eff, name)
		if err != nil {
			fmt.Fprintf(out, "error: %v\n", err)
			return 1
		}
		fmt.Fprint(out, text)
		fmt.Fprintf(out, "# rendered rule sha: %s\n", sha)
		return 0
	default:
		fmt.Fprintf(out, "unknown recipe subcommand: %s\n", sub)
		return 2
	}
}

func main() { os.Exit(run(os.Args[1:], os.Stdout)) }
