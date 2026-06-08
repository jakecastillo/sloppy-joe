package replay

import (
	"testing"

	"github.com/sloppyjoe/sloppy/core"
	"github.com/sloppyjoe/sloppy/rules"
)

func TestReplayDeterministic(t *testing.T) {
	rs, _ := rules.ParseRules([]byte(`
on: cost.budget_burn
when: signal.data.spend_1h_usd > 5.0
then: [ { route_override: { alias: gpt-4o, to: ollama/llama3 } } ]
`))
	rec, _ := rules.NewReconciler(rs)
	sigs := []core.Signal{
		{ID: "s1", Type: "cost.budget_burn", Subject: core.Subject{Alias: "gpt-4o"}, Data: map[string]any{"spend_1h_usd": 9.0}},
		{ID: "s2", Type: "cost.budget_burn", Data: map[string]any{"spend_1h_usd": 1.0}},
		{ID: "s3", Type: "latency.regression"},
	}
	a := Run(rec, sigs)
	b := Run(rec, sigs)
	if len(a) != 3 {
		t.Fatalf("want 3 results, got %d", len(a))
	}
	if !a[0].Matched || len(a[0].Intents) != 1 || a[0].Intents[0].Kind != "route_override" {
		t.Fatalf("s1 should fire route_override: %+v", a[0])
	}
	if a[1].Matched || a[2].Matched {
		t.Fatalf("s2/s3 should not match: %+v %+v", a[1], a[2])
	}
	// Deterministic.
	if a[0].Intents[0] != b[0].Intents[0] {
		t.Fatal("replay must be deterministic")
	}
}
