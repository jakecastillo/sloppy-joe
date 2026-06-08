# Changelog

All notable changes to this project are documented here.
Format follows [Keep a Changelog](https://keepachangelog.com/); this project is pre-1.0.

## [Unreleased]

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
