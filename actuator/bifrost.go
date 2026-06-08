package actuator

// NewBifrost builds a route_override actuator for the Bifrost admin API.
// NOTE: the admin path is provisional — verify against a running Bifrost.
func NewBifrost(baseURL string, token TokenFunc) Actuator {
	return newHTTPRoute("bifrost", baseURL, "/api/route/override", token)
}
