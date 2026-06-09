# Streamlined Config, Init & Platform/Recipe Management — Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL — use `superpowers:executing-plans` or
> `superpowers:subagent-driven-development` to implement task-by-task. Steps use
> checkbox (`- [ ]`) syntax. This plan is executed **inline** in the authoring
> session (Go verified locally: `gofmt`/`go vet`/`go build`/`go test ./...` green
> per phase before each commit).

**Goal:** Replace Sloppy Joe's scattered flags + `SLOPPY_*` env wiring with a
single declarative `sloppy.yaml` (source of truth), a one-time `sloppy init`
scaffold, read-only config introspection, declarative platform on/off that finally
wires the dead actuators, and a small set of typed, configurable recipes.

**Architecture:** A new config layer (`File` schema → `Effective` resolver with
`flags > env > file > defaults` precedence + provenance → `Validate` with secret
linting) feeds a shared bootstrap builder that constructs the actuator registry and
engine for both binaries. Recipes are `//go:embed`-ed YAML templates rendered
through the existing `rules.ParseRules` path, so each gets a real content-hash SHA
and flows through the existing engine/replay/audit. Edit model is **file-only /
read-only CLI**; `sloppy init` is the lone writer.

**Tech stack:** Go 1.26, stdlib `flag`, `gopkg.in/yaml.v3`, `github.com/mattn/go-isatty`
(already an indirect dep), `cel-go` (existing). No new direct deps beyond go-isatty.

**Spec:** `docs/superpowers/specs/2026-06-08-config-init-platform-management-design.md`

---

## Phase ordering & rationale

A → B → C → D. Hard constraints: the `config.Effective` layer (A) underpins
everything; the Slack→`TokenFunc` rewire must precede the bootstrap builder (B);
recipes (C) depend on the config schema (A) and the enabled-platform contract (B);
init + fail-mode + README polish (D) sit on top. Each phase ends green and is
committed independently.

---

## Phase A — Config foundation

**Delivers:** `File` schema + `LoadFile` (zero-config → defaults), `Effective`
resolver (precedence + provenance), `Validate` (structural + enum + version +
**secret-lint**), and read-only `sloppy config show` / `config validate`. The
binaries do **not** consume `Effective` yet (that's B) — no runtime behavior change.

**Files**
- Create `config/file.go` — `File` + nested config structs; `LoadFile(path) (File, bool, error)` (strict `KnownFields`); `Defaults()`; `applyDefaults`.
- Create `config/effective.go` — `Effective` (embeds resolved `File` + provenance); `Resolve(f, existed, FlagOverrides, getenv)`; `Source(key)`; duration accessors.
- Create `config/validate.go` — `Problem`; `Validate(File) []Problem`; secret-lint helpers (`looksSecret`, known platform/token_env checks).
- Create `config/render.go` — `RenderEffective(io.Writer, Effective, showProvenance)` (YAML + redaction; never resolves tokens).
- Modify `cmd/sloppy/main.go` — add `config` command → `show` / `validate` subcommands; `--config` flag; update usage line.
- Tests: `config/file_test.go`, `config/effective_test.go`, `config/validate_test.go`, `config/render_test.go`, `cmd/sloppy/config_test.go`.

**Tasks (TDD; each: write failing test → run red → implement → run green → commit at phase end)**
- [ ] `File` struct + `LoadFile` + `Defaults`/`applyDefaults`. Tests: parse a full sample; missing file → defaults, `existed=false`; unknown key rejected (strict); duration fields stay strings.
- [ ] `Effective` + `Resolve` + provenance. Tests: file value surfaces (`source=file`); flag override beats file (`source=flag`); `SLOPPY_LITELLM_URL` env → litellm platform url (`source=env`); `revert_interval` parses via accessor; defaults when zero-config.
- [ ] `Validate` + secret-lint. Tests: clean file → no problems; bad `store.kind`; redis w/o addr; bad `log_format`; bad `fail_mode`; unknown `version`; **inline secret in `slack.channel`/`platform.url` rejected**; `token_env` holding an inline secret rejected; `token_env` not matching `SLOPPY_TOKEN_*` flagged.
- [ ] `RenderEffective` redaction. Tests: output shows `token_env` **name**; setting `SLOPPY_TOKEN_LITELLM=supersecret` then rendering never prints `supersecret`; `--provenance` annotates env/flag overrides loudly.
- [ ] `config show` + `config validate` commands. Tests: `validate` exit 0 on good temp file, exit 1 on bad; `show` prints store kind + platform stanzas; `show` never prints a resolved token.

**Acceptance:** `gofmt` (new files) clean · `go vet` · `go build ./...` · `go test ./...` green. Commit: `feat(config): declarative sloppy.yaml + effective resolver + read-only show/validate`.

---

## Phase B — Secrets hardening + platform wiring (bootstrap builder)

**Delivers:** the Slack credential fix, broker `*_FILE` support, the shared
bootstrap builder that constructs the registry/engine from `Effective` (**wiring
Bifrost/Envoy/GitHub/Slack** — confirmed unreachable today), retirement of the
env-side-effect wiring, `sloppy platform list`, extended `doctor`, and the
`docker-compose.yml` inline-token fix.

**Files**
- Modify `actuator/slack.go` — `NewSlack(channel string, token TokenFunc)`; resolve webhook via broker; update `actuator/slack_test.go` and any constructors/tests referencing `NewSlack`.
- Modify `secrets/broker.go` — `EnvBroker.Get` honors `SLOPPY_TOKEN_<CAP>_FILE` (read+trim) before the raw env var; `secrets/broker_test.go`.
- Create `config/bootstrap.go` — `BuildRegistry(eff, broker, out) (*actuator.Registry, error)` and `BuildEngine(...)`; registers `Log` + enabled actuators via broker `TokenFunc`s; legacy fallback (no `platforms:` + `SLOPPY_LITELLM_URL`) preserves zero-config; `config/bootstrap_test.go`.
- Modify `cmd/sloppy/main.go` + `cmd/sloppyd/main.go` — consume `--config` through the builder; retire `os.Getenv("SLOPPY_LITELLM_URL")` side-effect wiring; add `platform list`.
- Modify `doctor/doctor.go` (+ test) — checks driven by effective config: enabled platforms, token presence (no values), recipe rules valid (stub until C).
- Modify `docker-compose.yml` — replace inline `SLOPPY_TOKEN_LITELLM: sk-sloppy-dev` with `${VAR}`/secrets indirection; scaffold note.

**Acceptance:** green; **transition test** (`no file + SLOPPY_LITELLM_URL` still wires LiteLLM identically); enabled-set → `registry.Kinds()`; Slack has no inline-secret path. Commit: `feat(config): bootstrap builder wires platforms from config + secret-safe actuators`.

---

## Phase C — Recipes

**Delivers:** the `recipe` package (registry, typed params, `//go:embed` YAML
templates, platform-aware `Render`, SHA over rendered bytes), `recipe list` /
`recipe show`, and appending rendered recipe rules to the loaded rule set. Initial
recipes mirror existing examples: `cost-guard`, `fallback-storm`, `latency-guard`.

**Files**
- Create `recipe/recipe.go` — `Registry`, `Recipe` (name + typed params decode/validate + embedded template), `Render(name, rawParams, enabled) ([]rules.Rule, error)`; SHA = `sha256(rendered bytes)` via existing `rules.ParseRules`.
- Create `recipe/templates/{cost-guard,fallback-storm,latency-guard}.yaml.tmpl` + `recipe/embed.go` (`//go:embed templates/*.tmpl`).
- Create `recipe/recipe_test.go` — each recipe renders to expected rule incl. platform-aware notify (enabled vs disabled set); SHA deterministic across two renders; SHA changes when template version line changes; rendered rule passes `ParseRules` + `rules.Validate`.
- Modify rule-loading path (`config.BuildEngine` / a `config.LoadRulesAndRecipes`) to append recipe-rendered rules; narrow `enabled` contract (`interface{ Enabled(string) bool }` or `[]string`).
- Modify `cmd/sloppy/main.go` — `recipe list` / `recipe show <name>` (prints rendered YAML + SHA); extend `config validate` to render+validate enabled recipes.

**Acceptance:** green; recipe SHA reproducibility test; `recipe show` prints rendered rule + SHA. Commit: `feat(recipe): embedded-YAML recipes render to canonical rules with real SHA`.

---

## Phase D — init + fail-mode + onboarding

**Delivers:** `sloppy init` (TTY-aware, no-clobber idempotent, scaffolds
`sloppy.yaml` + `rules/` + key + redacted `.env.sample` + schema header),
per-capability fail-mode (**fail-closed for mutating** route_override/
throttle_tenant/disable_deployment; fail-open for notify + Log), JSON Schema emitter
+ load-bearing `version`, README de-`up` + drift-guard test, CHANGELOG.

**Files**
- Create `cmd/sloppy/init.go` (+ test) — `sloppy init [--yes] [--force] [--config]`; `isatty` via `go-isatty`; embedded scaffold templates; no-clobber → exit 0; non-TTY w/o `--yes` and missing value → fail-fast with next command.
- Create `config/schema.go` (+ test) — JSON Schema generated from `File` (or hand-maintained + in-sync reflection test); `sloppy config schema` command.
- Modify `engine/engine.go` (+ test) — per-capability fail-mode map from config; default closed for mutating, open for notify/Log; loud audit + metric on any fail-open actuation.
- Modify `README.md` — remove `sloppy up`; document `init` + `sloppyd`; create `cmd/sloppy/readme_test.go` asserting every README-named `sloppy <cmd>` resolves.
- Modify `CHANGELOG.md` — note the fail-mode default change + new config surface under `[Unreleased]`.

**Acceptance:** green; init golden + no-clobber + non-TTY tests; fail-mode tests; README-commands test. Commit(s): `feat(cli): sloppy init scaffold + JSON schema`, `feat(engine): fail-closed default for mutating actuators`, `docs: README entrypoint + changelog`.

---

## Cross-cutting acceptance (every phase)

`gofmt -l` clean on touched files · `go vet ./...` · `CGO_ENABLED=0 go build ./...`
· `go test ./...` green · both binaries build · Conventional Commit, DCO `-s`, **no
AI co-author trailer**. Zero-config behavior preserved until each consumer migrates.
