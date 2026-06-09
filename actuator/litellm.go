package actuator

import "github.com/sloppyjoe/sloppy/core"

// NewLiteLLM builds an actuator for the LiteLLM admin API. It handles
// route_override (/model/update), throttle_tenant (/key/update rate limit), and
// disable_deployment (/model/update disabled flag). All are reversible.
//
// NOTE: the throttle/disable admin schemas are provisional — verify against a
// running LiteLLM (covered by the Plan 19 integration test).
func NewLiteLLM(baseURL string, token TokenFunc) Actuator {
	caps := []core.ActionKind{core.ActionRouteOverride, core.ActionThrottleTenant, core.ActionDisableDeployment}
	return newHTTPRoute("litellm", baseURL, token, caps, func(i core.RemediationIntent, isRevert bool) (string, map[string]any) {
		switch i.Kind {
		case core.ActionThrottleTenant:
			limit := 0 // 0 rpm = throttled
			if isRevert {
				limit = -1 // -1 = unlimited (restore)
			}
			return "/key/update", map[string]any{"key": i.Target, "rpm_limit": limit}
		case core.ActionDisableDeployment:
			return "/model/update", map[string]any{
				"model_name": i.Target,
				"model_info": map[string]any{"disabled": !isRevert},
			}
		default: // route_override
			return "/model/update", map[string]any{
				"model_name":     i.Target,
				"litellm_params": map[string]any{"model": routeDest(i, isRevert)},
				"model_info":     map[string]any{},
			}
		}
	})
}
