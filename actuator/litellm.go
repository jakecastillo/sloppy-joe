package actuator

import "github.com/sloppyjoe/sloppy/core"

// NewLiteLLM builds a route_override actuator for the LiteLLM admin API,
// using its /model/update schema (model_name + litellm_params{model} + model_info).
func NewLiteLLM(baseURL string, token TokenFunc) Actuator {
	return newHTTPRoute("litellm", baseURL, token, func(i core.RemediationIntent, isRevert bool) (string, map[string]any) {
		return "/model/update", map[string]any{
			"model_name":     i.Target,
			"litellm_params": map[string]any{"model": routeDest(i, isRevert)},
			"model_info":     map[string]any{},
		}
	})
}
