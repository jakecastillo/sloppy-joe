package rules

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"time"

	"gopkg.in/yaml.v3"
)

type rawRule struct {
	On   string                      `yaml:"on"`
	When string                      `yaml:"when"`
	For  string                      `yaml:"for"`
	Then []map[string]map[string]any `yaml:"then"`
	With With                        `yaml:"with"`
}

// ParseRules parses one YAML document into Rules (one doc = one rule in v0).
func ParseRules(b []byte) ([]Rule, error) {
	var raw rawRule
	if err := yaml.Unmarshal(b, &raw); err != nil {
		return nil, fmt.Errorf("rules: yaml: %w", err)
	}
	if raw.On == "" {
		return nil, fmt.Errorf("rules: missing `on:`")
	}
	if raw.When == "" {
		return nil, fmt.Errorf("rules: missing `when:`")
	}
	var dur time.Duration
	if raw.For != "" {
		d, err := time.ParseDuration(raw.For)
		if err != nil {
			return nil, fmt.Errorf("rules: bad `for:` %q: %w", raw.For, err)
		}
		dur = d
	}
	var actions []Action
	for _, m := range raw.Then {
		for kind, args := range m {
			actions = append(actions, Action{Kind: kind, Args: args})
		}
	}
	if len(actions) == 0 {
		return nil, fmt.Errorf("rules: `then:` must list at least one action")
	}
	sum := sha256.Sum256(b)
	return []Rule{{
		On: raw.On, When: raw.When, For: dur, Then: actions, With: raw.With,
		SHA: hex.EncodeToString(sum[:])[:12],
	}}, nil
}
