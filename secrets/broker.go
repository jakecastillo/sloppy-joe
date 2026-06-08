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
	v := os.Getenv("SLOPPY_TOKEN_" + strings.ToUpper(capability))
	if v == "" {
		return "", fmt.Errorf("secrets: no token for %q", capability)
	}
	return v, nil
}
