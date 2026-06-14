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

// defaultCloudflareLimit is the rate_limiting_limit restored on Revert when the
// intent does not carry a prior_limit (the Cloudflare AI Gateway "unlimited"
// sentinel is the absence of a limit; we use a conservative non-zero default).
const defaultCloudflareLimit = 1000

// cloudflare applies throttle_tenant / disable_deployment to a Cloudflare AI
// Gateway via its admin REST API. Unlike the POST-based httpRouteActuator, the
// Cloudflare control surface is a single authenticated PUT to the gateway
// resource, so this adapter builds its own request rather than reusing the
// shared (POST-hardcoded) postJSON helper.
//
// baseURL embeds the account-scoped collection path, e.g.
// https://api.cloudflare.com/client/v4/accounts/{account_id}/ai-gateway/gateways
// and intent.Target is the gateway id appended as the final path segment.
//
// Auth is a Cloudflare API token sent as Authorization: Bearer, matching the
// TokenFunc/Bearer assumption shared with the other HTTP actuators.
type cloudflare struct {
	baseURL string
	token   TokenFunc
	client  *http.Client
}

// NewCloudflare builds an actuator for the Cloudflare AI Gateway admin API. It
// advertises throttle_tenant and disable_deployment ONLY (no new ActionKind):
// both map to the same reversible operation — Apply pins rate_limiting_limit=0
// (no requests admitted), Revert restores the prior limit. The prior limit is
// taken from intent.Args["prior_limit"] when present, else defaultCloudflareLimit.
func NewCloudflare(baseURL string, token TokenFunc) Actuator {
	return &cloudflare{baseURL: baseURL, token: token, client: &http.Client{Timeout: 10 * time.Second}}
}

func (a *cloudflare) Capabilities() []core.ActionKind {
	return []core.ActionKind{core.ActionThrottleTenant, core.ActionDisableDeployment}
}

// body returns the gateway PUT payload for an intent. On apply the limit is 0
// (throttled/disabled); on revert it is the prior limit (restore). The reversible
// inverse is selected by isRevert, mirroring the routeDest-style logic.
func cloudflareBody(i core.RemediationIntent, isRevert bool) map[string]any {
	limit := 0
	if isRevert {
		limit = defaultCloudflareLimit
		if pl, ok := i.Args["prior_limit"]; ok {
			switch v := pl.(type) {
			case int:
				limit = v
			case float64:
				limit = int(v)
			}
		}
	}
	return map[string]any{
		"rate_limiting_limit":     limit,
		"rate_limiting_interval":  60,
		"rate_limiting_technique": "fixed",
	}
}

func (a *cloudflare) put(ctx context.Context, i core.RemediationIntent, isRevert bool) error {
	tok, err := a.token()
	if err != nil {
		return err
	}
	buf, err := json.Marshal(cloudflareBody(i, isRevert))
	if err != nil {
		return err
	}
	url := a.baseURL + "/" + i.Target
	req, err := http.NewRequestWithContext(ctx, http.MethodPut, url, bytes.NewReader(buf))
	if err != nil {
		return err
	}
	req.Header.Set("Authorization", "Bearer "+tok)
	req.Header.Set("Content-Type", "application/json")
	resp, err := a.client.Do(req)
	if err != nil {
		return fmt.Errorf("cloudflare: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 300 {
		return fmt.Errorf("cloudflare: http %d", resp.StatusCode)
	}
	return nil
}

func (a *cloudflare) Apply(ctx context.Context, i core.RemediationIntent) (core.Receipt, error) {
	if err := a.put(ctx, i, false); err != nil {
		return core.Receipt{IntentID: i.ID, Actuator: "cloudflare", Outcome: core.OutcomeFailed}, err
	}
	return core.Receipt{
		IntentID: i.ID, Actuator: "cloudflare", AppliedAt: time.Now().UTC(),
		Before: i.Target, After: i.Target, Outcome: core.OutcomeApplied,
	}, nil
}

func (a *cloudflare) Revert(ctx context.Context, i core.RemediationIntent) (core.Receipt, error) {
	if err := a.put(ctx, i, true); err != nil {
		return core.Receipt{IntentID: i.ID, Actuator: "cloudflare", Outcome: core.OutcomeFailed}, err
	}
	return core.Receipt{IntentID: i.ID, Actuator: "cloudflare", AppliedAt: time.Now().UTC(), Outcome: core.OutcomeReverted}, nil
}
