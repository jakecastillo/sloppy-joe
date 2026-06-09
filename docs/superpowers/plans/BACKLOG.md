# Sloppy Joe — Plan Backlog (ralph loop)

**Done:** Plans 1–8 (v0 + post-v0), quality audit (3 rounds), CI/CD, and Plans 9–14
(planning-pass NOW tier: demo fix, `rules validate`, governance enforcement,
ctx-through-Store + Redis TTL, persisted ledger, Phase-0 kit). See CHANGELOG + git.

Roadmap: `2026-06-08-roadmap.md`. This backlog promotes the roadmap's NEXT tier to NOW.

Each plan is **satisfied completely** only when: `gofmt` clean · `go vet` clean ·
`golangci-lint` 0 issues · `go test ./...` green · both binaries build · committed.
The loop takes the next unchecked plan in dependency order, finishes it to that bar,
checks it off, advances; stops when NOW is clear.

## NOW (this loop)

- [x] **Plan 15 — Real LiteLLM admin body + per-gateway actuator split.** Today
  `litellm`/`bifrost`/`envoy` share one `httpRouteActuator` body (`{model,to}`) that
  would 422 against real LiteLLM. Split so each gateway owns its request shape; give
  LiteLLM its documented `/model/update` schema (`model_name` + `litellm_params{model}`
  + `model_info`); mark Bifrost/Envoy experimental in godoc. *Satisfied: per-gateway
  httptest tests assert each body + all pass `actuator.Conformance`. (Live LiteLLM
  verification lands with the Plan 19 integration test.)*
- [x] **Plan 16 — Structured logging (`log/slog`).** Thread a `*slog.Logger` from
  `cmd/sloppyd` into engine + actuators (text for TTY, JSON via `--log-format=json`),
  carrying intent-id, rule SHA, correlation-key, target, outcome on decision/error
  paths; keep human stdout for the CLI. *Satisfied: engine `WithLogger` option
  (no-op default) + a test asserting a handled signal emits a structured record
  (slog test handler).*
- [x] **Plan 17 — `throttle_tenant` + `disable_deployment` intents.** Add the two
  `core.ActionKind`s; implement on the LiteLLM actuator (per-key rate-limit / model
  disable admin calls), reversible so TTL-revert + `rollback:on_clear` cover them;
  extend `actuator.Conformance`; add an example rule (cost runaway → `throttle_tenant`
  30m) + replay fixture; `rules validate` accepts them. *Satisfied: httptest tests for
  both actions + conformance + validate.*
- [x] **Plan 18 — Registry graceful-degrade + crash-boundary test.** `registry.Apply`
  hard-fails on an unknown kind; spec §11 promises graceful degrade. Make the registry
  fall back (unsupported kind → `open_issue`/`page`) and have `doctor` report adapter
  capabilities. Add a crash-boundary engine test: a Store that fails MarkReverted /
  MarkIntentApplied *after* the actuator call → assert no double-apply on resume,
  reverts stay pending + retry, and the `reverts_unmarked` / `state_write_failed`
  metrics fire. *Satisfied: degrade test + crash-boundary test.*
- [x] **Plan 19 — docker-compose + integration test.** `docker-compose.yml`
  (sloppyd + LiteLLM + Ollama + Redis) and a `//go:build integration` e2e test driving
  a cost spike → reroute/revert against the live stack; README deploy section.
  *Satisfied: `go build -tags integration ./...` compiles; compose validates
  structurally (`docker compose config` when Docker is present); README updated.
  Live run needs Docker — flagged, not run in the normal gate.*
- [x] **Plan 20 — Cut a trustworthy v0.1.0 (prep).** Pin CI tool versions (drop
  `@latest` for golangci-lint + govulncheck), add a coverage floor to CI, extend
  `.goreleaser.yaml` with syft SBOM + cosign keyless signing + SLSA provenance, bump
  CHANGELOG `[Unreleased]` → `[0.1.0]`. *Satisfied: pinned versions runnable; goreleaser
  config validates (`check` / `build --snapshot`); CHANGELOG updated. The tag/release
  fires in CI on tag push — human step.*

## LATER (demand-gated)
Richer signals (`eval.quality_regression`, `guardrail.tripped`) · approval gates over
`ee/` RBAC · Prometheus `/metrics` · Store consistency docs · real-Redis integration job ·
parked YAGNI: OTLP traces, Postgres backend, Sigstore (beyond cosign-in-release), xDS/CRD
Envoy driver. **Highest-leverage non-code step: run the Phase-0 validation (`docs/phase0/`).**
