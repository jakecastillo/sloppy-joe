# Dev tooling: Go-native formatters + DX niceties — Design

**Date:** 2026-06-08
**Status:** Approved (pending written-spec review)
**Scope:** One implementation plan. Adds file formatters + editor/DX hygiene across all
file types, enforced through the project's existing three-layer gate. No new language
toolchain (no Node/Python).

---

## 1. Context & goal

`sloppy-joe` is a Go project (79 `.go`, 20 `.md`, 13 YAML, 2 JSON, 2 shell hooks, 1
Dockerfile). It already has strong, deliberately **Go-native** quality tooling:

- `gofmt` (formatter), `go vet`, `golangci-lint` v2 (`@v2.12.2`), `govulncheck`,
  `go test -race` + a 72% coverage floor.
- A three-layer enforcement model (defense-in-depth):
  `.githooks/pre-commit` (fast, every commit) → `make ci` (full gate) →
  GitHub Actions (full gate + build matrix + CodeQL).
- Tools invoked via **pinned** `go run tool@vX.Y.Z` (no `@latest` drift).

**Goal:** extend formatting/hygiene to *all* file types (not just Go) and add
low-cost developer-experience niceties, **without** violating the project's stated
ethos: single static Go binary, no CGO, no extra runtime toolchain, editor-agnostic
(the repo gitignores both `.vscode/` and `.idea/`).

## 2. Constraints / principles (inherited from the repo)

1. **Go-native only.** Every new tool must run via pinned `go run tool@vX.Y.Z`, the
   same pattern as `golangci-lint`/`govulncheck`. No `package.json`, no `node_modules`,
   no Python.
2. **Editor-agnostic.** No committed editor config (`.vscode/`, `.idea/` stay ignored);
   `.editorconfig` is the universal, editor-neutral equivalent.
3. **Defense-in-depth, mirrored.** Each enforcement layer is self-contained (the repo
   already triplicates the `gofmt` check across hook/Makefile/CI); new checks follow
   the same shape.
4. **Don't mangle docs.** Markdown is *not* auto-reflowed (the docs contain Mermaid
   diagrams + tables); Markdown/JSON get whitespace hygiene via `.editorconfig` only.
5. **YAGNI.** Add what serves the goal; record deferred ideas rather than building them.

## 3. Decisions (resolved in brainstorming)

| # | Decision | Choice |
|---|----------|--------|
| 1 | Non-Go file formatting | **Go-native only** (best practice for this repo — see §2) |
| 2 | Go formatter strictness | **gofumpt + gci** import ordering (drop-in, config-only) |
| 3 | Enforcement | **Block in CI + pre-commit** (mirror the existing gate) |
| 4 | shfmt (shell) | **Include** — covers the two `.githooks` scripts |
| 5 | One-time reformat | Single `style:` commit + `.git-blame-ignore-revs` |
| 6 | npm distribution | **Documented future step** (GoReleaser `npms:`), not built now |

## 4. The toolchain (per file type)

| Files | Tool | Pinned invocation | New dep? |
|---|---|---|---|
| **Go** (79) | gofumpt + gci | `golangci-lint@v2.12.2` (bundled formatters) | No — config only |
| **YAML** (13) | yamlfmt | `go run github.com/google/yamlfmt/cmd/yamlfmt@v0.21.0` | pinned go-run |
| **Shell** (2 hooks) | shfmt | `go run mvdan.cc/sh/v3/cmd/shfmt@v3.13.1` | pinned go-run |
| **Workflows** (4) | actionlint | `go run github.com/rhysd/actionlint/cmd/actionlint@v1.7.12` | pinned go-run (lint only) |
| **All files** | `.editorconfig` | editor-native | none |
| **Line endings** | `.gitattributes` | git-native | none |

> Versions verified against live GitHub releases on 2026-06-08; re-verify exact tags at
> implementation. gofumpt/gci are **compiled into** the golangci-lint binary in v2 — no
> separate install.

### Key enforcement nuance (verified)

`golangci-lint fmt` *writes* formatting; `golangci-lint fmt --diff` only *prints*
diffs and **does not exit non-zero** (open upstream issue #5601). The reliable Go
format **gate** is the existing **`golangci-lint run`** — in v2 it executes the
configured formatters and reports violations as issues, exiting non-zero. So once the
formatters are enabled in `.golangci.yml`, the repo's current `make lint` / CI
`golangci-lint run ./...` step **already enforces gofumpt+gci** with no new Go wiring.
For the *fast* layers (pre-commit, `make fmt-check`) we use the non-mutating
`test -n "$(golangci-lint fmt --diff)"` pattern (fail when the diff is non-empty),
reusing the already-pinned binary.

## 5. New files

### `.editorconfig`
```ini
# https://editorconfig.org — editor-agnostic baseline (repo gitignores .vscode/ & .idea/).
root = true

[*]
charset = utf-8
end_of_line = lf
insert_final_newline = true
trim_trailing_whitespace = true
indent_style = space
indent_size = 2

[*.go]
indent_style = tab
indent_size = 4

[Makefile]
indent_style = tab

[.githooks/*]
indent_style = tab

[*.md]
# Preserve Markdown hard line breaks (two trailing spaces).
trim_trailing_whitespace = false
```

### `.gitattributes`
```gitattributes
# Normalize line endings: LF in the repo AND the working tree. This is a Unix-first
# project whose /bin/sh git hooks must stay LF even on Windows/OneDrive checkouts.
* text=auto eol=lf

# Defensive explicit LF for shell + git hooks.
*.sh        text eol=lf
.githooks/* text eol=lf

# Lockfile: keep as text, suppress diff noise.
go.sum -diff

# Future-proof binary assets.
*.png binary
*.gz  binary
*.zip binary
```

### `.yamlfmt`
```yaml
formatter:
  type: basic
  indent: 2
  retain_line_breaks: true
  include_document_start: false
  line_ending: lf
  trim_trailing_whitespace: true
  eof_newline: true
  pad_line_comments: 2
  max_line_length: 0   # 0 = no wrapping
```
> yamlfmt matches only `.yaml`/`.yml` by default — it will **not** touch the 2 JSON
> files (correct; JSON hygiene comes from `.editorconfig`).

### `.git-blame-ignore-revs`
```
# Revs to skip in `git blame` (bulk reformat, no semantic change).
# Local: git config blame.ignoreRevsFile .git-blame-ignore-revs   (GitHub honors it automatically)

# style: gofumpt + gci + yamlfmt + shfmt (whole tree)
<SHA of the style: reformat commit — filled in after that commit lands>
```

## 6. Modified files

### `.golangci.yml`
Swap the single-line `formatters: { enable: [gofmt] }` for gofumpt + gci. The
`linters:` block is unchanged.
```yaml
formatters:
  enable:
    - gofumpt
    - gci
  settings:
    gofumpt:
      module-path: github.com/sloppyjoe/sloppy
      extra-rules: false
    gci:
      sections:
        - standard
        - default
        - prefix(github.com/sloppyjoe/sloppy)
      custom-order: true   # REQUIRED for the section order above to be honored
```

### `Makefile`
Introduce pinned tool-version variables (kills the `@latest` drift in the current
`lint` target) and broaden `fmt` / `fmt-check` to all file types; add `lint-actions`;
wire both into `ci`.
```makefile
# Pinned tool versions (mirror CI; no @latest drift).
GOLANGCI_VERSION   ?= v2.12.2
YAMLFMT_VERSION    ?= v0.21.0
SHFMT_VERSION      ?= v3.13.1
ACTIONLINT_VERSION ?= v1.7.12

GOLANGCI   := go run github.com/golangci/golangci-lint/v2/cmd/golangci-lint@$(GOLANGCI_VERSION)
YAMLFMT    := go run github.com/google/yamlfmt/cmd/yamlfmt@$(YAMLFMT_VERSION)
SHFMT      := go run mvdan.cc/sh/v3/cmd/shfmt@$(SHFMT_VERSION) -ln posix -i 0 -s
ACTIONLINT := go run github.com/rhysd/actionlint/cmd/actionlint@$(ACTIONLINT_VERSION)

ci: fmt-check vet lint lint-actions vulncheck test-race

fmt:
	$(GOLANGCI) fmt
	$(YAMLFMT) .
	$(SHFMT) -w .githooks/

fmt-check:
	@d=$$($(GOLANGCI) fmt --diff); if [ -n "$$d" ]; then echo "$$d"; echo "go: run 'make fmt'"; exit 1; fi
	$(YAMLFMT) -lint .
	$(SHFMT) -d .githooks/

lint:
	$(GOLANGCI) run ./...

lint-actions:
	$(ACTIONLINT) -color -shellcheck= -pyflakes=
```
(`.PHONY` gains `lint-actions`.)

### `.githooks/pre-commit`
Replace the `gofmt -l .` block with the all-types **format check** (self-contained,
no `make` dependency — matching the existing inline style). gci import-order is left to
the full `golangci-lint run` gate (`make ci`/CI), consistent with defense-in-depth.
actionlint is *not* in the fast hook (workflows change rarely; it runs in `make ci`/CI).
```sh
echo "pre-commit: format check (go/yaml/shell)"
d=$(go run github.com/golangci/golangci-lint/v2/cmd/golangci-lint@v2.12.2 fmt --diff)
if [ -n "$d" ]; then echo "✗ Go not gofumpt/gci-clean (run: make fmt):"; echo "$d"; exit 1; fi
go run github.com/google/yamlfmt/cmd/yamlfmt@v0.21.0 -lint .
go run mvdan.cc/sh/v3/cmd/shfmt@v3.13.1 -ln posix -i 0 -s -d .githooks/
```
(vet / build / test steps unchanged.)

### `.github/workflows/ci.yml`
In the `lint` job: **remove** the standalone `gofmt` step (now redundant — the existing
`golangci-lint run` step enforces gofumpt+gci authoritatively) and add:
```yaml
      - name: format check (yaml + shell)
        run: |
          go run github.com/google/yamlfmt/cmd/yamlfmt@v0.21.0 -lint .
          go run mvdan.cc/sh/v3/cmd/shfmt@v3.13.1 -ln posix -i 0 -s -d .githooks/
      - name: actionlint
        run: go run github.com/rhysd/actionlint/cmd/actionlint@v1.7.12 -color -shellcheck= -pyflakes=
```

### `CONTRIBUTING.md`
Update **Dev setup** + **The CI gate** to note `make fmt` now covers Go + YAML + shell,
list the new pinned tools, and mention `.editorconfig` / `.gitattributes` /
`.git-blame-ignore-revs` (incl. the one-line `git config blame.ignoreRevsFile` tip).

## 7. One-time reformat sequencing

The new gates fail on pre-existing unformatted files, so normalization must land first:

1. **Config commit** — add all new files + modified configs (incl. `.git-blame-ignore-revs`
   with a header but no SHA yet). *Don't* add the CI/hook checks as blocking yet, or land
   them together with step 2 so CI sees the formatted tree.
2. **`style:` reformat commit** — run `make fmt` (gofumpt+gci via `golangci-lint fmt`,
   `yamlfmt .`, `shfmt -w .githooks/`) across the whole tree. Commit as
   `style: gofumpt + gci + yamlfmt + shfmt (whole tree)`.
   - **Safety net:** `go test ./...` exercises example-rule YAML parsing
     (`examples/rules/*.yaml`) + the deploy config, so any semantic YAML breakage from
     reformatting is caught. Review the diff against `examples/` and `deploy/` before
     committing.
3. **Blame-ignore commit** — append the step-2 SHA to `.git-blame-ignore-revs`.

## 8. npm distribution — forward-compat note (NOT built)

Distribution channel and dev formatters are orthogonal: npm ships the *compiled binary*;
formatters only ever touch *source*. Going Go-native does **not** block npm. If/when npm
is wanted it's a release-time GoReleaser change with **no Node in the dev or CI loop**.
Recorded in `docs/superpowers/plans/BACKLOG.md`:

> **npm distribution via GoReleaser `npms:`** — maps the existing `sloppy`/`sloppyd`
> builds into an `@sloppyjoe/sloppy` package (`npx`/`npm i -g`).
> **Prerequisites:** `npms` is **GoReleaser Pro-only** (since v2.8) → needs a Pro license
> key + `NPM_TOKEN` CI secret. Current `.goreleaser.yaml` is OSS-only.
> **Gotcha:** gate prereleases with `disable: "{{ ne .Prerelease \"\" }}"` —
> `disable: "{{ .Prerelease }}"` never disables (only the literal `true` disables).
> **Supply chain:** the generated package uses a `postinstall` that downloads+extracts the
> release archive (breaks under `--ignore-scripts`); pair with the repo's existing
> checksums + cosign signing.

## 9. Out of scope (YAGNI) — with rationale

- ❌ **Prettier / markdownlint (Node)** — second toolchain for ~22 files; Markdown
  auto-reflow would mangle Mermaid/tables. `.editorconfig` covers hygiene.
- ❌ **Committed `.vscode/` settings** — repo is editor-agnostic by design;
  `.editorconfig` is the neutral equivalent.
- ❌ **hadolint / shellcheck / typos** — non-Go toolchains for marginal gain
  (`misspell` already covers Go via golangci-lint; `actionlint` runs hermetically
  without shellcheck). Easy to add later.
- ❌ **CODEOWNERS** — org/process choice, unrelated to formatting.

## 10. Implementation order (for the plan)

1. Add `.editorconfig`, `.gitattributes`, `.yamlfmt`, `.git-blame-ignore-revs` (header).
2. Update `.golangci.yml` (gofumpt+gci).
3. Update `Makefile` (version vars, `fmt`, `fmt-check`, `lint`, `lint-actions`, `ci`).
4. `make fmt` → `style:` whole-tree reformat commit (review `examples/`/`deploy/` diff;
   `go test ./...` green).
5. Append the reformat SHA to `.git-blame-ignore-revs`.
6. Update `.githooks/pre-commit` + `.github/workflows/ci.yml` (new checks) + `CONTRIBUTING.md`.
7. Add the npm BACKLOG entry.
8. Verify: `make ci` green locally; push; CI green.

## Appendix — verification sources (2026-06-08)

- golangci-lint formatters: golangci-lint.run/docs/formatters/configuration ; issue #5601
  (no non-zero exit on `fmt --diff`); discussion #5945 (`run` enforces formatters).
- yamlfmt v0.21.0: github.com/google/yamlfmt/releases ; `-lint` exits 1 on diff.
- shfmt v3.13.1: github.com/mvdan/sh ; `-i 0` = tabs, `-ln posix` = `/bin/sh`, `-d` = diff+non-zero.
- actionlint v1.7.12: github.com/rhysd/actionlint ; `-shellcheck= -pyflakes=` = hermetic.
- GoReleaser `npms`: goreleaser.com/customization/npm (Pro-only; `disable` template caveat).
