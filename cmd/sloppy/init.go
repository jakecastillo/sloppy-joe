package main

import (
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"

	"github.com/sloppyjoe/sloppy/intent"
)

// scaffoldConfig is a working starter sloppy.yaml: it validates as-is and gives a
// live loop via the cost-guard recipe, with all credentialed platforms disabled.
const scaffoldConfig = `# yaml-language-server: $schema=https://raw.githubusercontent.com/sloppyjoe/sloppy/main/docs/sloppy.schema.json
# Sloppy Joe configuration. Hand-edited and Git-reviewed; the CLI never rewrites
# this file (this scaffold is the one exception). Check it with:
#   sloppy config validate    sloppy platform list    sloppy recipe list
version: 1

store:
  kind: sqlite          # sqlite | redis
  path: sloppy.db

engine:
  signing_key: sloppy.key
  log_format: text      # text | json

# Hand-written rules (optional once recipes below cover you).
rules:
  - ./rules

# Platforms to act on. Secrets are NEVER inline: reference an env var resolved by
# the SLOPPY_TOKEN_* broker (see .env.sample). Disabled until you add credentials.
platforms:
  litellm: { enabled: false, url: http://localhost:4000, token_env: SLOPPY_TOKEN_LITELLM }
  github:  { enabled: false, repo: owner/name,           token_env: SLOPPY_TOKEN_GITHUB }
  slack:   { enabled: false, channel: "#ai-ops",         token_env: SLOPPY_TOKEN_SLACK }

# Curated workflows. 'sloppy recipe list' to browse; 'sloppy recipe show <name>'
# to see the exact rule one renders to. Configure each inline.
recipes:
  cost-guard:
    enabled: true
    threshold_usd_1h: 5.0
    failover: { alias: gpt-4o, to: ollama/llama3, ttl: 30m }
`

// scaffoldEnv lists the secret env vars the platforms reference, redacted.
const scaffoldEnv = `# Sloppy Joe secrets. Fill in and load via your secret manager; never commit values.
# The broker also accepts <NAME>_FILE pointing at a mounted file (e.g. /run/secrets).
# SLOPPY_TOKEN_LITELLM=
# SLOPPY_TOKEN_GITHUB=
# SLOPPY_TOKEN_SLACK=
# SLOPPY_API_KEYS=key1=ingest:write,status:read
`

// scaffoldRulesSample is a commented starter dropped in ./rules/. It is written as
// a .yaml.sample (NOT .yaml/.yml) so the rule loader skips it: the scaffold relies
// on the cost-guard recipe out of the box, and an empty rules dir is fine. Rename a
// copy to *.yaml and uncomment to author your first hand-written rule.
const scaffoldRulesSample = `# Sloppy Joe hand-written rule (sample). Copy to a *.yaml file in this directory
# and uncomment to activate. One YAML document = one rule. Check with:
#   sloppy rules validate ./rules
#
# on: cost.budget_burn                       # signal type to match
# when: signal.data.spend_1h_usd > 5.0       # CEL-style predicate
# then:
#   - page: { slack: "#ai-ops" }             # action(s) to take when it fires
`

// scaffoldPricebookSample is a starter price book dropped beside the config as a
// .yaml.sample. The cost-guard recipe (enabled in the scaffold) only fires once
// spend is non-zero, and spend stays $0 until the ledger has prices — so a fresh
// install with no price book makes cost-guard look broken. Copy this to a *.yaml
// file and wire it with `sloppyd --pricebook <file>` (or `engine.pricebook`). The
// prices are ILLUSTRATIVE ONLY and will drift; replace with your provider's rates.
const scaffoldPricebookSample = `# Sloppy Joe price book (sample) — ILLUSTRATIVE ONLY, NOT AUTHORITATIVE.
# Copy to a *.yaml file and wire it with: sloppyd --pricebook <file>
# (or set engine.pricebook in sloppy.yaml). Maps a model to its per-1k-token price
# in USD; the cost ledger uses it to turn OTLP token metrics into estimated spend,
# which the cost-guard recipe guards against. A model absent here is priced at $0.
# These numbers are rough and WILL drift — replace with your provider's current rates.
#
# Schema: <model-name>: { input_per_1k: <usd>, output_per_1k: <usd> }
gpt-4o:
  input_per_1k: 0.0025
  output_per_1k: 0.01
gpt-4o-mini:
  input_per_1k: 0.00015
  output_per_1k: 0.0006
claude-3-5-sonnet:
  input_per_1k: 0.003
  output_per_1k: 0.015
ollama/llama3:
  input_per_1k: 0.0
  output_per_1k: 0.0
`

// cmdInit scaffolds a starter config, a redacted .env.sample, and a signing key.
// It is non-interactive (safe in CI), refuses to clobber an existing config without
// --force, and a no-op re-run exits 0 ("already initialized").
func cmdInit(args []string, out io.Writer) int {
	fs := flag.NewFlagSet("init", flag.ContinueOnError)
	fs.SetOutput(out)
	cfgPath := fs.String("config", "sloppy.yaml", "config file to scaffold")
	force := fs.Bool("force", false, "overwrite an existing config")
	fs.Bool("yes", false, "non-interactive (reserved; init is already non-interactive)")
	if err := fs.Parse(args); err != nil {
		return 2
	}

	if _, err := os.Stat(*cfgPath); err == nil && !*force {
		fmt.Fprintf(out, "%s already exists — already initialized, nothing to do (use --force to overwrite)\n", *cfgPath)
		return 0
	}

	dir := filepath.Dir(*cfgPath)
	if err := os.WriteFile(*cfgPath, []byte(scaffoldConfig), 0o644); err != nil {
		fmt.Fprintf(out, "error: %v\n", err)
		return 1
	}
	fmt.Fprintf(out, "✓ wrote %s\n", *cfgPath)

	envPath := filepath.Join(dir, ".env.sample")
	if err := os.WriteFile(envPath, []byte(scaffoldEnv), 0o644); err != nil {
		fmt.Fprintf(out, "error: %v\n", err)
		return 1
	}
	fmt.Fprintf(out, "✓ wrote %s\n", envPath)

	// A starter price book beside the config so the cost-guard recipe has non-zero
	// spend to guard against (an empty book signals $0 and cost-guard never fires).
	// Written .yaml.sample (the ledger never auto-loads it) and never clobbered, so a
	// re-run or --force leaves an operator's edited copy untouched.
	pbPath := filepath.Join(dir, "pricebook.yaml.sample")
	if _, err := os.Stat(pbPath); err != nil {
		if err := os.WriteFile(pbPath, []byte(scaffoldPricebookSample), 0o644); err != nil {
			fmt.Fprintf(out, "error: %v\n", err)
			return 1
		}
		fmt.Fprintf(out, "✓ wrote %s\n", pbPath)
	}

	keyPath := filepath.Join(dir, "sloppy.key")
	if _, err := intent.LoadOrCreateSigner(keyPath); err != nil {
		fmt.Fprintf(out, "error: %v\n", err)
		return 1
	}
	fmt.Fprintf(out, "✓ created signing key %s\n", keyPath)

	// Create ./rules/ so the scaffold's `rules: [./rules]` resolves and the very
	// first `sloppy doctor` does not fail on a missing path. The directory starts
	// empty (recipes cover the live loop); a commented *.yaml.sample shows how to
	// author your first rule (the loader ignores non-.yaml/.yml files).
	rulesDir := filepath.Join(dir, "rules")
	if err := os.MkdirAll(rulesDir, 0o750); err != nil {
		fmt.Fprintf(out, "error: %v\n", err)
		return 1
	}
	fmt.Fprintf(out, "✓ created rules dir %s\n", rulesDir)
	samplePath := filepath.Join(rulesDir, "example.yaml.sample")
	if err := os.WriteFile(samplePath, []byte(scaffoldRulesSample), 0o644); err != nil {
		fmt.Fprintf(out, "error: %v\n", err)
		return 1
	}
	fmt.Fprintf(out, "✓ wrote %s\n", samplePath)

	fmt.Fprintln(out, "next: run `sloppy config validate`, then `sloppy doctor`, then start the daemon with `sloppyd`")
	return 0
}
