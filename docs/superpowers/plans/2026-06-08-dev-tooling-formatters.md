# Dev Tooling: Go-native Formatters + DX Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Add file formatters for every file type (gofumpt+gci for Go, yamlfmt for YAML, shfmt for shell) plus editor/Git hygiene (`.editorconfig`, `.gitattributes`, `.git-blame-ignore-revs`) and workflow linting (actionlint), all enforced through the existing three-layer gate — with no Node/Python toolchain.

**Architecture:** Reuse the repo's pinned `go run tool@vX.Y.Z` pattern. Go formatting is a pure `.golangci.yml` config change (gofumpt+gci are bundled in golangci-lint v2 and already enforced by the existing `golangci-lint run` step). New `make fmt` / `make fmt-check` targets cover all file types; the pre-commit hook and CI `lint` job mirror the checks. A single blame-ignored `style:` commit normalizes the existing tree.

**Tech Stack:** Go 1.26, golangci-lint v2.12.2 (gofumpt+gci), yamlfmt v0.21.0, shfmt v3.13.1, actionlint v1.7.12, GNU make, `/bin/sh` git hooks, GitHub Actions.

**Spec:** `docs/superpowers/specs/2026-06-08-dev-tooling-formatters-design.md`

**Branch:** `chore/dev-tooling-formatters` (already created; the spec commit is `86e24b3`).

---

## File Structure

**Create:**
- `.editorconfig` — editor-agnostic whitespace/charset baseline for all file types.
- `.gitattributes` — line-ending normalization (LF in repo + working tree; protects `/bin/sh` hooks on Windows).
- `.yamlfmt` — yamlfmt config (2-space, LF, preserve line breaks).
- `.git-blame-ignore-revs` — records the bulk-reformat commit so `git blame` skips it.

**Modify:**
- `.golangci.yml` — swap the `gofmt` formatter for `gofumpt` + `gci`.
- `Makefile` — pinned tool-version vars; broaden `fmt`/`fmt-check` to all types; add `lint-actions`; pin `lint`; wire into `ci`.
- `.githooks/pre-commit` — replace the `gofmt -l` check with the all-types format check.
- `.github/workflows/ci.yml` — `lint` job: drop the now-redundant `gofmt` step, add yaml/shell format check + actionlint.
- `CONTRIBUTING.md` — document `make fmt` covering all file types + the new tools/files.
- `docs/superpowers/plans/BACKLOG.md` — add the npm-distribution future-step note.

**Sequencing rule:** the reformat (Task 4) must land *before* the blocking checks (Task 6), or the new gates fail on pre-existing files. Do **not** `git push` until Task 8 is green (intermediate local commits may be individually CI-red; CI only runs on push).

---

### Task 1: Editor + Git line-ending hygiene

**Files:**
- Create: `.editorconfig`
- Create: `.gitattributes`

- [ ] **Step 1: Create `.editorconfig`**

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

- [ ] **Step 2: Create `.gitattributes`**

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

- [ ] **Step 3: Renormalize line endings in the index**

Run: `git add --renormalize . && git status --short`
Expected: Mostly the two new files staged. Any other staged file means it had non-LF endings being normalized to LF — that is correct and desired. (On this Unix-first repo there will likely be few or none.)

- [ ] **Step 4: Confirm no content corruption**

Run: `go build ./... && go test ./...`
Expected: builds succeed; all tests PASS (renormalization changed only line endings, not content).

- [ ] **Step 5: Commit**

```bash
git add .editorconfig .gitattributes
git add --renormalize .
git commit -s -m "chore: add .editorconfig + .gitattributes (LF line-ending normalization)"
```

---

### Task 2: Formatter config files

**Files:**
- Create: `.yamlfmt`
- Create: `.git-blame-ignore-revs`

- [ ] **Step 1: Create `.yamlfmt`**

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
  max_line_length: 0
```

- [ ] **Step 2: Create `.git-blame-ignore-revs` (header only — the SHA is added in Task 5)**

```
# Revs to skip in `git blame` (bulk reformat, no semantic change).
# Local: git config blame.ignoreRevsFile .git-blame-ignore-revs   (GitHub honors it automatically)
#
# The style: whole-tree reformat commit SHA is appended in Task 5 once it exists.
```

- [ ] **Step 3: Smoke-test yamlfmt + its config (expected to report diffs — proves the gate works before we reformat)**

Run: `go run github.com/google/yamlfmt/cmd/yamlfmt@v0.21.0 -lint .`
Expected: prints diffs for existing YAML files and **exits non-zero**. This is correct — the tree is not reformatted yet; it confirms yamlfmt reads `.yamlfmt` and its `-lint` gate works.

- [ ] **Step 4: Commit**

```bash
git add .yamlfmt .git-blame-ignore-revs
git commit -s -m "chore: add .yamlfmt config + .git-blame-ignore-revs"
```

---

### Task 3: Go formatter config + Makefile targets

**Files:**
- Modify: `.golangci.yml` (replace the `formatters` block)
- Modify: `Makefile` (full replacement below)

- [ ] **Step 1: Replace `.golangci.yml` with gofumpt + gci formatters**

Full new file content:

```yaml
version: "2"

linters:
  enable:
    - errcheck
    - govet
    - ineffassign
    - staticcheck
    - unused
    - misspell
  exclusions:
    # Allow the idiomatic unchecked defer Close()/Write() etc.
    presets:
      - std-error-handling
    rules:
      # Tests may ignore errors on best-effort setup/teardown helpers.
      - path: _test\.go
        linters:
          - errcheck

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
      custom-order: true # REQUIRED for the section order above to be honored
```

- [ ] **Step 2: Verify the config parses and preview the Go reformat**

Run: `go run github.com/golangci/golangci-lint/v2/cmd/golangci-lint@v2.12.2 fmt --diff`
Expected: prints unified diffs (gofumpt tweaks + gci import regrouping) and exits 0. A clean parse with diffs confirms the config is valid. (Do **not** apply yet — that happens in Task 4.)

- [ ] **Step 3: Replace `Makefile` with the broadened targets**

Full new file content:

```makefile
.PHONY: test test-race cover build fmt fmt-check vet lint lint-actions vulncheck tidy ci hooks

# Pinned tool versions (mirror CI; no @latest drift).
GOLANGCI_VERSION   ?= v2.12.2
YAMLFMT_VERSION    ?= v0.21.0
SHFMT_VERSION      ?= v3.13.1
ACTIONLINT_VERSION ?= v1.7.12

GOLANGCI   := go run github.com/golangci/golangci-lint/v2/cmd/golangci-lint@$(GOLANGCI_VERSION)
YAMLFMT    := go run github.com/google/yamlfmt/cmd/yamlfmt@$(YAMLFMT_VERSION)
SHFMT      := go run mvdan.cc/sh/v3/cmd/shfmt@$(SHFMT_VERSION) -ln posix -i 0 -s
ACTIONLINT := go run github.com/rhysd/actionlint/cmd/actionlint@$(ACTIONLINT_VERSION)

# Mirror the CI gates so `make ci` reproduces the pipeline locally.
ci: fmt-check vet lint lint-actions vulncheck test-race

# Install the pre-commit gate (format checks + vet + build + test on every commit).
hooks:
	git config core.hooksPath .githooks
	@echo "git hooks installed (.githooks/pre-commit active)"

test:
	go test ./...

test-race:
	go test -race ./...

cover:
	go test -covermode=atomic -coverprofile=coverage.out ./...
	go tool cover -func=coverage.out | tail -1

build:
	CGO_ENABLED=0 go build -o bin/sloppy ./cmd/sloppy
	CGO_ENABLED=0 go build -o bin/sloppyd ./cmd/sloppyd

# Format every file type: Go (gofumpt+gci), YAML, shell hooks.
fmt:
	$(GOLANGCI) fmt
	$(YAMLFMT) .
	$(SHFMT) -w .githooks/

# Non-mutating check of every file type. Go uses `fmt --diff` (which does NOT
# exit non-zero on its own) wrapped to fail when the diff is non-empty.
fmt-check:
	@d=$$($(GOLANGCI) fmt --diff); if [ -n "$$d" ]; then echo "$$d"; echo "go: run 'make fmt'"; exit 1; fi
	$(YAMLFMT) -lint .
	$(SHFMT) -d .githooks/

vet:
	go vet ./...

# `run` also enforces the gofumpt+gci formatters (fails on unformatted Go).
lint:
	$(GOLANGCI) run ./...

lint-actions:
	$(ACTIONLINT) -color -shellcheck= -pyflakes=

vulncheck:
	go run golang.org/x/vuln/cmd/govulncheck@latest ./...

tidy:
	go mod tidy
```

- [ ] **Step 4: Verify the new targets resolve (dry-run, no execution)**

Run: `make -n fmt fmt-check lint-actions`
Expected: prints the `go run …` commands for each target. (Dry-run avoids failing `fmt-check` on the still-unformatted tree.)

- [ ] **Step 5: Commit**

```bash
git add .golangci.yml Makefile
git commit -s -m "build: enable gofumpt+gci formatters; broaden make fmt/fmt-check to all file types"
```

---

### Task 4: One-time whole-tree reformat (`style:` commit)

**Files:** all `.go`, `.yaml`/`.yml`, and `.githooks/*` (rewritten in place by `make fmt`).

- [ ] **Step 1: Apply formatting to the whole tree**

Run: `make fmt`
Expected: `golangci-lint fmt` rewrites Go files, `yamlfmt .` rewrites YAML, `shfmt -w .githooks/` rewrites the two hooks. Exit 0.

- [ ] **Step 2: Review the scope of the diff**

Run: `git diff --stat`
Expected: changes across `.go` files (import grouping + gofumpt), YAML files (`examples/rules/*.yaml`, `deploy/`, `.github/`, root configs), and `.githooks/pre-commit` + `.githooks/commit-msg`. Eyeball `examples/rules/*.yaml` and `deploy/litellm.config.yaml` to confirm yamlfmt only reflowed formatting, not semantics.

- [ ] **Step 3: Prove no semantic breakage (YAML rule parsing is exercised by tests)**

Run: `go test ./...`
Expected: all packages PASS (rule-loading tests parse the reformatted `examples/rules/*.yaml`).

- [ ] **Step 4: Build both binaries**

Run: `CGO_ENABLED=0 go build ./...`
Expected: success, no output.

- [ ] **Step 5: Confirm the tree is now formatter-clean**

Run: `make fmt-check && make lint`
Expected: `fmt-check` exits 0 (no diffs, yamlfmt `-lint` clean, shfmt `-d` clean); `make lint` reports 0 issues (gofumpt+gci + linters clean).

- [ ] **Step 6: Commit the reformat as a single labeled commit**

```bash
git add -A
git commit -s -m "style: gofumpt + gci + yamlfmt + shfmt (whole tree)

One-time normalization for the new formatters. No semantic change; go test
./... green. Recorded in .git-blame-ignore-revs."
```

---

### Task 5: Record the reformat commit in `.git-blame-ignore-revs`

**Files:**
- Modify: `.git-blame-ignore-revs` (append the Task 4 commit SHA)

- [ ] **Step 1: Capture the reformat commit SHA**

Run: `git rev-parse HEAD`
Expected: a 40-char SHA — this is the `style:` reformat commit from Task 4.

- [ ] **Step 2: Append the entry to `.git-blame-ignore-revs`**

Append these two lines to the file (replace `<SHA>` with the value from Step 1):

```
# style: gofumpt + gci + yamlfmt + shfmt (whole tree)
<SHA>
```

- [ ] **Step 3: Verify git blame honors it**

Run:
```bash
git config blame.ignoreRevsFile .git-blame-ignore-revs
git blame -- README.md | head -3
```
Expected: blame succeeds without error and attributes lines to their last *semantic* author rather than the `style:` commit. (A parse error here means the file has a malformed rev line — fix it.)

- [ ] **Step 4: Commit**

```bash
git add .git-blame-ignore-revs
git commit -s -m "chore: ignore the style reformat commit in git blame"
```

---

### Task 6: Enforce format checks in pre-commit + CI

**Files:**
- Modify: `.githooks/pre-commit` (full replacement below)
- Modify: `.github/workflows/ci.yml` (lint job)

- [ ] **Step 1: Replace `.githooks/pre-commit`**

Full new file content:

```sh
#!/bin/sh
# Fast local quality gate. Defense-in-depth: the full gate (golangci-lint, -race,
# govulncheck, actionlint) runs in `make ci` and in GitHub Actions; this keeps commits honest.
set -e

echo "pre-commit: format check (go/yaml/shell)"
d=$(go run github.com/golangci/golangci-lint/v2/cmd/golangci-lint@v2.12.2 fmt --diff)
if [ -n "$d" ]; then
	echo "✗ Go not gofumpt/gci-clean (run: make fmt):"
	echo "$d"
	exit 1
fi
go run github.com/google/yamlfmt/cmd/yamlfmt@v0.21.0 -lint .
go run mvdan.cc/sh/v3/cmd/shfmt@v3.13.1 -ln posix -i 0 -s -d .githooks/

echo "pre-commit: go vet"
go vet ./...

echo "pre-commit: go build"
CGO_ENABLED=0 go build ./...

echo "pre-commit: go test"
go test ./...

echo "pre-commit: ✓ ok"
```

- [ ] **Step 2: Update the `lint` job in `.github/workflows/ci.yml`**

Replace the existing `gofmt` step (the `- name: gofmt` block) and the `golangci-lint` step so the job's steps read exactly:

```yaml
      - uses: actions/checkout@v4
      - uses: actions/setup-go@v5
        with:
          go-version-file: go.mod
          cache: true
      - name: go vet
        run: go vet ./...
      - name: format check (yaml + shell)
        run: |
          go run github.com/google/yamlfmt/cmd/yamlfmt@v0.21.0 -lint .
          go run mvdan.cc/sh/v3/cmd/shfmt@v3.13.1 -ln posix -i 0 -s -d .githooks/
      # Pinned version (no @latest drift); v2 config in .golangci.yml.
      # `run` also enforces the gofumpt+gci formatters (fails on unformatted Go).
      - name: golangci-lint (lint + gofumpt/gci format gate)
        run: go run github.com/golangci/golangci-lint/v2/cmd/golangci-lint@v2.12.2 run ./...
      - name: actionlint
        run: go run github.com/rhysd/actionlint/cmd/actionlint@v1.7.12 -color -shellcheck= -pyflakes=
```

(The standalone `gofmt` step is intentionally removed — `golangci-lint run` now enforces Go formatting via the gofumpt+gci formatters.)

- [ ] **Step 3: Normalize any formatting drift from the hand-edits**

Run: `make fmt`
Expected: re-formats the just-edited `.github/workflows/ci.yml` (yamlfmt) and `.githooks/pre-commit` (shfmt) so they match the formatters. Likely a small or empty diff.

- [ ] **Step 4: Verify the new gates pass on the formatted tree**

Run: `make fmt-check && make lint-actions`
Expected: `fmt-check` exits 0; `actionlint` validates all 4 workflows (including the edited `ci.yml`) with **no issues**.

- [ ] **Step 5: Verify the hook end-to-end**

Run: `sh .githooks/pre-commit`
Expected: prints each stage and ends with `pre-commit: ✓ ok` (format check passes, vet/build/test pass).

- [ ] **Step 6: Commit**

```bash
git add .githooks/pre-commit .github/workflows/ci.yml
git commit -s -m "ci: enforce all-file-type format checks + actionlint (pre-commit + CI)"
```

---

### Task 7: Documentation — CONTRIBUTING + npm backlog note

**Files:**
- Modify: `CONTRIBUTING.md`
- Modify: `docs/superpowers/plans/BACKLOG.md`

- [ ] **Step 1: Update the `## Dev setup` block in `CONTRIBUTING.md`**

Replace the fenced `bash` block under `## Dev setup` with:

```bash
make build        # bin/sloppy + bin/sloppyd (CGO_ENABLED=0)
make test         # go test ./...
make fmt          # format all file types: Go (gofumpt+gci), YAML (yamlfmt), shell (shfmt)
make hooks        # install the pre-commit gate (format check + vet + build + test)
make ci           # the full gate locally: fmt-check + vet + lint + lint-actions + vulncheck + test-race
```

- [ ] **Step 2: Update the `## The CI gate` bullet list in `CONTRIBUTING.md`**

Replace the existing bullet list (the six `- ...` lines from the `gofmt -l .` bullet through `go build ./...`) with:

```markdown
- formatting is clean for every file type (`make fmt` to fix):
  - Go via `gofumpt` + `gci` (enforced by `golangci-lint run`)
  - YAML via `yamlfmt -lint`
  - shell hooks via `shfmt -d`
- `go vet ./...`
- `golangci-lint run ./...` (config in `.golangci.yml`)
- `actionlint` on `.github/workflows/` (hermetic: `-shellcheck= -pyflakes=`)
- `govulncheck ./...`
- `go test -race ./...` (all packages)
- `go build ./...` on linux/macOS/windows

All tools run via pinned `go run tool@vX.Y.Z` — no Node/Python toolchain. Editor
hygiene is editor-agnostic via `.editorconfig`; line endings are normalized by
`.gitattributes`. The bulk `style:` reformat is listed in `.git-blame-ignore-revs`
(`git config blame.ignoreRevsFile .git-blame-ignore-revs` to honor it locally).
```

- [ ] **Step 3: Add the npm-distribution note to the `## LATER` section of `docs/superpowers/plans/BACKLOG.md`**

Append to the end of the `## LATER (demand-gated)` section:

```markdown

**npm distribution (future, release-time only).** Ship the existing `sloppy`/`sloppyd`
binaries as an `@sloppyjoe/sloppy` npm package (`npx` / `npm i -g`) via GoReleaser's
`npms:` block — **no Node in the dev or CI loop** (GoReleaser produces the package at
release time; the source stays Go-native). Prerequisites: `npms` is **GoReleaser
Pro-only** (since v2.8) → needs a Pro license key + an `NPM_TOKEN` CI secret (current
`.goreleaser.yaml` is OSS-only). Gate prereleases with `disable: "{{ ne .Prerelease \"\" }}"`
(note: `disable: "{{ .Prerelease }}"` never disables — only the literal `true` does).
Supply chain: the generated package uses a `postinstall` that downloads+extracts the
release archive (breaks under `npm install --ignore-scripts`); pair it with the repo's
existing checksums + cosign signing.
```

- [ ] **Step 4: Verify docs are still formatter-clean and commit**

Run: `make fmt-check`
Expected: exit 0 (Markdown is not auto-formatted, but this confirms no YAML/Go/shell drift was introduced).

```bash
git add CONTRIBUTING.md docs/superpowers/plans/BACKLOG.md
git commit -s -m "docs: document all-file-type make fmt + npm distribution backlog note"
```

---

### Task 8: Full local gate verification

**Files:** none (verification only).

- [ ] **Step 1: Run the complete gate**

Run: `make ci`
Expected: every stage green — `fmt-check` (Go/YAML/shell clean), `vet`, `lint` (0 issues), `lint-actions` (0 issues), `vulncheck` (no vulnerabilities), `test-race` (all packages PASS).

- [ ] **Step 2: Build the release binaries**

Run: `make build`
Expected: `bin/sloppy` and `bin/sloppyd` produced, no errors.

- [ ] **Step 3: Confirm the branch is ready to push (human step)**

Run: `git log --oneline main..HEAD`
Expected: the spec commit + the 7 task commits, in order. The branch is now ready for `git push -u origin chore/dev-tooling-formatters` and a PR (left to the human — pushing triggers CI, which should reproduce the green `make ci`).

---

## Self-Review

**Spec coverage:**
- §4 toolchain (gofumpt+gci / yamlfmt / shfmt / actionlint / editorconfig / gitattributes) → Tasks 1–3, 6. ✓
- §5 new files (.editorconfig, .gitattributes, .yamlfmt, .git-blame-ignore-revs) → Tasks 1, 2, 5. ✓
- §6 modified files (.golangci.yml, Makefile, pre-commit, ci.yml, CONTRIBUTING) → Tasks 3, 6, 7. ✓
- §4 enforcement nuance (`fmt --diff` not a gate; rely on `golangci-lint run`) → Task 3 Makefile `fmt-check` wrapper + CI `golangci-lint run` comment. ✓
- §7 one-time reformat sequencing (config → reformat → blame-ignore → enforce) → Tasks 3 → 4 → 5 → 6. ✓
- §8 npm forward-compat note (Pro-only + `disable` template fix) → Task 7 Step 3. ✓
- §9 out-of-scope items → not built (correct). ✓

**Placeholder scan:** The only `<SHA>` placeholder is in Task 5, with explicit instructions to substitute the real value from `git rev-parse HEAD` — not a plan placeholder. No TBD/TODO/"handle edge cases". ✓

**Type/name consistency:** Tool versions (`v2.12.2`, `v0.21.0`, `v3.13.1`, `v1.7.12`), the module path `github.com/sloppyjoe/sloppy`, target names (`fmt`, `fmt-check`, `lint`, `lint-actions`, `ci`), and the shfmt flags (`-ln posix -i 0 -s`) are identical everywhere they appear (Makefile, pre-commit, ci.yml). ✓
