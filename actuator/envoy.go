package actuator

// NewEnvoy builds a route_override actuator for Envoy AI Gateway.
// NOTE: placeholder HTTP shim — the production Envoy AI Gateway control surface
// is Kubernetes CRD / xDS; this adapter targets a provisional admin endpoint and
// should be replaced by a CRD/xDS driver before real use.
func NewEnvoy(baseURL string, token TokenFunc) Actuator {
	return newHTTPRoute("envoy", baseURL, "/admin/routes/override", token)
}
