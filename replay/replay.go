// Package replay deterministically evaluates rules against recorded signals
// with no side effects — the basis for `sloppy test --replay` (a CI gate).
package replay

import (
	"github.com/sloppyjoe/sloppy/core"
	"github.com/sloppyjoe/sloppy/rules"
)

// Fired describes one intent a rule would produce.
type Fired struct {
	Rule   string
	Kind   string
	Target string
}

// Result is the dry-run outcome for one signal.
type Result struct {
	SignalID string
	Matched  bool
	Intents  []Fired
}

// Run evaluates every signal against the reconciler (pure; no actuation, no state writes).
func Run(rec *rules.Reconciler, signals []core.Signal) []Result {
	out := make([]Result, 0, len(signals))
	for _, sig := range signals {
		r := Result{SignalID: label(sig)}
		for _, m := range rec.EvaluateMatches(sig, nil) {
			r.Matched = true
			for _, in := range m.Intents {
				r.Intents = append(r.Intents, Fired{Rule: m.Rule.SHA, Kind: string(in.Kind), Target: in.Target})
			}
		}
		out = append(out, r)
	}
	return out
}

func label(s core.Signal) string {
	if s.ID != "" {
		return s.ID
	}
	return s.Type
}
