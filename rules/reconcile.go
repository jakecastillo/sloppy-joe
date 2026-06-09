package rules

import (
	"time"

	"github.com/sloppyjoe/sloppy/core"
)

type compiledRule struct {
	rule Rule
	cond *Condition
}

// Reconciler evaluates compiled rules against a signal.
type Reconciler struct{ rules []compiledRule }

// NewReconciler compiles all rule conditions up front.
func NewReconciler(rs []Rule) (*Reconciler, error) {
	out := make([]compiledRule, 0, len(rs))
	for _, r := range rs {
		c, err := CompileCondition(r.When)
		if err != nil {
			return nil, err
		}
		out = append(out, compiledRule{rule: r, cond: c})
	}
	return &Reconciler{rules: out}, nil
}

// Match is a fired rule together with the intents it produced. The Rule is
// carried so the engine can enforce `for:` windowing and governance.
type Match struct {
	Rule    Rule
	Intents []core.RemediationIntent
}

// EvaluateMatches returns every matching rule (type + condition) with its intents.
// Pure decision function; the engine handles `for:` windowing, idempotency, governance.
func (rc *Reconciler) EvaluateMatches(sig core.Signal, state map[string]any) []Match {
	var matches []Match
	for _, cr := range rc.rules {
		if cr.rule.On != sig.Type {
			continue
		}
		ok, err := cr.cond.Eval(sig, state)
		if err != nil || !ok {
			continue
		}
		ins := make([]core.RemediationIntent, 0, len(cr.rule.Then))
		for _, a := range cr.rule.Then {
			ins = append(ins, actionToIntent(a, cr.rule, sig))
		}
		matches = append(matches, Match{Rule: cr.rule, Intents: ins})
	}
	return matches
}

// Reconcile returns the flattened intents that fire for this signal+state.
func (rc *Reconciler) Reconcile(sig core.Signal, state map[string]any) []core.RemediationIntent {
	var intents []core.RemediationIntent
	for _, m := range rc.EvaluateMatches(sig, state) {
		intents = append(intents, m.Intents...)
	}
	return intents
}

// Cleared returns the `rollback: on_clear` rules of this signal's type whose
// condition is now FALSE (the incident has cleared), so the engine can revert
// their outstanding intents.
func (rc *Reconciler) Cleared(sig core.Signal, state map[string]any) []Rule {
	var out []Rule
	for _, cr := range rc.rules {
		if cr.rule.On != sig.Type || cr.rule.With.Rollback != "on_clear" {
			continue
		}
		if ok, err := cr.cond.Eval(sig, state); err == nil && !ok {
			out = append(out, cr.rule)
		}
	}
	return out
}

func actionToIntent(a Action, r Rule, sig core.Signal) core.RemediationIntent {
	target := sig.Subject.Alias
	ttl := time.Duration(0)
	if s, ok := a.Args["ttl"].(string); ok {
		if d, err := time.ParseDuration(s); err == nil {
			ttl = d
		}
	}
	id := core.DeterministicID(string(a.Kind), target, r.SHA, sig.CorrelationKey)
	return core.RemediationIntent{
		ID:      id,
		Kind:    core.ActionKind(a.Kind),
		Target:  target,
		Args:    a.Args,
		TTL:     ttl,
		DryRun:  r.With.DryRun,
		RuleSHA: r.SHA,
	}
}
