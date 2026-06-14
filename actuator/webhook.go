package actuator

import "github.com/sloppyjoe/sloppy/core"

// NewWebhook builds a generic route_override actuator for any service exposing a
// simple webhook admin API. It POSTs {"model": <target>, "to": <dest>} to /route,
// where dest is the override destination on apply and the original target (self-
// route restore) on revert.
//
// This is the lowest-common-denominator shape for integrations that don't have a
// dedicated actuator (LiteLLM/Bifrost/Envoy); point baseURL at the receiver.
func NewWebhook(baseURL string, token TokenFunc) Actuator {
	return newHTTPRoute("webhook", baseURL, token, []core.ActionKind{core.ActionRouteOverride}, func(i core.RemediationIntent, isRevert bool) (string, map[string]any) {
		return "/route", map[string]any{"model": i.Target, "to": routeDest(i, isRevert)}
	})
}
