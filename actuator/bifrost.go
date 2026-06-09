package actuator

import "github.com/sloppyjoe/sloppy/core"

// NewBifrost builds a route_override actuator for the Bifrost admin API.
//
// EXPERIMENTAL: the admin path + body are provisional — verify against a running
// Bifrost before relying on it (covered by the Plan 19 integration test).
func NewBifrost(baseURL string, token TokenFunc) Actuator {
	return newHTTPRoute("bifrost", baseURL, token, func(i core.RemediationIntent, isRevert bool) (string, map[string]any) {
		return "/api/route/override", map[string]any{"model": i.Target, "to": routeDest(i, isRevert)}
	})
}
