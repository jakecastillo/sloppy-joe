# Conventional Commits

Sloppy Joe uses [Conventional Commits](https://www.conventionalcommits.org/). A
machine-readable history powers the CHANGELOG and (via goreleaser) release notes,
and keeps the log scannable.

## Format

```
<type>(<scope>)?<!>?: <description>

[optional body]

[optional footer(s)]
```

- **type** — one of: `feat`, `fix`, `docs`, `style`, `refactor`, `perf`, `test`,
  `build`, `ci`, `chore`, `revert`.
- **scope** *(optional)* — the area touched, lowercase: `engine`, `actuator`,
  `state`, `rules`, `ledger`, `ingest`, `cli`, `ci`, … e.g. `feat(engine):`.
- **`!`** *(optional)* — marks a breaking change: `feat(state)!: …` (also note it
  in a `BREAKING CHANGE:` footer).
- **description** — imperative mood, lower-case, no trailing period:
  "add", not "added" / "adds".

## Examples

```
feat(engine): enforce intent_budget per rule window
fix(state): keep the audit chain intact under concurrent appends
docs: complete the OSS licensing posture
refactor(actuator): split per-gateway request bodies
ci: pin golangci-lint to a fixed version
feat(state)!: thread context.Context through every Store method
```

## What maps to a release bump

| Type / marker        | Semantic version bump |
|----------------------|-----------------------|
| `fix:`               | patch                 |
| `feat:`              | minor                 |
| `!` / `BREAKING CHANGE:` | major             |
| others (`docs`, `chore`, …) | no release      |

## Enforcement

- **Locally:** the `commit-msg` git hook rejects non-conforming subjects. Install
  it once with `make hooks` (sets `core.hooksPath=.githooks`).
- **On PRs:** the `Commit Lint` GitHub Action validates every commit in the PR.
- Merge commits, reverts, and `fixup!`/`squash!` commits are exempt.

Also remember to **sign off** each commit for the DCO: `git commit -s`
(see [CONTRIBUTING](../CONTRIBUTING.md)).
