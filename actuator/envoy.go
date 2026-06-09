package actuator

import "github.com/sloppyjoe/sloppy/core"

// NewEnvoy builds a route_override actuator for Envoy AI Gateway.
//
// EXPERIMENTAL: a placeholder HTTP shim — the production Envoy AI Gateway control
// surface is Kubernetes CRD / xDS; this targets a provisional admin endpoint and
// should be replaced by a CRD/xDS driver before real use.
func NewEnvoy(baseURL string, token TokenFunc) Actuator {
	return newHTTPRoute("envoy", baseURL, token, func(i core.RemediationIntent, isRevert bool) (string, map[string]any) {
		return "/admin/routes/override", map[string]any{"model": i.Target, "to": routeDest(i, isRevert)}
	})
}
