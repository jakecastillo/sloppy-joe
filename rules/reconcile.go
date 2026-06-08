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

// Reconcile returns the intents that fire for this signal+state.
// Pure decision function; the engine handles idempotency and governance.
func (rc *Reconciler) Reconcile(sig core.Signal, state map[string]any) []core.RemediationIntent {
	var intents []core.RemediationIntent
	for _, cr := range rc.rules {
		if cr.rule.On != sig.Type {
			continue
		}
		ok, err := cr.cond.Eval(sig, state)
		if err != nil || !ok {
			continue
		}
		for _, a := range cr.rule.Then {
			intents = append(intents, actionToIntent(a, cr.rule, sig))
		}
	}
	return intents
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
