# Sloppy Joe — Streamlined Config, Init & Platform/Recipe Management (Design)

- **Date:** 2026-06-08
- **Status:** Approved design, pending spec review → implementation plan
- **Topic:** A single declarative `sloppy.yaml` as the source of truth for both binaries, a one-time `sloppy init` scaffold, read-only config introspection, declarative platform on/off (wiring the actuators that exist but are unreachable today), and a small set of typed, configurable **recipes**.

> **Provenance.** This design was brainstormed, then put through a multi-agent
> "current software-development standards" review (7 research angles × 6
> adversarial critics + synthesis). Verdict: **proceed with changes, high
> confidence.** The core skeleton was independently validated against Terraform,
> Renovate, Grafana provisioning, Caddy, kubectl, helm, and docker-compose; the
> two P0 changes below were verified directly against this repo's code. The
> recipe mechanism was **reversed** from the original "compiled Go templates"
> proposal to embedded-YAML-data as a result of that review.

---

## 1. Context & problem

Sloppy Joe is a self-hostable Go **AI-ops control loop** (observe → decide → act
→ record → revert) that sits beside an LLM gateway, consumes telemetry, and
applies signed, reversible, Git-reviewed **intents** through capability-scoped
actuators. Today its configuration is **scattered and implicit**:

- ~11 `sloppyd` flags + CLI flags, plus `SLOPPY_*` env vars.
- Actuators wire by **env-var side effect**: `buildEngine`/`buildRegistry`
  (duplicated across `cmd/sloppy/main.go` and `cmd/sloppyd/main.go`) register
  `Log` always and LiteLLM **only if** `SLOPPY_LITELLM_URL` happens to be set.
- The `Bifrost`, `Envoy`, `GitHub`, and `Slack` actuators **exist and are tested**
  (`actuator/{bifrost,envoy,github,slack}.go`) but are **unreachable from either
  binary** — there is no supported way to turn them on.
- `README.md:32` advertises `sloppy up`, which **does not exist** in the CLI.

The goal: make initializing and configuring Sloppy Joe streamlined and easy to
manage — "CLI management of the AI systems you've wired up" — without violating
the project's load-bearing invariants (policy-as-Git-artifact, library-first,
YAGNI, off-hot-path, **never holds your provider keys**).

## 2. Goals / non-goals

**Goals**
1. One declarative, Git-reviewable `sloppy.yaml` = single source of truth for the
   `sloppy` CLI and the `sloppyd` daemon.
2. **Zero-config still works** — no file ⇒ today's defaults reproduced exactly.
3. **File-only / read-only CLI** edit model (humans hand-edit; the CLI never
   mutates the file). The single exception is `sloppy init`, a one-time
   generator.
4. Read-only introspection that delivers the ergonomics: `config show`
   (effective + provenance), `config validate`, `config schema`, `platform list`,
   `recipe list`/`recipe show`, extended `doctor`.
5. Declarative platform **enable/disable**, and a shared bootstrap builder that
   **finally wires** Bifrost/Envoy/GitHub/Slack from config.
6. A small, curated set of **recipes** (typed, configurable workflows) that render
   to the **same `rules.Rule` artifact** as hand-written rules — same engine,
   replay, audit, and SHA-provenance path.

**Non-goals / blessed YAGNI cuts (explicitly do NOT build)**
- No CLI mutation of the file (no `config set`, no `platform enable` that writes).
- No XDG/user-level config layer (the file travels with the repo).
- No multi-environment overlay/profile engine — *document* the
  per-environment-committed-file pattern instead.
- No `cobra`/`viper` (hand-rolled `flag` + `yaml.v3` stays; more idiomatic for
  this surface and dependency posture).
- No SOPS/sealed-secrets/ESO (references-only sidesteps encrypt-in-Git).
- No plugin marketplace / external recipe ecosystem.
- No schema-migration tooling beyond a load-bearing `version` check.

## 3. The config file (`sloppy.yaml`)

Project-local (`./sloppy.yaml`, override with `--config`), like `docker-compose.yml`.

```yaml
version: 1                                  # load-bearing; validate fails closed on unknown

server:                                     # sloppyd-only
  addr: ":8723"
  revert_interval: 30s

store:
  kind: sqlite                              # sqlite | redis
  path: sloppy.db                           # sqlite
  redis_addr: ""                            # redis (host:port)

engine:
  signing_key: sloppy.key
  log_format: text                          # text | json
  pricebook: ""                             # optional path
  fail_mode:                                # per-capability; see §9
    default: closed                         # mutating actions fail-closed by default
    notify: open                            # open_issue/page fail-open

auth:
  enabled: false
  keys_env: SLOPPY_API_KEYS                 # reference only; never inline keys

rules:
  - ./rules                                 # hand-written rules (dirs/files), unchanged

platforms:                                  # turn AI systems on/off here
  litellm:
    enabled: true
    url: http://localhost:4000              # admin endpoint (non-secret)
    token_env: SLOPPY_TOKEN_LITELLM         # admin token via broker
  bifrost: { enabled: false, experimental: true, url: "", token_env: SLOPPY_TOKEN_BIFROST }
  envoy:   { enabled: false, experimental: true, url: "", token_env: SLOPPY_TOKEN_ENVOY }
  github:
    enabled: false
    repo: owner/name                        # non-secret identifier
    base_url: https://api.github.com
    token_env: SLOPPY_TOKEN_GITHUB
  slack:
    enabled: false
    channel: "#ai-ops"                      # display name (non-secret)
    token_env: SLOPPY_TOKEN_SLACK           # the WEBHOOK URL is the secret — broker only

recipes:                                    # few, each typed + configurable
  cost-guard:
    enabled: true
    on: cost.budget_burn
    threshold_usd_1h: 5.0
    for: 5m
    failover: { alias: gpt-4o, to: ollama/llama3, ttl: 30m }
    intent_budget: 3/h
    notify: { open_issue: true, page: true }  # auto-skipped if that platform is disabled
```

**Sensitivity rule (see §5):** non-secret identifiers (URLs, repo slug, channel
name) may be inline; **anything bearer-equivalent (tokens, Slack webhook URLs,
keys, PEMs) is *always* a `token_env` reference**, never inline.

## 4. Precedence & the `Effective` resolver

**Precedence: flags > env > file > built-in defaults** (verbatim clig.dev; matches
docker/kubectl/helm). `config.Effective` is the **single merge point**. The
existing env-var side-effect wiring (`os.Getenv("SLOPPY_LITELLM_URL")` in both
`main.go`s, `ee.LoadFromEnv`) is **retired** and folded into the resolver when the
bootstrap builder lands, so there are not two competing precedence paths.

`Effective` tracks **provenance** per field (default/file/env/flag). `config show
--provenance` renders any non-file value **loudly** (e.g.
`addr=:9000 (from --addr, overrides file)`) so flag/env overrides cannot silently
reintroduce the out-of-band drift the file-only model exists to prevent.

## 5. Secrets model

- **Classify by sensitivity, not field type.** Bearer-equivalent values
  (tokens, **Slack webhook URLs**, keys, PEMs) resolve only through `token_env` →
  the `SLOPPY_TOKEN_*` broker (`secrets.Broker`, default-deny capability
  allowlist). Only true non-secrets may be inline.
- **[P0] Rewire the Slack actuator.** `actuator/slack.go` currently is
  `NewSlack(webhook string)` and stores the full webhook (a bearer credential) as
  a plain string — unlike `actuator/github.go`'s `NewGitHub(baseURL, TokenFunc)`.
  Change `NewSlack` to take a `TokenFunc` (resolve the webhook via
  `SLOPPY_TOKEN_SLACK`). **This must land before the bootstrap builder**, so every
  credentialed actuator constructs through one uniform token contract
  (safe-by-construction, not correct-by-accident).
- **`config show` never leaks.** It renders `token_env` as the env-var **name**
  only and never resolves it — the broker is **not invoked by introspection at
  all**. Sensitive inline fields are masked. A test asserts no resolved token byte
  appears in `config show` output.
- **`config validate` is the enforced guardrail** (the only automated one, since
  the file is hand-edited): (a) rejects inline values matching
  token/webhook/PEM/secret-URL heuristics (e.g. `hooks.slack.com/services/...`),
  (b) verifies every `token_env` names a broker-allowed `SLOPPY_TOKEN_*`
  capability. **Fails closed in CI.**
- **[P1] Broker `*_FILE` support.** `secrets.NewEnvBroker` honors
  `SLOPPY_TOKEN_<CAP>_FILE` (read path, trim) before the raw env var — the
  canonical container/k8s secret-file path (`/run/secrets`).
- **[P1] Fix `docker-compose.yml`.** It currently inlines
  `SLOPPY_TOKEN_LITELLM: sk-sloppy-dev` (the one container artifact does the
  opposite of our posture). Switch to `${VAR}`/env-file or a compose `secrets:`
  block + `_FILE`. `sloppy init` scaffolds a redacted `.env.sample` listing
  required `SLOPPY_TOKEN_*` placeholders. `doctor` reports token presence/absence
  **without printing values**.

## 6. Platform model & the bootstrap builder

Platforms are enabled declaratively via `platforms.<name>.enabled`
(config-as-data — the convergent standard: Renovate, k8s feature gates, Terraform,
Caddy). A new shared builder (`config.BuildRegistry` / a small `bootstrap`
package) constructs the `actuator.Registry` from the effective platform config +
broker:

```
BuildRegistry(eff Effective, broker secrets.Broker, out io.Writer) (*actuator.Registry, error)
```

It registers `Log` plus exactly the enabled actuators
(`litellm`/`bifrost`/`envoy`/`github`/`slack`), each receiving a `TokenFunc` from
the broker. This is the **single highest-value change**: it **fixes the confirmed
bug** that Bifrost/Envoy/GitHub/Slack are unreachable, and **DRYs** the duplicated
`buildRegistry`/`buildEngine` across both binaries. Both `main.go`s switch to it.

## 7. Recipe model (reversed from the original proposal)

> **Why reversed.** The original design compiled recipes into typed Go templates.
> The standards review (unanimous across the maintainability, ethos, ops,
> contrarian, and preset-research lenses) and the code overturned it: the three
> initial recipes already exist **byte-for-byte** as `examples/rules/*.yaml` and
> already render through `ParseRules`, where each gets a real content-hash SHA.
> `Rule.SHA` (`rules/parse.go:49`) is **load-bearing** — it is the for-window key
> (`engine/engine.go:123`), throttle/budget key (`:163`, `:170`), dedup key
> (`:176`), and signed `RuleSHA` provenance. Rules rendered at runtime from
> compiled Go have **no reviewable on-disk bytes**, so their SHAs would collide or
> drift between binary builds — breaking the replay-in-CI guarantee the product
> sells. Compiling recipes in is also a Principle-of-Least-Power / "config as
> code" regression (the pattern ESLint publicly reversed in 2025) and a second
> code-extension surface the anti-marketplace invariant forbids.

**Mechanism.** A recipe = a curated **`//go:embed`-ed YAML template** over the
existing rule schema + a small **typed param struct** (for defaults/validation).
At load:

```
params (from sloppy.yaml) ──decode+validate──▶ render embedded template
   with params + enabled-platform set ──▶ rendered YAML bytes
   ──ParseRules──▶ rules.Rule{ SHA = sha256(rendered bytes) }  ──▶ appended to hand-written rules
```

- **Configurable knobs in Git** (`threshold_usd_1h`, `failover`, `ttl`,
  `intent_budget`, …) — the "easily configurable" steer, satisfied by editing
  data, not recompiling.
- **Platform-aware:** the template emits `open_issue`/`page` actions only for
  platforms that are enabled (passed as a **narrow contract** — an
  `interface{ Enabled(name string) bool }` or `[]string` of enabled names, **not**
  the platforms config struct, keeping dependency direction one-way).
- **Provenance is real and reviewable.** `sloppy recipe show <name>` prints the
  fully rendered YAML **and** its SHA. Rendering is deterministic (no time/random),
  so the SHA is reproducible across binaries given the same params. Each embedded
  template carries a `# recipe: <name> vN` header line, so a template change
  deliberately changes the rendered bytes (and the SHA) — visible and auditable. A
  replay test asserts SHA reproducibility.
- **Curated & closed:** the recipe set ships embedded; adding one is a curated
  data change, not a plugin surface. Initial set: `cost-guard`, `fallback-storm`,
  `latency-guard` (mirroring the existing example rules).

`recipe.Registry`: `List()`, `Get(name)`, and
`Render(name, rawParams, enabled) ([]rules.Rule, error)`.

## 8. Command surface (read-only except `init`)

| Command | Role |
|---|---|
| `sloppy init [--yes] [--force]` | One-time generator: writes `sloppy.yaml` + `rules/` + signing key + redacted `.env.sample`. **No-clobber**; re-run without `--force` prints "already initialized, nothing to do" and exits **0** (idempotent). `--force` regenerates. TTY-aware (§10). |
| `sloppy config show [--provenance]` | Render **effective** merged config; `token_env` shown as name only, sensitive fields masked; provenance annotates non-file values loudly. |
| `sloppy config validate` | Parse + schema + enum + `version` + **secret-lint** + CEL-compile of rendered recipe rules. Non-zero on error — a CI gate, like `rules validate`. |
| `sloppy config schema` | Emit the JSON Schema generated from the `File` struct (§11). |
| `sloppy platform list` | Table: platform · enabled · experimental · token/url present · capabilities. |
| `sloppy recipe list` / `recipe show <name>` | List recipes + knobs; `show` prints the **rendered rule(s) + SHA**. |
| `sloppy doctor` | Extended to read config: which platforms enabled + reachable, recipe rules valid, token presence (no values). |
| existing `inject`/`test`/`audit`/`rules`, `sloppyd` | Gain `--config`; defaults (rules path, db, store, fail-mode…) come from the file; flags still override. |

**`sloppy up`:** removed from `README.md`. The documented entrypoint becomes
`sloppy init` (scaffold) then `sloppyd` (run). A CLI test asserts **every command
named in the README resolves** in the CLI switch, so docs cannot drift again.

## 9. Fail-mode model

Today the engine defaults to **fail-open** (`engine.New` → `FailOpen`): on a
state-store error it actuates anyway, skipping the dedup/budget/circuit/audit
state it could not read. For privileged **mutating** actions that means the
riskiest moment bypasses every guardrail.

**Change:** fail-mode is **per-capability**, defaulting:
- **fail-closed** for gateway-state-changing actions: `route_override`,
  `throttle_tenant`, `disable_deployment`.
- **fail-open** for side-effect-free notify (`open_issue`, `page`) and `Log`.

Overridable via `engine.fail_mode` in the config (`default` + `notify` knobs, and
room for per-kind overrides). A fail-open actuation emits the same **loud audit
entry + metric** the fail-closed path already does. This is a **behavior change**
for existing deployments relying on availability-over-strictness — gated loudly in
release notes / CHANGELOG.

## 10. `init` UX details

- **isatty(stdin):** prompt interactively only when stdin is a TTY. In non-TTY/CI
  contexts, proceed with safe defaults (or, if a required value is missing, **fail
  fast** printing the exact next command — never hang on a prompt). `--yes` forces
  non-interactive defaults even on a TTY.
- **No-clobber idempotency:** never overwrite an existing file; re-run is a no-op
  exit 0 unless `--force`.
- **Scaffolds:** `sloppy.yaml` (with the `# yaml-language-server: $schema=` header),
  a starter `rules/` set, the signing key (via `intent.LoadOrCreateSigner`), and a
  redacted `.env.sample`.

## 11. JSON Schema & versioning

- **Generate a JSON Schema from the `File` struct** (reuse, not new surface).
  `sloppy config schema` emits it; `init` writes the
  `# yaml-language-server: $schema=<url>` header into the scaffold so hand-editors
  get autocomplete + a field reference (the highest-leverage ergonomic the
  file-only model otherwise lacks). The schema is generated from / tested in sync
  with the struct so it cannot rot.
- **`version` is load-bearing:** `config validate` fails closed on an
  unknown/unsupported `version` with an actionable message. Compatibility policy
  documented: additive fields = minor; removal/rename = version bump.

## 12. Code structure & boundaries

- **`config` (extend):** `file.go` (`File` struct + `LoadFile`), `effective.go`
  (`Effective` + `Resolve` single merge point + provenance), `validate.go`
  (`Validate` incl. secret-lint), `schema.go` (JSON Schema emitter),
  `BuildRegistry`/`BuildEngine` (or a small `bootstrap` package).
- **`recipe` (new):** registry, typed params, embedded templates, `Render`.
- **`actuator/slack.go`:** `NewSlack(channel string, token TokenFunc)`.
- **`engine`:** per-capability fail-mode map + config wiring.
- **`secrets/broker.go`:** `*_FILE` resolution.
- **`cmd/sloppy/main.go` + `cmd/sloppyd/main.go`:** new/extended commands; both
  consume `--config` through the shared builder; flags override.
- **`docker-compose.yml`, `README.md`, `CHANGELOG.md`:** secret indirection,
  remove `up`, note the fail-mode default change.

Dependency direction stays one-way: `config`/`bootstrap` → `recipe`,
`actuator`, `engine`. `recipe` never imports the platforms config struct.

## 13. Backward compatibility & migration

**No breaking changes.** No `sloppy.yaml` ⇒ defaults reproduce today exactly
(sqlite, `rules/`, `Log` + LiteLLM-if-`SLOPPY_LITELLM_URL`). Existing flags/env
keep working as overrides. A transition test asserts `no file + SLOPPY_LITELLM_URL
set` still wires LiteLLM identically, and that both binaries consume a consistent
effective config. The only intentional behavior change is the fail-mode default
(§9), gated in release notes.

## 14. Testing strategy (TDD; project bar: gofmt/vet/golangci-lint/`go test ./...` green, both binaries build)

Table tests for:
- **config:** parse; defaults merge; precedence + provenance; `validate`
  good/bad/missing-token/unknown-version/**inline-secret-rejected**.
- **secret-leak invariant:** `config show` output contains **no resolved token
  bytes**; broker not invoked by `show`.
- **recipe:** each recipe → expected rendered rule incl. platform-aware notify
  (enabled vs disabled set); **SHA deterministic across two renders**; SHA changes
  when the template version line changes; rendered rule passes `ParseRules` +
  `Validate`.
- **bootstrap:** enabled platform set → `registry.Kinds()` includes expected
  actuators; disabled → absent; Slack constructed via `TokenFunc` (no inline
  secret path exists).
- **engine fail-mode:** store error + mutating kind → refuse (fail-closed) + loud
  audit/metric; notify kind → proceed (fail-open) + audit.
- **init:** golden scaffold; no-clobber re-run → exit 0; `--force` regenerates;
  non-TTY without `--yes` and a missing required value → fail-fast with next
  command; `.env.sample` is redacted.
- **backward-compat:** the transition test above.
- **README commands:** every command token named in `README.md` resolves.
- **broker:** `SLOPPY_TOKEN_<CAP>_FILE` read+trim takes precedence over raw env.

## 15. Open risks

- **Recipe SHA contract** — mitigated by deterministic rendering + the embedded
  `# recipe: <name> vN` version line + a replay reproducibility test. Do **not**
  introduce any non-deterministic input into rendering.
- **Single-merge-point migration** — retiring env side-effects must not regress
  zero-config or the existing compose invocation; guarded by the transition test.
- **`config show` redaction** is a hard correctness requirement, enforced by test.
- **Slack rewrite ordering** — `NewSlack`→`TokenFunc` is a prerequisite of the
  bootstrap builder; sequence it first; `config validate` secret-lint is the
  backstop.
- **Fail-mode default change** could surprise availability-first deployments — gate
  in release notes; keep observe/notify fail-open.
- **Scope creep into enterprise ceremony** — resist the blessed YAGNI cuts (§2).
- **JSON Schema sync** — generate from / test against the struct so it can't rot.

## 16. Out of scope (future, demand-gated)

XDG/user-level layering; a real multi-env overlay/`extends` engine (only if a
second environment shares >80% of one file); Vault/other broker backends beyond
`*_FILE`; promoting a *specific* recipe to a codegen/template path **only** if it
ever needs genuine programmatic fan-out beyond static YAML + CEL + platform
filtering; constant-time API-key compare + routing `SLOPPY_API_KEYS` through the
broker (P2 hygiene, opportunistic); `/readyz` + Dockerfile `HEALTHCHECK` (P2).
