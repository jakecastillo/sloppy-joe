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
