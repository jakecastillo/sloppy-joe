# Sloppy Joe — Post-v0 Plan Backlog (ralph loop)

Each plan is implemented with TDD and is **satisfied completely** only when:
`gofmt` clean · `go vet ./...` clean · `go test ./...` green · both binaries build · committed.

The ralph loop takes the next unchecked plan, finishes it to that bar, checks it off, and advances. When all are checked, the loop stops.

- [x] **Plan 5 — OTLP metrics ingest → ledger.** `POST /v1/otlp/metrics` accepting OTLP/JSON metrics that carry gen_ai token usage; map data points (token type / tenant / model / value) into `ledger.Record`. Test by posting a crafted OTLP/JSON doc and asserting ledger spend. *Satisfied: parser + endpoint tested; ledger reflects usage.*
- [ ] **Plan 6 — Redis state backend.** `state.OpenRedis(addr)` implementing the same `state.Store` contract (applied-intent idempotency, hash-chained audit, pending reverts) on Redis. Shared contract test runs against both SQLite and Redis (via `miniredis`). *Satisfied: both backends pass one shared Store contract test.*
- [ ] **Plan 7 — Bifrost + Envoy actuators.** `actuator/bifrost.go` + `actuator/envoy.go` implementing `route_override` against their admin APIs (httptest-mocked), each run through `actuator.Conformance`. *Satisfied: both adapters tested + conformant.*
- [ ] **Plan 8 — enterprise auth shim (`ee/`).** API-key RBAC middleware (capability scopes, e.g. `ingest:write`, `status:read`) wrapping the ingest handler; keys→scopes from env. Optional `sloppyd --auth`. *Satisfied: allow/deny middleware tested; wired behind a flag.*

Post-backlog horizon (not in this loop): real OTLP traces, Sigstore keyless signing, Postgres backend, full multi-tenant, Phase-0 design-partner validation.
