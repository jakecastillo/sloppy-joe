package rules

import (
	"testing"

	"github.com/sloppyjoe/sloppy/core"
)

// TestEvalRuntimeErrorOnMissingKey pins that a CEL *runtime* eval error (distinct
// from a compile error) is surfaced by Condition.Eval, not swallowed. Comparing a
// missing map key against a number is a runtime "no such key" error in cel-go.
func TestEvalRuntimeErrorOnMissingKey(t *testing.T) {
	c, err := CompileCondition(`signal.data.nope > 5.0`)
	if err != nil {
		t.Fatalf("compile should succeed (the key is only missing at eval time): %v", err)
	}
	ok, err := c.Eval(core.Signal{Data: map[string]any{}}, nil)
	if err == nil {
		t.Fatal("expected a runtime eval error for a missing data key")
	}
	if ok {
		t.Fatal("a runtime error must report ok=false, never a spurious true")
	}
}

// TestEvalNonBoolReturn pins the sentinel error when a condition compiles but does
// not evaluate to a bool (e.g. an arithmetic or string expression). This is the
// guard that keeps a non-bool from being coerced into a match.
func TestEvalNonBoolReturn(t *testing.T) {
	for _, expr := range []string{`1 + 1`, `signal.tenant`} {
		c, err := CompileCondition(expr)
		if err != nil {
			t.Fatalf("compile %q: %v", expr, err)
		}
		ok, err := c.Eval(core.Signal{Subject: core.Subject{Tenant: "acme"}}, nil)
		if err == nil {
			t.Fatalf("expr %q: expected non-bool error, got ok=%v", expr, ok)
		}
		if got := err.Error(); got != "rules: condition did not return bool" {
			t.Fatalf("expr %q: unexpected error %q", expr, got)
		}
		if ok {
			t.Fatalf("expr %q: non-bool must report ok=false", expr)
		}
	}
}

// TestEvaluateMatchesFoldsRuntimeErrorIntoNonMatch pins the CURRENT intended
// reconcile behavior: a rule whose `when` raises a *runtime* eval error (here a
// missing data key) is treated as a non-match (skipped), exactly like a clean
// false — it must NOT fire intents and must NOT panic/propagate.
//
// NOTE (product decision pinned): EvaluateMatches deliberately folds eval errors
// into non-match (`if err != nil || !ok { continue }`). The engine separately
// guards *state.* rules against an unavailable state store via StateDependentRules
// (fail-closed inconclusive surfacing); a signal-only rule that errors at eval
// stays a silent non-match here. If that ever needs to become "inconclusive" for
// signal-derived guards too, this test must change deliberately.
func TestEvaluateMatchesFoldsRuntimeErrorIntoNonMatch(t *testing.T) {
	rs, err := ParseRules([]byte(`
on: cost.budget_burn
when: signal.data.spend_1h_usd > 5.0
then: [ { page: { slack: "#x" } } ]
`))
	if err != nil {
		t.Fatal(err)
	}
	rec, err := NewReconciler(rs)
	if err != nil {
		t.Fatal(err)
	}
	// The data map is missing spend_1h_usd entirely -> the `>` raises a runtime
	// "no such key" error inside Eval.
	sig := core.Signal{Type: "cost.budget_burn", Subject: core.Subject{Tenant: "acme"}, Data: map[string]any{}}
	if ms := rec.EvaluateMatches(sig, nil); len(ms) != 0 {
		t.Fatalf("a runtime eval error must fold into non-match (0 matches), got %d", len(ms))
	}
	if ins := rec.Reconcile(sig, nil); len(ins) != 0 {
		t.Fatalf("a runtime eval error must yield 0 intents, got %d", len(ins))
	}
}

// TestEvaluateMatchesFoldsNonBoolIntoNonMatch pins that a non-bool `when` (compiles
// fine, evaluates to a non-bool) is also folded into non-match rather than firing.
func TestEvaluateMatchesFoldsNonBoolIntoNonMatch(t *testing.T) {
	rs, err := ParseRules([]byte(`
on: cost.budget_burn
when: signal.tenant
then: [ { page: { slack: "#x" } } ]
`))
	if err != nil {
		t.Fatal(err)
	}
	rec, err := NewReconciler(rs)
	if err != nil {
		t.Fatal(err)
	}
	sig := core.Signal{Type: "cost.budget_burn", Subject: core.Subject{Tenant: "acme"}}
	if ms := rec.EvaluateMatches(sig, nil); len(ms) != 0 {
		t.Fatalf("a non-bool `when` must fold into non-match (0 matches), got %d", len(ms))
	}
}

// TestClearedReturnsOnClearRulesWhenConditionFalse pins Cleared(): for a
// `rollback: on_clear` rule of the signal's type, Cleared returns the rule exactly
// when its condition is now FALSE (the incident has cleared), and returns nothing
// while the condition still holds.
func TestClearedReturnsOnClearRulesWhenConditionFalse(t *testing.T) {
	rs, err := ParseRules([]byte(`
on: cost.budget_burn
when: signal.data.spend_1h_usd > 5.0
then: [ { route_override: { alias: gpt-4o, to: ollama/llama3 } } ]
with: { rollback: on_clear }
`))
	if err != nil {
		t.Fatal(err)
	}
	rec, err := NewReconciler(rs)
	if err != nil {
		t.Fatal(err)
	}
	base := core.Signal{Type: "cost.budget_burn", Subject: core.Subject{Tenant: "acme", Alias: "gpt-4o"}}

	// Condition still TRUE (spend high) -> nothing cleared.
	hot := base
	hot.Data = map[string]any{"spend_1h_usd": 9.0}
	if cleared := rec.Cleared(hot, nil); len(cleared) != 0 {
		t.Fatalf("while the condition holds, Cleared must return nothing, got %d", len(cleared))
	}

	// Condition now FALSE (spend dropped) -> the on_clear rule is cleared.
	cool := base
	cool.Data = map[string]any{"spend_1h_usd": 1.0}
	cleared := rec.Cleared(cool, nil)
	if len(cleared) != 1 {
		t.Fatalf("a cleared on_clear rule must be returned exactly once, got %d", len(cleared))
	}
	if cleared[0].With.Rollback != "on_clear" {
		t.Fatalf("Cleared must carry the on_clear rule, got %+v", cleared[0].With)
	}
}

// TestClearedIgnoresNonOnClearAndWrongType pins the two exclusions in Cleared:
// a rule without `rollback: on_clear` is never returned (even when its condition is
// false), and a rule of a different signal type is skipped.
func TestClearedIgnoresNonOnClearAndWrongType(t *testing.T) {
	// No rollback policy: must never appear in Cleared regardless of condition.
	noRollback, _ := ParseRules([]byte(`
on: cost.budget_burn
when: signal.data.spend_1h_usd > 5.0
then: [ { page: { slack: "#x" } } ]
`))
	rec, _ := NewReconciler(noRollback)
	cool := core.Signal{Type: "cost.budget_burn", Data: map[string]any{"spend_1h_usd": 1.0}}
	if cleared := rec.Cleared(cool, nil); len(cleared) != 0 {
		t.Fatalf("a rule without rollback:on_clear must never be cleared, got %d", len(cleared))
	}

	// on_clear rule but the signal is a different type: skipped.
	onClear, _ := ParseRules([]byte(`
on: cost.budget_burn
when: signal.data.spend_1h_usd > 5.0
then: [ { route_override: { alias: gpt-4o, to: ollama/llama3 } } ]
with: { rollback: on_clear }
`))
	rec2, _ := NewReconciler(onClear)
	other := core.Signal{Type: "latency.spike", Data: map[string]any{"spend_1h_usd": 1.0}}
	if cleared := rec2.Cleared(other, nil); len(cleared) != 0 {
		t.Fatalf("an on_clear rule of a different type must be skipped, got %d", len(cleared))
	}
}

// TestActionToIntentTargetSwitching pins the per-kind remediation subject mapping
// in actionToIntent: throttle_tenant targets the tenant, disable_deployment targets
// the deployment, and every other kind (route_override/page/open_issue) targets the
// alias. It also pins the deterministic ID and rule-SHA provenance, and the fall-back
// to alias when the kind-specific subject field is empty.
func TestActionToIntentTargetSwitching(t *testing.T) {
	r := Rule{SHA: "sha-abc", With: With{DryRun: true}}
	sig := core.Signal{
		CorrelationKey: "acme:cost",
		Subject:        core.Subject{Tenant: "acme", Deployment: "dep-1", Alias: "gpt-4o"},
	}

	cases := []struct {
		kind       core.ActionKind
		wantTarget string
	}{
		{core.ActionThrottleTenant, "acme"},     // -> tenant
		{core.ActionDisableDeployment, "dep-1"}, // -> deployment
		{core.ActionRouteOverride, "gpt-4o"},    // -> alias
		{core.ActionPage, "gpt-4o"},             // -> alias
		{core.ActionOpenIssue, "gpt-4o"},        // -> alias
	}
	for _, c := range cases {
		in := actionToIntent(Action{Kind: string(c.kind)}, r, sig)
		if in.Target != c.wantTarget {
			t.Fatalf("kind %s: want target %q, got %q", c.kind, c.wantTarget, in.Target)
		}
		if in.Kind != c.kind {
			t.Fatalf("kind %s: intent kind mismatch %q", c.kind, in.Kind)
		}
		if in.RuleSHA != "sha-abc" {
			t.Fatalf("kind %s: rule SHA provenance dropped: %q", c.kind, in.RuleSHA)
		}
		if !in.DryRun {
			t.Fatalf("kind %s: With.DryRun must propagate onto the intent", c.kind)
		}
		want := core.DeterministicID(string(c.kind), c.wantTarget, r.SHA, sig.CorrelationKey)
		if in.ID != want {
			t.Fatalf("kind %s: deterministic ID mismatch: want %q got %q", c.kind, want, in.ID)
		}
	}
}

// TestActionToIntentFallsBackToAliasWhenSubjectEmpty pins that throttle_tenant and
// disable_deployment fall back to the alias target when their kind-specific subject
// field is empty (the switch only overrides when the field is non-empty).
func TestActionToIntentFallsBackToAliasWhenSubjectEmpty(t *testing.T) {
	r := Rule{SHA: "sha-xyz"}
	// No Tenant / no Deployment set, only Alias.
	sig := core.Signal{Subject: core.Subject{Alias: "gpt-4o"}}

	thr := actionToIntent(Action{Kind: string(core.ActionThrottleTenant)}, r, sig)
	if thr.Target != "gpt-4o" {
		t.Fatalf("throttle_tenant with empty tenant must fall back to alias, got %q", thr.Target)
	}
	dis := actionToIntent(Action{Kind: string(core.ActionDisableDeployment)}, r, sig)
	if dis.Target != "gpt-4o" {
		t.Fatalf("disable_deployment with empty deployment must fall back to alias, got %q", dis.Target)
	}
}

// TestActionToIntentParsesTTL pins that a well-formed `ttl` arg is parsed onto the
// intent and a malformed/absent one leaves TTL zero (no actuation window).
func TestActionToIntentParsesTTL(t *testing.T) {
	r := Rule{SHA: "sha-ttl"}
	sig := core.Signal{Subject: core.Subject{Alias: "gpt-4o"}}

	good := actionToIntent(Action{Kind: string(core.ActionRouteOverride), Args: map[string]any{"ttl": "30m"}}, r, sig)
	if good.TTL.String() != "30m0s" {
		t.Fatalf("ttl 30m must parse onto the intent, got %v", good.TTL)
	}
	bad := actionToIntent(Action{Kind: string(core.ActionRouteOverride), Args: map[string]any{"ttl": "not-a-duration"}}, r, sig)
	if bad.TTL != 0 {
		t.Fatalf("malformed ttl must leave TTL zero, got %v", bad.TTL)
	}
	none := actionToIntent(Action{Kind: string(core.ActionRouteOverride)}, r, sig)
	if none.TTL != 0 {
		t.Fatalf("absent ttl must leave TTL zero, got %v", none.TTL)
	}
}
