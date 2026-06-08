// Package rules loads, compiles, and evaluates Sloppy Joe rules.
package rules

import "time"

// Action is one entry in a rule's `then:` list.
type Action struct {
	Kind string
	Args map[string]any
}

// With holds governance knobs.
type With struct {
	DryRun       bool   `yaml:"dry_run"`
	IntentBudget string `yaml:"intent_budget"`
	Rollback     string `yaml:"rollback"`
}

// Rule is a parsed, Git-versioned governance rule.
type Rule struct {
	On   string
	When string
	For  time.Duration
	Then []Action
	With With
	SHA  string // content hash for audit provenance
}
