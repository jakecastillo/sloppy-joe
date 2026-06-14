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
