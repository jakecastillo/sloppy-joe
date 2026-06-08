package actuator

// NewLiteLLM builds a route_override actuator for the LiteLLM admin API.
func NewLiteLLM(baseURL string, token TokenFunc) Actuator {
	return newHTTPRoute("litellm", baseURL, "/model/update", token)
}
