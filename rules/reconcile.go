package rules

import (
	"strings"
	"time"

	"github.com/sloppyjoe/sloppy/core"
)

type compiledRule struct {
	rule Rule
	cond *Condition
}

// Reconciler evaluates compiled rules against a signal. Rules are indexed by
// their `on:` signal type so each evaluation iterates only the same-type bucket
// instead of scanning every compiled rule. Within a bucket, rules retain their
// original (parse) order, preserving fired-intent ordering-within-type.
type Reconciler struct{ byType map[string][]compiledRule }

// NewReconciler compiles all rule conditions up front and indexes them by the
// `on:` signal type. Iteration order within each type bucket matches the input
// rule order.
func NewReconciler(rs []Rule) (*Reconciler, error) {
	byType := make(map[string][]compiledRule, len(rs))
	for _, r := range rs {
		c, err := CompileCondition(r.When)
		if err != nil {
			return nil, err
		}
		byType[r.On] = append(byType[r.On], compiledRule{rule: r, cond: c})
	}
	return &Reconciler{byType: byType}, nil
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
	for _, cr := range rc.byType[sig.Type] {
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
	for _, cr := range rc.byType[sig.Type] {
		if cr.rule.With.Rollback != "on_clear" {
			continue
		}
		if ok, err := cr.cond.Eval(sig, state); err == nil && !ok {
			out = append(out, cr.rule)
		}
	}
	return out
}

// StateDependentRules returns the rules of the given signal type whose `when`
// condition references ledger-derived `state.*` (e.g. cost-runaway guards). The
// engine uses this to avoid silently non-matching such rules when the state read
// fails: an unreadable `state.spend_1h_usd` must be treated as inconclusive, not
// as a clean below-threshold non-match. A signal-only rule (no `state.` ref) is
// unaffected by a state-store blip and is excluded.
func (rc *Reconciler) StateDependentRules(sigType string) []Rule {
	var out []Rule
	for _, cr := range rc.byType[sigType] {
		if referencesState(cr.rule.When) {
			out = append(out, cr.rule)
		}
	}
	return out
}

// referencesState reports whether a CEL `when` expression reads the `state.*`
// namespace. Conditions are flat boolean expressions over {signal, state}; a
// substring match on the `state.` selector is sufficient and avoids re-parsing.
func referencesState(when string) bool {
	return strings.Contains(when, "state.")
}

func actionToIntent(a Action, r Rule, sig core.Signal) core.RemediationIntent {
	// The remediation subject differs by action kind.
	target := sig.Subject.Alias
	switch core.ActionKind(a.Kind) {
	case core.ActionThrottleTenant:
		if sig.Subject.Tenant != "" {
			target = sig.Subject.Tenant
		}
	case core.ActionDisableDeployment:
		if sig.Subject.Deployment != "" {
			target = sig.Subject.Deployment
		}
	}
	ttl := time.Duration(0)
	if s, ok := a.Args["ttl"].(string); ok {
		if d, err := time.ParseDuration(s); err == nil {
			ttl = d
		}
	}
	id := core.DeterministicID(a.Kind, target, r.SHA, sig.CorrelationKey)
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
