.PHONY: test test-race cover build fmt vet lint vulncheck tidy ci

# Mirror the CI gates so `make ci` reproduces the pipeline locally.
ci: fmt-check vet lint vulncheck test-race

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

fmt:
	gofmt -w .

fmt-check:
	@out=$$(gofmt -l .); if [ -n "$$out" ]; then echo "unformatted:"; echo "$$out"; exit 1; fi

vet:
	go vet ./...

lint:
	go run github.com/golangci/golangci-lint/v2/cmd/golangci-lint@latest run ./...

vulncheck:
	go run golang.org/x/vuln/cmd/govulncheck@latest ./...

tidy:
	go mod tidy
