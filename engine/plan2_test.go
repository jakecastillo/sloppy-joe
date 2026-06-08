package engine

import (
	"context"
	"testing"
	"time"

	"github.com/sloppyjoe/sloppy/actuator"
	"github.com/sloppyjoe/sloppy/core"
	"github.com/sloppyjoe/sloppy/intent"
	"github.com/sloppyjoe/sloppy/ledger"
	"github.com/sloppyjoe/sloppy/rules"
	"github.com/sloppyjoe/sloppy/state"
)

func mustEngine(t *testing.T, ruleYAML string, opts ...Option) (*Engine, *actuator.Fake, state.Store) {
	t.Helper()
	rs, err := rules.ParseRules([]byte(ruleYAML))
	if err != nil {
		t.Fatal(err)
	}
	rec, err := rules.NewReconciler(rs)
	if err != nil {
		t.Fatal(err)
	}
	st, err := state.OpenSQLite(t.TempDir() + "/p2.db")
	if err != nil {
		t.Fatal(err)
	}
	reg := actuator.NewRegistry()
	f := &actuator.Fake{}
	reg.Register(f)
	signer, _ := intent.NewEd25519Signer()
	return New(rec, reg, st, signer, opts...), f, st
}

func TestForWindowGating(t *testing.T) {
	base := time.Unix(1749340800, 0).UTC()
	now := base
	clock := func() time.Time { return now }
	e, f, st := mustEngine(t, `
on: cost.budget_burn
when: signal.data.spend_1h_usd > 5.0
for: 1m
then: [ { route_override: { alias: gpt-4o, to: ollama/llama3 } } ]
`, WithClock(clock))
	defer st.Close()

	sig := core.Signal{Type: "cost.budget_burn", CorrelationKey: "acme:cost",
		Subject: core.Subject{Alias: "gpt-4o"}, Data: map[string]any{"spend_1h_usd": 9.0}}

	// First sighting → pending, not applied.
	res, _ := e.Handle(context.Background(), sig)
	if len(res) != 1 || res[0].Outcome != OutPending {
		t.Fatalf("first sighting should be pending, got %+v", res)
	}
	if f.Applied != 0 {
		t.Fatal("must not actuate before for-window elapses")
	}

	// Still within window.
	now = base.Add(30 * time.Second)
	res, _ = e.Handle(context.Background(), sig)
	if res[0].Outcome != OutPending {
		t.Fatalf("still within window should be pending, got %+v", res)
	}

	// After window → applied.
	now = base.Add(90 * time.Second)
	res, _ = e.Handle(context.Background(), sig)
	if countApplied(res) != 1 || f.Applied != 1 {
		t.Fatalf("after for-window should apply once, applied=%d fake=%d", countApplied(res), f.Applied)
	}
}

func TestLedgerDrivesState(t *testing.T) {
	now := time.Unix(1749340800, 0).UTC()
	l := ledger.New(ledger.PriceBook{"gpt-4o": {InputPer1K: 5, OutputPer1K: 15}})
	l.Record("acme", "gpt-4o", 1000, 1000, now) // $20 in the last hour

	e, f, st := mustEngine(t, `
on: cost.budget_burn
when: state.spend_1h_usd > 10.0
then: [ { route_override: { alias: gpt-4o, to: ollama/llama3 } } ]
`, WithLedger(l), WithClock(func() time.Time { return now }))
	defer st.Close()

	sig := core.Signal{Type: "cost.budget_burn", CorrelationKey: "acme:cost",
		Subject: core.Subject{Tenant: "acme", Alias: "gpt-4o"}}
	res, _ := e.Handle(context.Background(), sig)
	if countApplied(res) != 1 || f.Applied != 1 {
		t.Fatalf("ledger-driven rule should fire, applied=%d fake=%d", countApplied(res), f.Applied)
	}
}

func TestTTLRevertScheduledAndProcessed(t *testing.T) {
	base := time.Unix(1749340800, 0).UTC()
	now := base
	e, f, st := mustEngine(t, `
on: cost.budget_burn
when: signal.data.spend_1h_usd > 5.0
then: [ { route_override: { alias: gpt-4o, to: ollama/llama3, ttl: 30m } } ]
`, WithClock(func() time.Time { return now }))
	defer st.Close()

	sig := core.Signal{Type: "cost.budget_burn", CorrelationKey: "acme:cost",
		Subject: core.Subject{Alias: "gpt-4o"}, Data: map[string]any{"spend_1h_usd": 9.0}}
	if res, _ := e.Handle(context.Background(), sig); countApplied(res) != 1 {
		t.Fatal("expected apply")
	}
	// Not due yet.
	if n, _ := e.ProcessDueReverts(context.Background(), base.Add(10*time.Minute)); n != 0 {
		t.Fatalf("nothing should revert at +10m, reverted %d", n)
	}
	// Due after ttl.
	n, _ := e.ProcessDueReverts(context.Background(), base.Add(31*time.Minute))
	if n != 1 || f.Reverted != 1 {
		t.Fatalf("expected 1 revert after ttl, got n=%d fake=%d", n, f.Reverted)
	}
	// Idempotent: already reverted.
	if n, _ := e.ProcessDueReverts(context.Background(), base.Add(2*time.Hour)); n != 0 {
		t.Fatalf("revert must be one-shot, got %d", n)
	}
	if !st.VerifyAudit() {
		t.Fatal("audit chain invalid after revert")
	}
}
