.PHONY: test build tidy vet

test:
	go test ./...

vet:
	go vet ./...

build:
	CGO_ENABLED=0 go build -o bin/sloppy ./cmd/sloppy

tidy:
	go mod tidy
