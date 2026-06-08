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

// Slack posts to an incoming webhook URL.
type Slack struct {
	webhook string
	client  *http.Client
}

// NewSlack builds the adapter from a webhook URL.
func NewSlack(webhook string) *Slack {
	return &Slack{webhook: webhook, client: &http.Client{Timeout: 10 * time.Second}}
}

func (s *Slack) Capabilities() []core.ActionKind { return []core.ActionKind{core.ActionPage} }

func (s *Slack) Apply(ctx context.Context, i core.RemediationIntent) (core.Receipt, error) {
	channel, _ := i.Args["slack"].(string)
	payload := map[string]any{"text": fmt.Sprintf("🥪 Sloppy Joe fired %s on %s (%s)", i.Kind, i.Target, channel)}
	buf, _ := json.Marshal(payload)
	req, _ := http.NewRequestWithContext(ctx, http.MethodPost, s.webhook, bytes.NewReader(buf))
	req.Header.Set("Content-Type", "application/json")
	resp, err := s.client.Do(req)
	if err != nil {
		return core.Receipt{IntentID: i.ID, Actuator: "slack", Outcome: core.OutcomeFailed}, err
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 300 {
		return core.Receipt{IntentID: i.ID, Actuator: "slack", Outcome: core.OutcomeFailed}, fmt.Errorf("slack: %d", resp.StatusCode)
	}
	return core.Receipt{IntentID: i.ID, Actuator: "slack", AppliedAt: time.Now().UTC(), Outcome: core.OutcomeApplied}, nil
}

func (s *Slack) Revert(_ context.Context, i core.RemediationIntent) (core.Receipt, error) {
	return core.Receipt{IntentID: i.ID, Actuator: "slack", Outcome: core.OutcomeReverted}, nil
}
