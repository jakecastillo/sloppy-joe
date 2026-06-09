//go:build integration

package e2e

import (
	"io"
	"net/http"
	"os"
	"strings"
	"testing"
	"time"
)

// TestE2ECostSpikeRemediates drives a cost spike at a live sloppyd and asserts a
// remediation was applied. Skipped unless SLOPPY_E2E_BASE is set, e.g.:
//
//	docker compose up -d --build
//	SLOPPY_E2E_BASE=http://localhost:8723 go test -tags integration ./test/e2e/...
func TestE2ECostSpikeRemediates(t *testing.T) {
	base := os.Getenv("SLOPPY_E2E_BASE")
	if base == "" {
		t.Skip("set SLOPPY_E2E_BASE (e.g. http://localhost:8723) to run against the compose stack")
	}
	// The compose stack runs sloppyd with --auth on a network-reachable bind (the
	// bind guard refuses an unauthenticated public bind), so authenticated routes
	// require an api key. SLOPPY_E2E_API_KEY supplies it; /healthz stays public.
	apiKey := os.Getenv("SLOPPY_E2E_API_KEY")
	client := &http.Client{Timeout: 10 * time.Second}
	authed := func(req *http.Request) (*http.Response, error) {
		if apiKey != "" {
			req.Header.Set("X-API-Key", apiKey)
		}
		return client.Do(req)
	}

	resp, err := client.Get(base + "/healthz")
	if err != nil || resp.StatusCode != http.StatusOK {
		t.Fatalf("healthz: %v (status %v)", err, resp)
	}
	resp.Body.Close()

	body := `{"type":"cost.budget_burn","correlation_key":"e2e:cost","subject":{"tenant":"acme","alias":"gpt-4o"},"data":{"spend_1h_usd":99.0}}`
	sigReq, _ := http.NewRequest("POST", base+"/v1/signals", strings.NewReader(body))
	sigReq.Header.Set("Content-Type", "application/json")
	resp, err = authed(sigReq)
	if err != nil || resp.StatusCode != http.StatusOK {
		t.Fatalf("post signal failed: %v", err)
	}
	resp.Body.Close()

	statusReq, _ := http.NewRequest("GET", base+"/status", nil)
	resp, err = authed(statusReq)
	if err != nil {
		t.Fatalf("status: %v", err)
	}
	defer resp.Body.Close()
	out, _ := io.ReadAll(resp.Body)
	if !strings.Contains(string(out), `"intents_applied"`) {
		t.Fatalf("status missing intents_applied: %s", out)
	}
}
