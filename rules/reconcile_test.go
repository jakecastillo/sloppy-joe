package rules

import (
	"testing"

	"github.com/sloppyjoe/sloppy/core"
)

func TestReconcileProducesIntents(t *testing.T) {
	rs, err := ParseRules([]byte(sampleRule))
	if err != nil {
		t.Fatal(err)
	}
	rec, err := NewReconciler(rs)
	if err != nil {
		t.Fatalf("compile rules: %v", err)
	}
	sig := core.Signal{
		Type:           "cost.budget_burn",
		CorrelationKey: "acme:cost",
		Subject:        core.Subject{Tenant: "acme", Alias: "gpt-4o"},
		Data:           map[string]any{"spend_1h_usd": 7.5},
	}
	intents := rec.Reconcile(sig, nil)
	if len(intents) != 3 {
		t.Fatalf("want 3 intents, got %d", len(intents))
	}
	for _, in := range intents {
		if in.RuleSHA == "" || in.ID == "" {
			t.Fatalf("intent missing provenance/id: %+v", in)
		}
	}
	// Wrong type → no intents.
	if got := rec.Reconcile(core.Signal{Type: "other"}, nil); len(got) != 0 {
		t.Fatalf("non-matching type should yield 0, got %d", len(got))
	}
	// Below threshold → no intents.
	low := sig
	low.Data = map[string]any{"spend_1h_usd": 1.0}
	if got := rec.Reconcile(low, nil); len(got) != 0 {
		t.Fatalf("below-threshold should yield 0, got %d", len(got))
	}
}

// TestReconcileIndexedBySignalType pins that indexing compiled rules by `on:`
// type preserves the prior flat-scan results: a signal only fires the rules of
// its own type, rules of other types are never evaluated, and a signal type with
// no rules yields nothing. Multiple rules share one type to also pin
// ordering-within-type (intents fire in input/parse order).
func TestReconcileIndexedBySignalType(t *testing.T) {
	// ParseRules is one-document-per-call, so parse each rule separately and
	// concatenate to preserve a stable input order. Two cost.budget_burn rules
	// (in this order) plus one latency.spike rule.
	ruleDocs := []string{
		`
on: cost.budget_burn
when: signal.data.spend_1h_usd > 5.0
then: [ { route_override: { alias: gpt-4o, to: ollama/llama3 } } ]
`,
		`
on: cost.budget_burn
when: signal.data.spend_1h_usd > 5.0
then: [ { page: { slack: "#oncall" } } ]
`,
		`
on: latency.spike
when: signal.data.p99_ms > 1000
then: [ { throttle_tenant: {} } ]
`,
	}
	var rs []Rule
	for i, doc := range ruleDocs {
		parsed, err := ParseRules([]byte(doc))
		if err != nil {
			t.Fatalf("parse rule %d: %v", i, err)
		}
		rs = append(rs, parsed...)
	}
	if len(rs) != 3 {
		t.Fatalf("want 3 parsed rules, got %d", len(rs))
	}
	rec, err := NewReconciler(rs)
	if err != nil {
		t.Fatalf("compile rules: %v", err)
	}

	// cost.budget_burn fires both same-type rules, in parse order, and never the
	// latency.spike rule.
	cost := core.Signal{
		Type:           "cost.budget_burn",
		CorrelationKey: "acme:cost",
		Subject:        core.Subject{Tenant: "acme", Alias: "gpt-4o"},
		Data:           map[string]any{"spend_1h_usd": 9.0, "p99_ms": 5000.0},
	}
	costMatches := rec.EvaluateMatches(cost, nil)
	if len(costMatches) != 2 {
		t.Fatalf("cost signal must fire exactly its 2 same-type rules, got %d", len(costMatches))
	}
	for _, m := range costMatches {
		if m.Rule.On != "cost.budget_burn" {
			t.Fatalf("cost signal fired a rule of wrong type %q", m.Rule.On)
		}
	}
	// Ordering-within-type: route_override before page, matching parse order.
	if k := costMatches[0].Intents[0].Kind; k != core.ActionRouteOverride {
		t.Fatalf("first fired intent must preserve parse order (route_override), got %q", k)
	}
	if k := costMatches[1].Intents[0].Kind; k != core.ActionPage {
		t.Fatalf("second fired intent must preserve parse order (page), got %q", k)
	}

	// latency.spike fires only its own (different-type) rule.
	lat := core.Signal{
		Type:           "latency.spike",
		CorrelationKey: "acme:lat",
		Subject:        core.Subject{Tenant: "acme"},
		Data:           map[string]any{"p99_ms": 5000.0, "spend_1h_usd": 9.0},
	}
	latMatches := rec.EvaluateMatches(lat, nil)
	if len(latMatches) != 1 {
		t.Fatalf("latency signal must fire exactly its 1 same-type rule, got %d", len(latMatches))
	}
	if latMatches[0].Rule.On != "latency.spike" {
		t.Fatalf("latency signal fired a rule of wrong type %q", latMatches[0].Rule.On)
	}

	// A signal type with no rules in the index yields no matches.
	none := core.Signal{Type: "unknown.type", Data: map[string]any{"spend_1h_usd": 9.0}}
	if got := rec.EvaluateMatches(none, nil); len(got) != 0 {
		t.Fatalf("a signal type with no indexed rules must yield 0 matches, got %d", len(got))
	}
	if got := rec.StateDependentRules("unknown.type"); len(got) != 0 {
		t.Fatalf("StateDependentRules for an unindexed type must be empty, got %d", len(got))
	}
	if got := rec.Cleared(none, nil); len(got) != 0 {
		t.Fatalf("Cleared for an unindexed type must be empty, got %d", len(got))
	}
}

func TestReconcileDeterministicIntentIDs(t *testing.T) {
	rs, _ := ParseRules([]byte(sampleRule))
	rec, _ := NewReconciler(rs)
	sig := core.Signal{
		Type: "cost.budget_burn", CorrelationKey: "acme:cost",
		Subject: core.Subject{Tenant: "acme", Alias: "gpt-4o"}, Data: map[string]any{"spend_1h_usd": 9.0},
	}
	a := rec.Reconcile(sig, nil)
	b := rec.Reconcile(sig, nil)
	if a[0].ID != b[0].ID {
		t.Fatal("intent IDs must be deterministic for idempotency")
	}
}
