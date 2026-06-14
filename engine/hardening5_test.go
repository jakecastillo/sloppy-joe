package engine

import (
	"context"
	"testing"
	"time"

	"github.com/sloppyjoe/sloppy/actuator"
	"github.com/sloppyjoe/sloppy/core"
	"github.com/sloppyjoe/sloppy/intent"
	"github.com/sloppyjoe/sloppy/ledger"
	"github.com/sloppyjoe/sloppy/metrics"
	"github.com/sloppyjoe/sloppy/rules"
	"github.com/sloppyjoe/sloppy/state"
)

// budgetRule throttles after a single firing per hour so the budget path is exercised.
const budgetRule = `
on: cost.budget_burn
when: signal.data.spend_1h_usd > 5.0
then: [ { route_override: { alias: gpt-4o, to: ollama/llama3 } } ]
with: { intent_budget: "1/h" }
`

// countActionsFailStore embeds a real store but fails the budget read (CountActions),
// simulating a store blip while intent_budget enforcement is in effect.
type countActionsFailStore struct {
	state.Store
}

func (countActionsFailStore) CountActions(context.Context, string, time.Time) (int, error) {
	return 0, errBoom
}

// recordActionFailStore embeds a real store but fails the budget write (RecordAction).
type recordActionFailStore struct {
	state.Store
}

func (recordActionFailStore) RecordAction(context.Context, string, time.Time) error { return errBoom }

func budgetEngine(t *testing.T, st state.Store, m *metrics.Registry, opts ...Option) (*Engine, *actuator.Fake) {
	t.Helper()
	rs, err := rules.ParseRules([]byte(budgetRule))
	if err != nil {
		t.Fatal(err)
	}
	rec, err := rules.NewReconciler(rs)
	if err != nil {
		t.Fatal(err)
	}
	reg := actuator.NewRegistry()
	f := &actuator.Fake{}
	reg.Register(f)
	signer, _ := intent.NewEd25519Signer()
	all := append([]Option{WithMetrics(m)}, opts...)
	return New(rec, reg, st, signer, all...), f
}

// (a) A failed budget read under FailClosed must DENY (throttle), not silently
// disable budget enforcement — and must surface a metric + audit.
func TestBudgetCheckFailClosedDenies(t *testing.T) {
	inner, _ := state.OpenSQLite(t.TempDir() + "/bc.db")
	defer inner.Close()
	st := countActionsFailStore{Store: inner}
	m := metrics.New()
	e, f := budgetEngine(t, st, m, WithFailMode(FailClosed))

	res, _ := e.Handle(context.Background(), burnSig())
	if f.Applied != 0 {
		t.Fatalf("fail-closed budget read must deny actuation, applied=%d", f.Applied)
	}
	if len(res) != 1 || res[0].Outcome != OutThrottled {
		t.Fatalf("expected OutThrottled, got %+v", res)
	}
	if m.Snapshot()["budget_check_failed"] != 1 {
		t.Fatalf("budget_check_failed metric must fire, got %d", m.Snapshot()["budget_check_failed"])
	}
	if !auditHas(t, inner, "intent.budget_check_failed") {
		t.Fatal("expected intent.budget_check_failed audit entry")
	}
}

// (a) A failed budget read under FailOpen proceeds (availability) but must still
// be observable via the metric + audit.
func TestBudgetCheckFailOpenProceedsButSurfaces(t *testing.T) {
	inner, _ := state.OpenSQLite(t.TempDir() + "/bo.db")
	defer inner.Close()
	st := countActionsFailStore{Store: inner}
	m := metrics.New()
	e, f := budgetEngine(t, st, m, WithFailMode(FailOpen))

	res, _ := e.Handle(context.Background(), burnSig())
	if countApplied(res) != 1 || f.Applied != 1 {
		t.Fatalf("fail-open budget read should still actuate, applied=%d fake=%d", countApplied(res), f.Applied)
	}
	if m.Snapshot()["budget_check_failed"] != 1 {
		t.Fatalf("budget_check_failed metric must fire even on fail-open, got %d", m.Snapshot()["budget_check_failed"])
	}
	if !auditHas(t, inner, "intent.budget_check_failed") {
		t.Fatal("expected intent.budget_check_failed audit entry on fail-open")
	}
}

// (a) A failed budget write (RecordAction) must not silently drop: the firing
// proceeds, but the lost budget accounting is surfaced via metric + audit.
func TestBudgetRecordFailureSurfaced(t *testing.T) {
	inner, _ := state.OpenSQLite(t.TempDir() + "/br.db")
	defer inner.Close()
	st := recordActionFailStore{Store: inner}
	m := metrics.New()
	e, f := budgetEngine(t, st, m, WithFailMode(FailOpen))

	res, _ := e.Handle(context.Background(), burnSig())
	if countApplied(res) != 1 || f.Applied != 1 {
		t.Fatalf("budget-record failure must not block the firing, applied=%d fake=%d", countApplied(res), f.Applied)
	}
	if m.Snapshot()["budget_record_failed"] != 1 {
		t.Fatalf("budget_record_failed metric must fire, got %d", m.Snapshot()["budget_record_failed"])
	}
	if !auditHas(t, inner, "intent.budget_record_failed") {
		t.Fatal("expected intent.budget_record_failed audit entry")
	}
}

// stateRule fires only off ledger-derived state.* — exactly the cost-runaway guard
// that must not silently vanish when the cost ledger's state read fails.
const stateRule = `
on: cost.budget_burn
when: state.spend_1h_usd > 10.0
then: [ { throttle_tenant: { tenant: acme } } ]
`

// spendFailLedgerStore makes the ledger's StateFor fail (SpendSince errors).
type spendFailLedgerStore struct {
	state.Store
}

func (spendFailLedgerStore) SpendSince(context.Context, string, time.Time) (float64, error) {
	return 0, errBoom
}

func stateRuleEngine(t *testing.T, m *metrics.Registry, fm FailMode) (*Engine, *actuator.Fake, state.Store) {
	t.Helper()
	rs, err := rules.ParseRules([]byte(stateRule))
	if err != nil {
		t.Fatal(err)
	}
	rec, err := rules.NewReconciler(rs)
	if err != nil {
		t.Fatal(err)
	}
	inner, _ := state.OpenSQLite(t.TempDir() + "/sf.db")
	st := spendFailLedgerStore{Store: inner}
	l := ledger.New(ledger.PriceBook{}, st)
	reg := actuator.NewRegistry()
	f := &actuator.Fake{}
	reg.Register(f)
	signer, _ := intent.NewEd25519Signer()
	now := time.Unix(1749340800, 0).UTC()
	e := New(rec, reg, st, signer, WithLedger(l), WithMetrics(m),
		WithFailMode(fm), WithClock(func() time.Time { return now }))
	return e, f, inner
}

// (b) Under FailClosed, a dropped StateFor read must NOT let a state.* cost-runaway
// rule silently non-match: it is inconclusive (OutFailed) + metric + audit.
func TestStateUnavailableFailClosedInconclusive(t *testing.T) {
	m := metrics.New()
	e, f, inner := stateRuleEngine(t, m, FailClosed)
	defer inner.Close()

	sig := core.Signal{
		Type: "cost.budget_burn", CorrelationKey: "acme:cost",
		Subject: core.Subject{Tenant: "acme", Alias: "gpt-4o"},
	}
	res, _ := e.Handle(context.Background(), sig)

	if m.Snapshot()["state_unavailable"] != 1 {
		t.Fatalf("state_unavailable metric must fire, got %d", m.Snapshot()["state_unavailable"])
	}
	if f.Applied != 0 {
		t.Fatalf("fail-closed must not actuate on inconclusive state, applied=%d", f.Applied)
	}
	if len(res) != 1 || res[0].Outcome != OutFailed {
		t.Fatalf("state-referencing rule should be inconclusive (OutFailed), got %+v", res)
	}
	if !auditHas(t, inner, "state.fail_closed") {
		t.Fatal("expected state.fail_closed audit entry")
	}
}

// (b) Under FailOpen, a dropped StateFor read is surfaced via the metric but
// remediation proceeds (availability over strictness); no inconclusive Result.
func TestStateUnavailableFailOpenSurfacedNotInconclusive(t *testing.T) {
	m := metrics.New()
	e, _, inner := stateRuleEngine(t, m, FailOpen)
	defer inner.Close()

	sig := core.Signal{
		Type: "cost.budget_burn", CorrelationKey: "acme:cost",
		Subject: core.Subject{Tenant: "acme", Alias: "gpt-4o"},
	}
	res, _ := e.Handle(context.Background(), sig)

	if m.Snapshot()["state_unavailable"] != 1 {
		t.Fatalf("state_unavailable metric must fire on fail-open too, got %d", m.Snapshot()["state_unavailable"])
	}
	for _, r := range res {
		if r.Outcome == OutFailed {
			t.Fatalf("fail-open must not produce inconclusive OutFailed, got %+v", res)
		}
	}
	if auditHas(t, inner, "state.fail_closed") {
		t.Fatal("fail-open must not emit state.fail_closed")
	}
}
