// Package doctor runs connectivity/capability checks for `sloppy doctor`.
package doctor

import (
	"fmt"
	"net/http"
	"time"

	"github.com/sloppyjoe/sloppy/config"
	"github.com/sloppyjoe/sloppy/state"
)

// Check is one diagnostic result.
type Check struct {
	Name   string
	OK     bool
	Detail string
}

// CheckRules verifies rules load and parse.
func CheckRules(path string) Check {
	rs, err := config.LoadRules(path)
	if err != nil {
		return Check{"rules", false, err.Error()}
	}
	return Check{"rules", true, fmt.Sprintf("%d rule(s) loaded", len(rs))}
}

// CheckDB verifies the state store opens (and migrates).
func CheckDB(path string) Check {
	st, err := state.OpenSQLite(path)
	if err != nil {
		return Check{"state-db", false, err.Error()}
	}
	_ = st.Close()
	return Check{"state-db", true, "opens + migrates ok"}
}

// CheckLiteLLM probes a LiteLLM admin endpoint if configured.
func CheckLiteLLM(url string) Check {
	if url == "" {
		return Check{"litellm", true, "not configured (skipped)"}
	}
	client := &http.Client{Timeout: 3 * time.Second}
	resp, err := client.Get(url + "/health")
	if err != nil {
		return Check{"litellm", false, "unreachable: " + err.Error()}
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 500 {
		return Check{"litellm", false, fmt.Sprintf("status %d", resp.StatusCode)}
	}
	return Check{"litellm", true, fmt.Sprintf("reachable (status %d)", resp.StatusCode)}
}
