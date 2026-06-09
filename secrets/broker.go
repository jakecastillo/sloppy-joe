// Package secrets holds the minimal-surface token broker. Provider keys never
// live here — only scoped admin/notify tokens. In-proc for v0, sidecar-ready.
package secrets

import (
	"fmt"
	"os"
	"strings"
)

// Broker hands out scoped tokens just-in-time. Default-deny by allowlist.
type Broker interface {
	Get(capability string) (string, error)
}

type envBroker struct{ allowed map[string]bool }

// NewEnvBroker reads tokens from SLOPPY_TOKEN_<CAP> env vars, allowlisted by capability.
func NewEnvBroker(allowed []string) Broker {
	m := map[string]bool{}
	for _, a := range allowed {
		m[a] = true
	}
	return &envBroker{allowed: m}
}

func (b *envBroker) Get(capability string) (string, error) {
	if !b.allowed[capability] {
		return "", fmt.Errorf("secrets: capability %q not allowed (default-deny)", capability)
	}
	key := "SLOPPY_TOKEN_" + strings.ToUpper(capability)
	// File-backed secret (the canonical container/k8s path, e.g. /run/secrets) wins
	// over the raw env var so operators can mount secrets without exporting them.
	if path := os.Getenv(key + "_FILE"); path != "" {
		data, err := os.ReadFile(path)
		if err != nil {
			return "", fmt.Errorf("secrets: read %s: %w", key+"_FILE", err)
		}
		v := strings.TrimSpace(string(data))
		if v == "" {
			return "", fmt.Errorf("secrets: %s is empty", key+"_FILE")
		}
		return v, nil
	}
	v := os.Getenv(key)
	if v == "" {
		return "", fmt.Errorf("secrets: no token for %q", capability)
	}
	return v, nil
}
