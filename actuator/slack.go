package actuator

import (
	"context"
	"fmt"
	"net/http"
	"time"

	"github.com/sloppyjoe/sloppy/core"
)

// Slack posts to an incoming-webhook URL. The webhook URL IS a bearer credential
// (anyone holding it can post), so it is resolved just-in-time through a TokenFunc
// (the SLOPPY_TOKEN_* broker) and never stored inline in config or on the struct.
type Slack struct {
	token  TokenFunc
	client *http.Client
}

// NewSlack builds the adapter. token resolves the incoming-webhook URL (the secret).
func NewSlack(token TokenFunc) *Slack {
	return &Slack{token: token, client: &http.Client{Timeout: 10 * time.Second}}
}

func (s *Slack) Capabilities() []core.ActionKind { return []core.ActionKind{core.ActionPage} }

func (s *Slack) Apply(ctx context.Context, i core.RemediationIntent) (core.Receipt, error) {
	webhook, err := s.token()
	if err != nil {
		return core.Receipt{IntentID: i.ID, Actuator: "slack", Outcome: core.OutcomeFailed}, err
	}
	channel, _ := i.Args["slack"].(string)
	payload := map[string]any{"text": fmt.Sprintf("🥪 Sloppy Joe fired %s on %s (%s)", i.Kind, i.Target, channel)}
	if err := postJSON(ctx, s.client, webhook, map[string]string{"Content-Type": "application/json"}, payload); err != nil {
		return core.Receipt{IntentID: i.ID, Actuator: "slack", Outcome: core.OutcomeFailed}, err
	}
	return core.Receipt{IntentID: i.ID, Actuator: "slack", AppliedAt: time.Now().UTC(), Outcome: core.OutcomeApplied}, nil
}

func (s *Slack) Revert(_ context.Context, i core.RemediationIntent) (core.Receipt, error) {
	return core.Receipt{IntentID: i.ID, Actuator: "slack", Outcome: core.OutcomeReverted}, nil
}
