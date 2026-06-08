package actuator

import (
	"context"
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
	if err := postJSON(ctx, s.client, s.webhook, map[string]string{"Content-Type": "application/json"}, payload); err != nil {
		return core.Receipt{IntentID: i.ID, Actuator: "slack", Outcome: core.OutcomeFailed}, err
	}
	return core.Receipt{IntentID: i.ID, Actuator: "slack", AppliedAt: time.Now().UTC(), Outcome: core.OutcomeApplied}, nil
}

func (s *Slack) Revert(_ context.Context, i core.RemediationIntent) (core.Receipt, error) {
	return core.Receipt{IntentID: i.ID, Actuator: "slack", Outcome: core.OutcomeReverted}, nil
}
