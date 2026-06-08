# Sloppy Joe v0 — Plan 2: Ingest + Cost Ledger + Daemon + for-window + TTL revert

> Implemented autonomously (TDD). Builds on Plan 1. Each item shipped with tests; full suite green, both binaries build.

**Goal:** Move from a one-shot CLI loop to a continuously-running control loop: derive a priced cost ledger, expose `state.*` to rules, accept signals/usage over HTTP, run as a daemon, enforce `for:` windows, and durably auto-revert TTL'd remediations.

**Tech added:** `net/http` ingest, `gopkg.in/yaml.v3` price book; everything else stdlib + existing deps.

## What shipped

| Area | Files | Notes |
|---|---|---|
| Cost ledger | `ledger/ledger.go` (+test) | Priced from token usage × `PriceBook` (YAML); windowed `Spend`; `StateFor` → `state.spend_1h_usd`, `state.spend_24h_usd`. Marked **estimated**. |
| `state.*` rules | `rules/reconcile.go` (`EvaluateMatches`, `Match`) (+`matches_test.go`) | Reconciler now returns matches carrying the `Rule` (so the engine can gate on `for:`); `Reconcile` kept as a flattening wrapper. CEL `state.*` driven by the ledger. |
| Durable TTL revert | `state/store.go` + `state/sqlite.go` (`pending_reverts` table) (+`revert_test.go`) | `ScheduleRevert`/`DueReverts`/`MarkReverted`; survives restart. |
| Engine upgrades | `engine/engine.go` (+`plan2_test.go`) | Options `WithLedger`/`WithClock` (back-compatible `New`); `for:`-window gating (condition must hold ≥ `for:`); TTL scheduling on apply; `ProcessDueReverts`. |
| HTTP ingest | `ingest/http.go` (+test) | `POST /v1/signals` (runs the loop), `POST /v1/usage` (feeds ledger), `/healthz`. OTLP-collector → `/v1/usage` is a thin shim (not built). |
| Daemon | `cmd/sloppyd/main.go` (+test) | `sloppyd` = ingest server + TTL-revert ticker + graceful shutdown (SIGINT/SIGTERM). Flags: `--addr --rules --db --pricebook --revert-interval`. |
| DRY | `config/load.go` | Shared `LoadRules`/`LoadSignal`; `cmd/sloppy` refactored to use it. |

## API changes (intentional evolution from Plan 1)
- `engine.New(rec, reg, store, signer, ...Option)` — variadic options; Plan 1 callers unchanged.
- `engine.Handle` adds `OutPending` (for-window) and schedules reverts.
- `rules.Reconciler.EvaluateMatches` added; `Reconcile` retained.
- `state.Store` gains `ScheduleRevert`/`DueReverts`/`MarkReverted` + `PendingRevert`.

## Verification
`go mod tidy` clean · `gofmt -l` clean · `go vet ./...` clean · `go test ./...` green (12 packages) · `CGO_ENABLED=0` builds `bin/sloppy` + `bin/sloppyd` · live daemon smoke: `healthz` ok, `POST /v1/signals` fired 2 actions (`applied`), `POST /v1/usage` → `202`.

## Deferred (Plan 3/4)
`sloppy test --replay` + signal breadth (latency/error/fallback) + `doctor` → Plan 3. fail-open/closed knob + Actuator conformance suite + persisted signing key + self-metrics → Plan 4. OTLP receiver proper (vs the `/v1/usage` shim) remains a later adapter.
