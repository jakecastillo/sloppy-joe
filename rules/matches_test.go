package rules

import (
	"testing"

	"github.com/sloppyjoe/sloppy/core"
)

func TestEvaluateMatchesCarriesRule(t *testing.T) {
	rs, _ := ParseRules([]byte(sampleRule))
	rec, _ := NewReconciler(rs)
	sig := core.Signal{Type: "cost.budget_burn", CorrelationKey: "acme:cost",
		Subject: core.Subject{Tenant: "acme", Alias: "gpt-4o"}, Data: map[string]any{"spend_1h_usd": 9.0}}
	ms := rec.EvaluateMatches(sig, nil)
	if len(ms) != 1 {
		t.Fatalf("want 1 match, got %d", len(ms))
	}
	if ms[0].Rule.For.String() != "5m0s" {
		t.Fatalf("match must carry rule For: %v", ms[0].Rule.For)
	}
	if len(ms[0].Intents) != 3 {
		t.Fatalf("want 3 intents in match, got %d", len(ms[0].Intents))
	}
}

func TestEvaluateMatchesUsesState(t *testing.T) {
	rs, _ := ParseRules([]byte(`
on: cost.budget_burn
when: state.spend_1h_usd > 5.0
then: [ { page: { slack: "#x" } } ]
`))
	rec, _ := NewReconciler(rs)
	sig := core.Signal{Type: "cost.budget_burn", Subject: core.Subject{Tenant: "acme"}}
	if got := rec.EvaluateMatches(sig, map[string]any{"spend_1h_usd": 9.0}); len(got) != 1 {
		t.Fatalf("state-driven rule should match, got %d", len(got))
	}
	if got := rec.EvaluateMatches(sig, map[string]any{"spend_1h_usd": 1.0}); len(got) != 0 {
		t.Fatalf("below threshold should not match, got %d", len(got))
	}
}
