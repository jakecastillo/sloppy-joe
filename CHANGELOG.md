# Changelog

All notable changes to this project are documented here.
Format follows [Keep a Changelog](https://keepachangelog.com/); this project is pre-1.0.

## [Unreleased]

## [0.1.0] - 2026-06-08

### Added
- **v0 control loop** (Plans 1–4): Signal (OTel-GenAI/CloudEvents-shaped) → YAML+CEL
  rule → signed, reversible RemediationIntent → actuator → hash-chained audit. Off the
  inference hot path; provider keys stay in the gateway.
- Cost ledger (priced from token usage) feeding `state.*` CEL fields; `for:`-window
  gating; durable TTL auto-revert.
- HTTP ingest (`/v1/signals`, `/v1/usage`, `/v1/otlp/metrics`, `/status`, `/healthz`)
  and the `sloppyd` daemon.
- Actuators: LiteLLM, Bifrost, Envoy (shared `httpRouteActuator`), GitHub, Slack —
  all verified via `actuator.Conformance`.
- State backends: SQLite (default) and Redis (`--store redis`), validated by a shared
  store contract test.
- `sloppy test --replay` (deterministic CI gate), `sloppy doctor`, self-metrics,
  persisted ed25519 signing key, fail-open/closed knob, and `ee/` API-key RBAC (`--auth`).

### Fixed (quality audit)
- SQLite writes no longer drop under daemon concurrency (busy_timeout + WAL +
  single writer; transactional audit append).
- Redis audit append is now atomic (WATCH/MULTI/EXEC), preventing chain forks.
- Engine no longer swallows state-write errors (idempotency/revert/audit failures are
  audited + counted); failed reverts stay pending and retry.
- Dropped the `context.Context` struct field in the Redis store; swept the unbounded
  `for:`-window pending map.

### CI/CD
- GitHub Actions: lint (gofmt/vet/golangci-lint), test (`-race` + coverage), build
  matrix (linux/macOS/windows), `govulncheck`, CodeQL; Dependabot; goreleaser release.

### Added (post-v0 hardening + governance, Plans 9–20)
- Flagship demo fixed: `sloppy inject --now` bypasses `for:` windows for one-shot CLI runs.
- `sloppy rules validate` — zero-infra CI gate (CEL compile + action/budget checks).
- Governance enforced: `intent_budget` throttling and `rollback: on_clear`.
- `context.Context` threaded through `state.Store`; Redis idempotency keys bounded
  (per-id TTL); rule-action logs pruned.
- Cost ledger persisted behind `state.Store` (survives restarts; pruned + TTL'd).
- Real per-gateway actuator bodies (LiteLLM `/model/update`); Bifrost/Envoy marked experimental.
- Structured logging (`log/slog`; `sloppyd --log-format json`).
- New reversible actions: `throttle_tenant`, `disable_deployment`.
- Registry graceful-degrade (known-but-unsupported kind → notify) + crash-boundary test.
- Local end-to-end `docker-compose` stack (Ollama + LiteLLM + Redis + sloppyd) +
  `//go:build integration` e2e test.
- Phase-0 demand-validation kit (`docs/phase0/`).

### Security / supply chain
- Apache-2.0 license with DCO sign-off; NOTICE + AUTHORS.
- Pre-commit hook (gofmt/vet/build/test) + commit-msg hook (Conventional Commits), via `make hooks`.
- CI: pinned tool versions (golangci-lint, govulncheck), a 72% coverage floor, and a
  Conventional-Commits PR check.
- Releases: SBOM (syft) + cosign keyless signatures + SLSA build provenance.

[0.1.0]: https://github.com/sloppyjoe/sloppy/releases/tag/v0.1.0
