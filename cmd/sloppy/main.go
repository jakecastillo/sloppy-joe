// Command sloppy is the Sloppy Joe CLI.
package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"

	sloppyjoe "github.com/sloppyjoe/sloppy"
	"github.com/sloppyjoe/sloppy/actuator"
	"github.com/sloppyjoe/sloppy/core"
	"github.com/sloppyjoe/sloppy/engine"
	"github.com/sloppyjoe/sloppy/intent"
	"github.com/sloppyjoe/sloppy/rules"
	"github.com/sloppyjoe/sloppy/secrets"
	"github.com/sloppyjoe/sloppy/state"
)

func run(args []string, out io.Writer) int {
	if len(args) == 0 {
		fmt.Fprintln(out, "usage: sloppy <version|inject|audit>")
		return 2
	}
	switch args[0] {
	case "version":
		fmt.Fprintf(out, "sloppy %s\n", sloppyjoe.Version)
		return 0
	case "inject":
		return cmdInject(args[1:], out)
	case "audit":
		return cmdAudit(args[1:], out)
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
	if err := fs.Parse(args); err != nil {
		return 2
	}
	rest := fs.Args()
	if len(rest) != 1 {
		fmt.Fprintln(out, "usage: sloppy inject [--rules dir] [--db path] <signal.json>")
		return 2
	}
	sig, err := loadSignal(rest[0])
	if err != nil {
		fmt.Fprintf(out, "error: %v\n", err)
		return 1
	}
	rs, err := loadRules(*rulesPath)
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
	signer, err := intent.NewEd25519Signer()
	if err != nil {
		fmt.Fprintf(out, "error: %v\n", err)
		return 1
	}
	e := engine.New(rec, buildRegistry(out), st, signer)
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
	// Allow the friendly "sloppy audit tail" form.
	if len(args) > 0 && args[0] == "tail" {
		args = args[1:]
	}
	fs := flag.NewFlagSet("audit", flag.ContinueOnError)
	fs.SetOutput(out)
	dbPath := fs.String("db", "sloppy.db", "sqlite db path")
	if err := fs.Parse(args); err != nil {
		return 2
	}
	st, err := state.OpenSQLite(*dbPath)
	if err != nil {
		fmt.Fprintf(out, "error: %v\n", err)
		return 1
	}
	defer st.Close()
	entries, err := st.Audit()
	if err != nil {
		fmt.Fprintf(out, "error: %v\n", err)
		return 1
	}
	for _, e := range entries {
		fmt.Fprintf(out, "%4d  %-16s  %s\n", e.Seq, e.Kind, e.Detail)
	}
	status := "verified ✓"
	if !st.VerifyAudit() {
		status = "TAMPERED ✗"
	}
	fmt.Fprintf(out, "chain: %s (%d entries)\n", status, len(entries))
	return 0
}

// buildRegistry uses a logging actuator by default (demo without a live gateway);
// if SLOPPY_LITELLM_URL is set, route_override is wired to a real LiteLLM admin API
// using a token from the secret broker (SLOPPY_TOKEN_LITELLM).
func buildRegistry(out io.Writer) *actuator.Registry {
	reg := actuator.NewRegistry()
	reg.Register(&actuator.Log{W: out})
	if url := os.Getenv("SLOPPY_LITELLM_URL"); url != "" {
		br := secrets.NewEnvBroker([]string{"litellm"})
		reg.Register(actuator.NewLiteLLM(url, func() (string, error) { return br.Get("litellm") }))
	}
	return reg
}

func loadSignal(path string) (core.Signal, error) {
	b, err := os.ReadFile(path)
	if err != nil {
		return core.Signal{}, err
	}
	var s core.Signal
	if err := json.Unmarshal(b, &s); err != nil {
		return core.Signal{}, err
	}
	return s, nil
}

func loadRules(path string) ([]rules.Rule, error) {
	info, err := os.Stat(path)
	if err != nil {
		return nil, err
	}
	var files []string
	if info.IsDir() {
		entries, _ := os.ReadDir(path)
		for _, en := range entries {
			if en.IsDir() {
				continue
			}
			if ext := filepath.Ext(en.Name()); ext == ".yaml" || ext == ".yml" {
				files = append(files, filepath.Join(path, en.Name()))
			}
		}
	} else {
		files = []string{path}
	}
	var all []rules.Rule
	for _, f := range files {
		b, err := os.ReadFile(f)
		if err != nil {
			return nil, err
		}
		rs, err := rules.ParseRules(b)
		if err != nil {
			return nil, fmt.Errorf("%s: %w", f, err)
		}
		all = append(all, rs...)
	}
	if len(all) == 0 {
		return nil, fmt.Errorf("no rules found in %s", path)
	}
	return all, nil
}

func main() { os.Exit(run(os.Args[1:], os.Stdout)) }
