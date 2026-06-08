# Sloppy Joe v0 — Plan 4: Hardening + conformance + persisted key + self-metrics

> Implemented autonomously (TDD). Final plan of the v0 series. Full suite green, both binaries build.

**Goal:** Production-readiness hardening for the v0 loop: a deliberate fail-open/closed policy, a reusable Actuator conformance contract, signatures stable across restarts, and operator-visible self-metrics.

## What shipped

| Area | Files | Notes |
|---|---|---|
| fail-open/closed | `engine/engine.go` `WithFailMode` (+`plan4_test.go`) | On a state-store error: `FailOpen` (default) proceeds; `FailClosed` refuses to act and records `intent.fail_closed`. Tested with an injected failing store. |
| Self-metrics | `metrics/metrics.go` (+test), `engine` `WithMetrics`, `ingest` `/status` | Counters: `signals_handled`, `intents_applied/failed/skipped/dry_run/reverted`. Daemon serves them at `GET /status`. (OTLP export is a later adapter — not claimed.) |
| Persisted signing key | `intent.LoadOrCreateSigner` (+`persist_test.go`) | ed25519 key loaded from / created at a file (mode 0600); signatures verify across restarts. `sloppyd --key`. |
| Actuator conformance | `actuator/conformance.go` (+test) | Exported `Conformance(tb, a, sample)` — any actuator author runs it to verify the Apply/Revert/Receipt contract. Run against `Fake` + `Log`. |
| Daemon wiring | `cmd/sloppyd/main.go` | New flags `--key`, `--fail-closed`; metrics registry wired to engine + `/status`. |

## Verification
`go mod tidy`/`gofmt`/`go vet` clean · `go test ./...` green (16 packages: +`metrics`) · both binaries build · live daemon smoke: `POST /v1/signals` → `/status` returns `{"intents_applied":1,"signals_handled":1}`, persisted key created at mode 600.

## v0 series complete
Plans 1–4 deliver: closed signal→CEL-rule→signed-reversible-intent→actuator→hash-chained-audit loop · cost ledger + `state.*` · `for:`-window + durable TTL auto-revert · HTTP ingest + `sloppyd` daemon · replay CI gate · doctor · fail-open/closed · conformance suite · persisted keys · self-metrics. **Next horizon (post-v0):** real OTLP receiver, Redis/Postgres state for multi-replica, Sigstore signing, Bifrost/Envoy actuators, `ee/` enterprise (SSO/RBAC/KMS), and the Phase-0 design-partner validation.
