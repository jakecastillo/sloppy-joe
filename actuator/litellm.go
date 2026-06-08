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

// LiteLLM applies route_override via the LiteLLM admin API.
type LiteLLM struct {
	baseURL string
	token   TokenFunc
	client  *http.Client
}

// NewLiteLLM builds the adapter. baseURL is the LiteLLM admin endpoint.
func NewLiteLLM(baseURL string, token TokenFunc) *LiteLLM {
	return &LiteLLM{baseURL: baseURL, token: token, client: &http.Client{Timeout: 10 * time.Second}}
}

func (l *LiteLLM) Capabilities() []core.ActionKind {
	return []core.ActionKind{core.ActionRouteOverride}
}

func (l *LiteLLM) post(ctx context.Context, body map[string]any) error {
	tok, err := l.token()
	if err != nil {
		return err
	}
	buf, _ := json.Marshal(body)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, l.baseURL+"/model/update", bytes.NewReader(buf))
	if err != nil {
		return err
	}
	req.Header.Set("Authorization", "Bearer "+tok)
	req.Header.Set("Content-Type", "application/json")
	resp, err := l.client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 300 {
		return fmt.Errorf("litellm: admin returned %d", resp.StatusCode)
	}
	return nil
}

// Apply reroutes Target → args["to"].
func (l *LiteLLM) Apply(ctx context.Context, i core.RemediationIntent) (core.Receipt, error) {
	to, _ := i.Args["to"].(string)
	if err := l.post(ctx, map[string]any{"model": i.Target, "to": to}); err != nil {
		return core.Receipt{IntentID: i.ID, Actuator: "litellm", Outcome: core.OutcomeFailed}, err
	}
	return core.Receipt{IntentID: i.ID, Actuator: "litellm", AppliedAt: time.Now().UTC(),
		Before: i.Target, After: to, Outcome: core.OutcomeApplied}, nil
}

// Revert restores Target to route to itself.
func (l *LiteLLM) Revert(ctx context.Context, i core.RemediationIntent) (core.Receipt, error) {
	if err := l.post(ctx, map[string]any{"model": i.Target, "to": i.Target}); err != nil {
		return core.Receipt{IntentID: i.ID, Actuator: "litellm", Outcome: core.OutcomeFailed}, err
	}
	return core.Receipt{IntentID: i.ID, Actuator: "litellm", AppliedAt: time.Now().UTC(), Outcome: core.OutcomeReverted}, nil
}
