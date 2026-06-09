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

	keyPath := filepath.Join(dir, "sloppy.key")
	if _, err := intent.LoadOrCreateSigner(keyPath); err != nil {
		fmt.Fprintf(out, "error: %v\n", err)
		return 1
	}
	fmt.Fprintf(out, "✓ created signing key %s\n", keyPath)

	fmt.Fprintln(out, "next: edit sloppy.yaml, then run `sloppy config validate` and `sloppyd`")
	return 0
}
