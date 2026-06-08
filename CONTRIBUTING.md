# Contributing to Sloppy Joe

Thanks for helping clean up the slop. 🥪

## Dev setup

Requires Go (see the version in `go.mod`). No CGO — everything builds static.

```bash
make build        # bin/sloppy + bin/sloppyd (CGO_ENABLED=0)
make test         # go test ./...
make ci           # the full CI gate locally: fmt-check + vet + lint + vulncheck + test-race
```

## The CI gate

A PR must pass (these are what `make ci` and `.github/workflows/ci.yml` run):

- `gofmt -l .` is empty (`make fmt` to fix)
- `go vet ./...`
- `golangci-lint run ./...` (config in `.golangci.yml`)
- `govulncheck ./...`
- `go test -race ./...` (all packages)
- `go build ./...` on linux/macOS/windows

## How we work

- **TDD.** Write the failing test first; keep packages green.
- **YAGNI / DRY.** Prefer the smallest thing that works; don't add abstraction or
  surface until something needs it. Shared logic lives in one place (e.g. `state.ChainHash`,
  `actuator.postJSON`).
- **New actuators** must pass `actuator.Conformance(t, …)`.
- **New `state.Store` backends** must pass the shared `storeContract` test (see `state/contract_test.go`).
- Keep provider keys out of Sloppy Joe — only scoped admin/notify tokens, via the secret broker.

## Design docs

Specs and plans live under `docs/superpowers/` (`specs/`, `plans/`, and `plans/BACKLOG.md`).
Significant changes should start from a short spec there.

## Commits

Conventional-commit style (`feat:`, `fix:`, `refactor:`, `docs:`, `ci:`). Small, focused commits.
