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

// requestFn builds a gateway's admin request (path + JSON body) for an intent.
// isRevert=true asks for the inverse (restore) request.
type requestFn func(i core.RemediationIntent, isRevert bool) (path string, body map[string]any)

// httpRouteActuator applies route_override via a gateway's HTTP admin API. The
// HTTP plumbing (token, post, receipt) is shared; each gateway supplies its own
// request shape via a requestFn, so LiteLLM != Bifrost != Envoy bodies.
type httpRouteActuator struct {
	name    string
	baseURL string
	token   TokenFunc
	caps    []core.ActionKind
	build   requestFn
	client  *http.Client
}

func newHTTPRoute(name, baseURL string, token TokenFunc, caps []core.ActionKind, build requestFn) Actuator {
	return &httpRouteActuator{name: name, baseURL: baseURL, token: token, caps: caps, build: build, client: &http.Client{Timeout: 10 * time.Second}}
}

func (a *httpRouteActuator) Capabilities() []core.ActionKind { return a.caps }

func (a *httpRouteActuator) do(ctx context.Context, i core.RemediationIntent, isRevert bool) error {
	tok, err := a.token()
	if err != nil {
		return err
	}
	path, body := a.build(i, isRevert)
	hdr := map[string]string{"Authorization": "Bearer " + tok, "Content-Type": "application/json"}
	if err := postJSON(ctx, a.client, a.baseURL+path, hdr, body); err != nil {
		return fmt.Errorf("%s: %w", a.name, err)
	}
	return nil
}

func (a *httpRouteActuator) Apply(ctx context.Context, i core.RemediationIntent) (core.Receipt, error) {
	if err := a.do(ctx, i, false); err != nil {
		return core.Receipt{IntentID: i.ID, Actuator: a.name, Outcome: core.OutcomeFailed}, err
	}
	to, _ := i.Args["to"].(string)
	return core.Receipt{
		IntentID: i.ID, Actuator: a.name, AppliedAt: time.Now().UTC(),
		Before: i.Target, After: to, Outcome: core.OutcomeApplied,
	}, nil
}

func (a *httpRouteActuator) Revert(ctx context.Context, i core.RemediationIntent) (core.Receipt, error) {
	if err := a.do(ctx, i, true); err != nil {
		return core.Receipt{IntentID: i.ID, Actuator: a.name, Outcome: core.OutcomeFailed}, err
	}
	return core.Receipt{IntentID: i.ID, Actuator: a.name, AppliedAt: time.Now().UTC(), Outcome: core.OutcomeReverted}, nil
}

// routeDest returns the destination model for an intent: args["to"] on apply,
// or the original target (restore self-route) on revert.
func routeDest(i core.RemediationIntent, isRevert bool) string {
	if isRevert {
		return i.Target
	}
	if to, ok := i.Args["to"].(string); ok {
		return to
	}
	return i.Target
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
