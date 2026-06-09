package engine

import (
	"context"
	"testing"
	"time"

	"github.com/sloppyjoe/sloppy/actuator"
	"github.com/sloppyjoe/sloppy/core"
	"github.com/sloppyjoe/sloppy/intent"
	"github.com/sloppyjoe/sloppy/rules"
	"github.com/sloppyjoe/sloppy/state"
)

func govEngine(t *testing.T, ruleYAML string, now time.Time) (*Engine, *actuator.Fake, state.Store) {
	t.Helper()
	rs, err := rules.ParseRules([]byte(ruleYAML))
	if err != nil {
		t.Fatal(err)
	}
	rec, err := rules.NewReconciler(rs)
	if err != nil {
		t.Fatal(err)
	}
	st, err := state.OpenSQLite(t.TempDir() + "/gov.db")
	if err != nil {
		t.Fatal(err)
	}
	reg := actuator.NewRegistry()
	f := &actuator.Fake{}
	reg.Register(f)
	signer, _ := intent.NewEd25519Signer()
	return New(rec, reg, st, signer, WithClock(func() time.Time { return now })), f, st
}

func TestIntentBudgetThrottles(t *testing.T) {
	now := time.Unix(1749340800, 0).UTC()
	e, f, st := govEngine(t, `
on: cost.budget_burn
when: signal.data.spend_1h_usd > 5.0
then: [ { route_override: { alias: gpt-4o, to: ollama/llama3 } } ]
with: { intent_budget: "2/h" }
`, now)
	defer st.Close()

	mk := func(corr string) core.Signal {
		return core.Signal{Type: "cost.budget_burn", CorrelationKey: corr,
			Subject: core.Subject{Alias: "gpt-4o"}, Data: map[string]any{"spend_1h_usd": 9.0}}
	}
	r1, _ := e.Handle(context.Background(), mk("a1"))
	r2, _ := e.Handle(context.Background(), mk("a2"))
	r3, _ := e.Handle(context.Background(), mk("a3"))

	if countApplied(r1) != 1 || countApplied(r2) != 1 {
		t.Fatalf("first two incidents should apply (%d, %d)", countApplied(r1), countApplied(r2))
	}
	if f.Applied != 2 {
		t.Fatalf("actuator should fire exactly twice under a 2/h budget, got %d", f.Applied)
	}
	throttled := false
	for _, r := range r3 {
		if r.Outcome == OutThrottled {
			throttled = true
		}
	}
	if !throttled || countApplied(r3) != 0 {
		t.Fatalf("third incident should be throttled, got %+v", r3)
	}
}

func TestRollbackOnClear(t *testing.T) {
	now := time.Unix(1749340800, 0).UTC()
	e, f, st := govEngine(t, `
on: cost.budget_burn
when: signal.data.spend_1h_usd > 5.0
then: [ { route_override: { alias: gpt-4o, to: ollama/llama3 } } ]
with: { rollback: on_clear }
`, now)
	defer st.Close()

	hi := core.Signal{Type: "cost.budget_burn", CorrelationKey: "acme:cost",
		Subject: core.Subject{Alias: "gpt-4o"}, Data: map[string]any{"spend_1h_usd": 9.0}}
	lo := hi
	lo.Data = map[string]any{"spend_1h_usd": 1.0}

	if r, _ := e.Handle(context.Background(), hi); countApplied(r) != 1 {
		t.Fatal("high-spend signal should apply")
	}
	if f.Reverted != 0 {
		t.Fatalf("no revert before the condition clears, got %d", f.Reverted)
	}
	// Condition clears → outstanding intent reverted.
	e.Handle(context.Background(), lo)
	if f.Reverted != 1 {
		t.Fatalf("clearing the condition should revert the outstanding intent, got %d", f.Reverted)
	}
	// Clearing again is a no-op (nothing outstanding).
	e.Handle(context.Background(), lo)
	if f.Reverted != 1 {
		t.Fatalf("second clear should not revert again, got %d", f.Reverted)
	}
	if !st.VerifyAudit() {
		t.Fatal("audit chain invalid")
	}
}
