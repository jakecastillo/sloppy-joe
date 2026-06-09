# Contributing to Sloppy Joe

Thanks for helping clean up the slop. 🥪

## Dev setup

Requires Go (see the version in `go.mod`). No CGO — everything builds static.

```bash
make build        # bin/sloppy + bin/sloppyd (CGO_ENABLED=0)
make test         # go test ./...
make hooks        # install the pre-commit gate (gofmt + vet + build + test)
make ci           # the full CI gate locally: fmt-check + vet + lint + vulncheck + test-race
```

**Quality is enforced in three layers (defense in depth):** the `make hooks`
pre-commit gate (fast, every commit) → `make ci` (full gate, before pushing) →
GitHub Actions (full gate + build matrix + govulncheck + CodeQL, on every push/PR).

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

[**Conventional Commits**](docs/conventional-commits.md) — `type(scope): description`
(e.g. `feat(engine): enforce intent_budget`). Small, focused commits. Enforced by
the `commit-msg` hook (`make hooks`) and the `Commit Lint` check on PRs.
**Sign off every commit** for the DCO (`git commit -s`) — see below.

Commits are **not** attributed to AI tools: no `Co-Authored-By` / generated-by
trailers. The `commit-msg` hook and CI reject them, and Claude Code is configured
with `includeCoAuthoredBy: false` (`.claude/settings.json`).

## License & the DCO

Sloppy Joe is licensed under the [Apache License 2.0](LICENSE). Contributions are
accepted under that same license: by contributing, you agree your contribution is
licensed under Apache-2.0 (its §5 inbound = outbound grant).

We certify provenance with the [Developer Certificate of Origin](.github/DCO)
rather than a CLA. Add a `Signed-off-by` line to every commit by committing with `-s`:

```bash
git commit -s -m "feat: add a thing"
# appends: Signed-off-by: Your Name <you@example.com>
```

The name and email must be your real identity and match your Git config. On your
first contribution, add yourself to `AUTHORS`. Sign-off can be enforced by a DCO
check on pull requests.
