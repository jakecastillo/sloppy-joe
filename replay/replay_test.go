package replay

import (
	"reflect"
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

// TestReplayMultiRuleMultiActionGolden pins Results and Fired values plus
// ordering for a fixture where a single signal fires several rules, each with
// several actions. Presizing the inner Intents slice must not perturb the
// output: the result is compared deep-equal against a recorded golden run, and
// against a re-run, guaranteeing byte-identical values and ordering.
func TestReplayMultiRuleMultiActionGolden(t *testing.T) {
	// Two rules on the same signal type, each emitting multiple actions, so a
	// single matching signal produces several intents per match and exercises
	// the presized append target across match boundaries.
	rs1, err := rules.ParseRules([]byte(`
on: cost.budget_burn
when: signal.data.spend_1h_usd > 5.0
then:
  - { route_override: { alias: gpt-4o, to: ollama/llama3 } }
  - { throttle_tenant: { ttl: 5m } }
  - { open_issue: { title: budget burn } }
`))
	if err != nil {
		t.Fatalf("parse rule 1: %v", err)
	}
	rs2, err := rules.ParseRules([]byte(`
on: cost.budget_burn
when: signal.data.spend_1h_usd > 5.0
then:
  - { page: { who: oncall } }
  - { disable_deployment: {} }
`))
	if err != nil {
		t.Fatalf("parse rule 2: %v", err)
	}
	sha1, sha2 := rs1[0].SHA, rs2[0].SHA

	rec, err := rules.NewReconciler(append(append([]rules.Rule{}, rs1...), rs2...))
	if err != nil {
		t.Fatalf("reconciler: %v", err)
	}
	sigs := []core.Signal{
		{ID: "hot", Type: "cost.budget_burn", Subject: core.Subject{Alias: "gpt-4o", Tenant: "acme", Deployment: "web-1"}, Data: map[string]any{"spend_1h_usd": 12.0}},
		{ID: "cold", Type: "cost.budget_burn", Data: map[string]any{"spend_1h_usd": 1.0}},
		{ID: "other", Type: "latency.regression"},
	}

	// Golden: rule 1's three actions in order, then rule 2's two actions in
	// order. throttle_tenant/disable_deployment retarget to tenant/deployment.
	want := []Result{
		{
			SignalID: "hot",
			Matched:  true,
			Intents: []Fired{
				{Rule: sha1, Kind: "route_override", Target: "gpt-4o"},
				{Rule: sha1, Kind: "throttle_tenant", Target: "acme"},
				{Rule: sha1, Kind: "open_issue", Target: "gpt-4o"},
				{Rule: sha2, Kind: "page", Target: "gpt-4o"},
				{Rule: sha2, Kind: "disable_deployment", Target: "web-1"},
			},
		},
		{SignalID: "cold", Matched: false, Intents: nil},
		{SignalID: "other", Matched: false, Intents: nil},
	}

	got := Run(rec, sigs)
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("replay output diverged from golden\n got: %+v\nwant: %+v", got, want)
	}

	// Re-run must be byte-identical to the first run (deterministic, and the
	// presize/reuse must not leak state between runs).
	if rerun := Run(rec, sigs); !reflect.DeepEqual(rerun, want) {
		t.Fatalf("replay not byte-identical on re-run\n got: %+v\nwant: %+v", rerun, want)
	}

	// The matching signal's Intents must be presized to the exact total so the
	// backing array is not regrown by appends.
	if c := cap(got[0].Intents); c != len(want[0].Intents) {
		t.Fatalf("Intents not presized to exact total: cap=%d want=%d", c, len(want[0].Intents))
	}
}
