package actuator

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	"github.com/sloppyjoe/sloppy/core"
)

// TokenFunc returns a scoped admin token just-in-time (from the secret broker).
type TokenFunc func() (string, error)

// httpRouteActuator applies route_override via a gateway's HTTP admin API.
// Shared by the LiteLLM, Bifrost, and Envoy adapters — only the path differs.
type httpRouteActuator struct {
	name    string
	baseURL string
	path    string
	token   TokenFunc
	client  *http.Client
}

func newHTTPRoute(name, baseURL, path string, token TokenFunc) Actuator {
	return &httpRouteActuator{name: name, baseURL: baseURL, path: path, token: token, client: &http.Client{Timeout: 10 * time.Second}}
}

func (a *httpRouteActuator) Capabilities() []core.ActionKind {
	return []core.ActionKind{core.ActionRouteOverride}
}

// postJSON marshals body, POSTs it with the given headers, and checks status < 300.
// Shared by every HTTP actuator so the marshal/request/close/status dance lives once.
func postJSON(ctx context.Context, c *http.Client, url string, hdr map[string]string, body any) error {
	buf, err := json.Marshal(body)
	if err != nil {
		return err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(buf))
	if err != nil {
		return err
	}
	for k, v := range hdr {
		req.Header.Set(k, v)
	}
	resp, err := c.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 300 {
		return fmt.Errorf("http %d", resp.StatusCode)
	}
	return nil
}

func (a *httpRouteActuator) post(ctx context.Context, body map[string]any) error {
	tok, err := a.token()
	if err != nil {
		return err
	}
	hdr := map[string]string{"Authorization": "Bearer " + tok, "Content-Type": "application/json"}
	if err := postJSON(ctx, a.client, a.baseURL+a.path, hdr, body); err != nil {
		return fmt.Errorf("%s: %w", a.name, err)
	}
	return nil
}

func (a *httpRouteActuator) Apply(ctx context.Context, i core.RemediationIntent) (core.Receipt, error) {
	to, _ := i.Args["to"].(string)
	if err := a.post(ctx, map[string]any{"model": i.Target, "to": to}); err != nil {
		return core.Receipt{IntentID: i.ID, Actuator: a.name, Outcome: core.OutcomeFailed}, err
	}
	return core.Receipt{IntentID: i.ID, Actuator: a.name, AppliedAt: time.Now().UTC(),
		Before: i.Target, After: to, Outcome: core.OutcomeApplied}, nil
}

func (a *httpRouteActuator) Revert(ctx context.Context, i core.RemediationIntent) (core.Receipt, error) {
	if err := a.post(ctx, map[string]any{"model": i.Target, "to": i.Target}); err != nil {
		return core.Receipt{IntentID: i.ID, Actuator: a.name, Outcome: core.OutcomeFailed}, err
	}
	return core.Receipt{IntentID: i.ID, Actuator: a.name, AppliedAt: time.Now().UTC(), Outcome: core.OutcomeReverted}, nil
}
