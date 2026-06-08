package engine

import (
	"context"
	"testing"

	"github.com/sloppyjoe/sloppy/actuator"
	"github.com/sloppyjoe/sloppy/core"
	"github.com/sloppyjoe/sloppy/intent"
	"github.com/sloppyjoe/sloppy/rules"
	"github.com/sloppyjoe/sloppy/state"
)

const rule = `
on: cost.budget_burn
when: signal.data.spend_1h_usd > 5.0
then:
  - route_override: { alias: gpt-4o, to: ollama/llama3, ttl: 30m }
with: { dry_run: false }
`

func build(t *testing.T, ruleYAML string) (*Engine, *actuator.Fake, state.Store, intent.Signer) {
	t.Helper()
	rs, err := rules.ParseRules([]byte(ruleYAML))
	if err != nil {
		t.Fatal(err)
	}
	rec, err := rules.NewReconciler(rs)
	if err != nil {
		t.Fatal(err)
	}
	st, err := state.OpenSQLite(t.TempDir() + "/e.db")
	if err != nil {
		t.Fatal(err)
	}
	reg := actuator.NewRegistry()
	f := &actuator.Fake{}
	reg.Register(f)
	s, _ := intent.NewEd25519Signer()
	return New(rec, reg, st, s), f, st, s
}

func countApplied(rs []Result) int {
	n := 0
	for _, r := range rs {
		if r.Outcome == OutApplied {
			n++
		}
	}
	return n
}

func TestEngineClosesLoopSignsAndIsIdempotent(t *testing.T) {
	e, f, st, signer := build(t, rule)
	defer st.Close()
	sig := core.Signal{Type: "cost.budget_burn", CorrelationKey: "acme:cost",
		Subject: core.Subject{Alias: "gpt-4o"}, Data: map[string]any{"spend_1h_usd": 9.0}}

	res, err := e.Handle(context.Background(), sig)
	if err != nil || countApplied(res) != 1 {
		t.Fatalf("first handle applied=%d err=%v", countApplied(res), err)
	}
	got := res[0]
	if got.Intent.Signature == "" || !signer.Verify(got.Intent.CanonicalBytes(), got.Intent.Signature) {
		t.Fatal("applied intent must carry a verifiable signature")
	}

	res2, _ := e.Handle(context.Background(), sig)
	if countApplied(res2) != 0 || res2[0].Outcome != OutSkipped {
		t.Fatalf("replay should skip, got %+v", res2)
	}
	if f.Applied != 1 {
		t.Fatalf("actuator should fire exactly once, fired %d", f.Applied)
	}
	if !st.VerifyAudit() {
		t.Fatal("audit chain invalid")
	}
}

func TestEngineDryRunDoesNotActuate(t *testing.T) {
	e, f, st, _ := build(t, `
on: cost.budget_burn
when: signal.data.spend_1h_usd > 5.0
then: [ { route_override: { alias: gpt-4o, to: ollama/llama3 } } ]
with: { dry_run: true }
`)
	defer st.Close()
	res, _ := e.Handle(context.Background(), core.Signal{Type: "cost.budget_burn",
		Subject: core.Subject{Alias: "gpt-4o"}, Data: map[string]any{"spend_1h_usd": 9.0}})
	if f.Applied != 0 {
		t.Fatalf("dry_run must not actuate, fired %d", f.Applied)
	}
	if len(res) != 1 || res[0].Outcome != OutDryRun {
		t.Fatalf("expected dry_run outcome, got %+v", res)
	}
}
