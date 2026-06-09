// Package e2e holds end-to-end tests that drive a live sloppyd (the
// docker-compose stack). They are guarded by the `integration` build tag and
// skip unless SLOPPY_E2E_BASE points at a running daemon, so the normal suite
// (`go test ./...`) never touches them.
package e2e
