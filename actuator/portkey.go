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

// portkeyStatusActive is the config 'status' restored on Revert; Apply pins it to
// portkeyStatusInactive to model disable_deployment (the named Gateway Config is
// taken out of rotation without deleting it).
const (
	portkeyStatusActive   = "active"
	portkeyStatusInactive = "inactive"
)

// portkey applies route_override / disable_deployment to a Portkey AI Gateway by
// updating a named Gateway Config via its Admin API. Like the Cloudflare adapter
// (and unlike the POST-based httpRouteActuator), the Portkey control surface is a
// single authenticated PUT to the config resource, so this adapter builds its own
// request rather than reusing the shared (POST-hardcoded) postJSON helper. It
// also differs from Cloudflare in its auth: Portkey expects the token in the
// x-portkey-api-key header, not Authorization: Bearer.
//
// baseURL embeds the control-plane configs collection path, e.g.
// https://api.portkey.ai/v1/configs, and intent.Target is the config slug
// appended as the final path segment (PUT /configs/{slug}).
//
// Because PUT is a full replace of the named config, Revert restores the prior
// routing config and status carried in intent.Args (mirroring Cloudflare's
// prior_limit) so unrelated fields are not clobbered.
type portkey struct {
	baseURL string
	token   TokenFunc
	client  *http.Client
}

// NewPortkey builds an actuator for the Portkey AI Gateway Admin API. It
// advertises route_override and disable_deployment ONLY (no new ActionKind):
//   - Apply pins the config's routing target to intent.Args["to"] and sets
//     status=inactive (disabled); Revert restores the prior routing config from
//     intent.Args["prior_config"] (when present) and status=active.
//
// The prior config is taken from intent.Args["prior_config"] so a full-replace
// PUT on Revert does not drop unrelated config fields.
func NewPortkey(baseURL string, token TokenFunc) Actuator {
	return &portkey{baseURL: baseURL, token: token, client: &http.Client{Timeout: 10 * time.Second}}
}

func (a *portkey) Capabilities() []core.ActionKind {
	return []core.ActionKind{core.ActionRouteOverride, core.ActionDisableDeployment}
}

// portkeyBody returns the config PUT payload for an intent. On apply the routing
// target is overridden (config.strategy points at args["to"]) and status is
// pinned inactive; on revert the prior config is restored from
// intent.Args["prior_config"] and status returns to active. The reversible
// inverse is selected by isRevert, mirroring the Cloudflare cloudflareBody logic.
func portkeyBody(i core.RemediationIntent, isRevert bool) map[string]any {
	if isRevert {
		body := map[string]any{"status": portkeyStatusActive}
		if pc, ok := i.Args["prior_config"].(map[string]any); ok {
			body["config"] = pc
		}
		return body
	}
	to, _ := i.Args["to"].(string)
	return map[string]any{
		"status": portkeyStatusInactive,
		"config": map[string]any{
			"strategy": map[string]any{"mode": "single"},
			"targets":  []any{map[string]any{"override_params": map[string]any{"model": to}}},
		},
	}
}

func (a *portkey) put(ctx context.Context, i core.RemediationIntent, isRevert bool) error {
	tok, err := a.token()
	if err != nil {
		return err
	}
	buf, err := json.Marshal(portkeyBody(i, isRevert))
	if err != nil {
		return err
	}
	url := a.baseURL + "/" + i.Target
	req, err := http.NewRequestWithContext(ctx, http.MethodPut, url, bytes.NewReader(buf))
	if err != nil {
		return err
	}
	req.Header.Set("x-portkey-api-key", tok)
	req.Header.Set("Content-Type", "application/json")
	resp, err := a.client.Do(req)
	if err != nil {
		return fmt.Errorf("portkey: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 300 {
		return fmt.Errorf("portkey: http %d", resp.StatusCode)
	}
	return nil
}

func (a *portkey) Apply(ctx context.Context, i core.RemediationIntent) (core.Receipt, error) {
	if err := a.put(ctx, i, false); err != nil {
		return core.Receipt{IntentID: i.ID, Actuator: "portkey", Outcome: core.OutcomeFailed}, err
	}
	to, _ := i.Args["to"].(string)
	return core.Receipt{
		IntentID: i.ID, Actuator: "portkey", AppliedAt: time.Now().UTC(),
		Before: i.Target, After: to, Outcome: core.OutcomeApplied,
	}, nil
}

func (a *portkey) Revert(ctx context.Context, i core.RemediationIntent) (core.Receipt, error) {
	if err := a.put(ctx, i, true); err != nil {
		return core.Receipt{IntentID: i.ID, Actuator: "portkey", Outcome: core.OutcomeFailed}, err
	}
	return core.Receipt{IntentID: i.ID, Actuator: "portkey", AppliedAt: time.Now().UTC(), Outcome: core.OutcomeReverted}, nil
}
