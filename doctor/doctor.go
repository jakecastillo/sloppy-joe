// Package doctor runs connectivity/capability checks for `sloppy doctor`.
package doctor

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"os"
	"sort"
	"strings"
	"time"

	"github.com/sloppyjoe/sloppy/config"
	"github.com/sloppyjoe/sloppy/core"
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
		if errors.Is(err, os.ErrNotExist) {
			// Replace the raw OS syscall text (e.g. "GetFileAttributesEx
			// rules: The system cannot find the file specified.") with an
			// actionable message naming the path and the --rules remedy.
			return Check{"rules", false, fmt.Sprintf(
				"rules path %s not found; create it and add *.yaml rule files, "+
					"or point at an existing one with --rules <dir|file>", path)}
		}
		// An existing-but-empty rules directory (e.g. the one `sloppy init`
		// scaffolds before you write any rules) is fine: recipes may already
		// cover you, so treat it as informational rather than a hard fail.
		if info, statErr := os.Stat(path); statErr == nil && info.IsDir() {
			return Check{"rules", true, fmt.Sprintf(
				"%s exists but has no rule files yet (add *.yaml rules, or rely on recipes)", path)}
		}
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

// CheckLedger verifies the cost-ledger usage store is queryable.
func CheckLedger(path string) Check {
	st, err := state.OpenSQLite(path)
	if err != nil {
		return Check{"ledger", false, err.Error()}
	}
	defer st.Close()
	if _, err := st.SpendSince(context.Background(), "_doctor", time.Now().Add(-time.Hour)); err != nil {
		return Check{"ledger", false, err.Error()}
	}
	return Check{"ledger", true, "usage store queryable"}
}

// CheckActuators reports which action kinds the registry can handle.
func CheckActuators(kinds []core.ActionKind) Check {
	if len(kinds) == 0 {
		return Check{"actuators", false, "no actuators registered"}
	}
	names := make([]string, len(kinds))
	for i, k := range kinds {
		names[i] = string(k)
	}
	return Check{"actuators", true, fmt.Sprintf("%d kind(s): %s", len(kinds), strings.Join(names, ", "))}
}

// CheckPlatforms reports which platforms are enabled and whether their token_env
// (or its *_FILE form) is present. It NEVER prints a token value.
func CheckPlatforms(eff config.Effective) Check {
	var enabled, missing []string
	for name, p := range eff.Platforms {
		if !p.Enabled {
			continue
		}
		enabled = append(enabled, name)
		if p.TokenEnv != "" && os.Getenv(p.TokenEnv) == "" && os.Getenv(p.TokenEnv+"_FILE") == "" {
			missing = append(missing, name)
		}
	}
	sort.Strings(enabled)
	sort.Strings(missing)
	if len(enabled) == 0 {
		return Check{"platforms", true, "none enabled (Log only)"}
	}
	if len(missing) > 0 {
		return Check{"platforms", false, fmt.Sprintf("enabled: %s; missing token: %s",
			strings.Join(enabled, ", "), strings.Join(missing, ", "))}
	}
	return Check{"platforms", true, fmt.Sprintf("enabled: %s (tokens present)", strings.Join(enabled, ", "))}
}

// CheckLiteLLM probes a LiteLLM admin endpoint when LiteLLM is enabled.
//
// When LiteLLM is disabled in the effective config the probe is informational:
// a fresh scaffold ships litellm.enabled=false, and an unreachable endpoint on a
// platform the operator has not turned on must not fail `sloppy doctor`. Pass the
// effective enabled flag so a disabled platform reports OK-but-skipped.
func CheckLiteLLM(enabled bool, url string) Check {
	if !enabled {
		return Check{"litellm", true, "disabled (probe skipped)"}
	}
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
