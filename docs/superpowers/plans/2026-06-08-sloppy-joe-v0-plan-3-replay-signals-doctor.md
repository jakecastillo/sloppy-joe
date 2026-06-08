# Sloppy Joe v0 — Plan 3: Replay CLI + signal breadth + doctor

> Implemented autonomously (TDD). Builds on Plans 1–2. Full suite green, binaries build.

**Goal:** Make rules safe to evolve (deterministic replay as a CI gate), broaden the demonstrated signal taxonomy, and give operators a one-shot health/connectivity check.

## What shipped

| Area | Files | Notes |
|---|---|---|
| Deterministic replay | `replay/replay.go` (+test) | Pure `EvaluateMatches` over recorded signals — no actuation, no state writes. |
| Replay CLI | `cmd/sloppy` `test --replay` + `config.LoadSignalsJSONL` | `sloppy test --replay <fixture.jsonl> [--rules dir]` prints what *would* fire + a summary; the CI gate for rule changes. |
| Signal breadth | `examples/rules/latency-guard.yaml`, `examples/rules/fallback-storm.yaml`, `examples/fixtures/replay.jsonl` | New signal types (`latency.regression`, `fallback.fired`) need no engine code — they match on `signal.data.*` / `signal.severity`. Demonstrates the type-agnostic design. |
| Doctor | `doctor/doctor.go` (+test), `cmd/sloppy` `doctor` | `CheckRules` / `CheckDB` / `CheckLiteLLM` (probes `SLOPPY_LITELLM_URL` if set). `sloppy doctor` prints a checklist; non-zero exit if any check fails. |

## Verification
`gofmt`/`go vet` clean · `go test ./...` green (14 packages: +`replay`, +`doctor`) · `bin/sloppy` builds · real replay over `examples/fixtures/replay.jsonl`: 4 signals → 6 intents would fire (cost 3, latency 2, fallback 1; quiet signal → no rule) · real `doctor`: rules ✓ / state-db ✓ / litellm skipped ✓.

## Notes / deferred
Replay evaluates with empty `state.*` (ledger-independent) for determinism — rules that depend on `state.spend_*` should encode the value in `signal.data` for replay fixtures, or assert via the live ledger path. Plan 4: fail-open/closed knob, Actuator conformance suite, persisted signing key, self-metrics/status endpoint.
