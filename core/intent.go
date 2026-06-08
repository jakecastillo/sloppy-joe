package core

import (
	"encoding/json"
	"sort"
	"strings"
	"time"
)

// ActionKind enumerates the governed actions a rule may request.
type ActionKind string

const (
	ActionRouteOverride ActionKind = "route_override"
	ActionOpenIssue     ActionKind = "open_issue"
	ActionPage          ActionKind = "page"
)

// RemediationIntent is a signed, reversible request to change the world.
type RemediationIntent struct {
	ID        string         `json:"id"`
	Kind      ActionKind     `json:"kind"`
	Target    string         `json:"target"`
	Args      map[string]any `json:"args,omitempty"`
	TTL       time.Duration  `json:"ttl,omitempty"`
	DryRun    bool           `json:"dry_run,omitempty"`
	Evidence  []Evidence     `json:"evidence,omitempty"`
	RuleSHA   string         `json:"rule_sha"`
	Signature string         `json:"signature,omitempty"`
}

// CanonicalBytes returns a deterministic serialization (sorted arg keys,
// signature excluded) suitable for signing and idempotency keys.
func (i RemediationIntent) CanonicalBytes() []byte {
	var sb strings.Builder
	sb.WriteString(i.ID + "|" + string(i.Kind) + "|" + i.Target + "|" + i.RuleSHA + "|" + i.TTL.String() + "|")
	keys := make([]string, 0, len(i.Args))
	for k := range i.Args {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	for _, k := range keys {
		v, _ := json.Marshal(i.Args[k])
		sb.WriteString(k + "=" + string(v) + ";")
	}
	return []byte(sb.String())
}
