package engine

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/sloppyjoe/sloppy/actuator"
	"github.com/sloppyjoe/sloppy/core"
	"github.com/sloppyjoe/sloppy/intent"
	"github.com/sloppyjoe/sloppy/metrics"
	"github.com/sloppyjoe/sloppy/rules"
	"github.com/sloppyjoe/sloppy/state"
)

var errBoom = errors.New("store unavailable")

// errStore implements state.Store but fails reads/writes (simulates outage).
type errStore struct{}

func (errStore) IsIntentApplied(context.Context, string) (bool, error) { return false, errBoom }
func (errStore) MarkIntentApplied(context.Context, string) error       { return errBoom }
func (errStore) AppendAudit(context.Context, string, string) (state.AuditEntry, error) {
	return state.AuditEntry{}, errBoom
}
func (errStore) Audit(context.Context) ([]state.AuditEntry, error) { return nil, errBoom }
func (errStore) VerifyAudit(context.Context) bool                  { return false }
func (errStore) ScheduleRevert(context.Context, state.PendingRevert) error {
	return errBoom
}
func (errStore) DueReverts(context.Context, time.Time) ([]state.PendingRevert, error) {
	return nil, errBoom
}
func (errStore) MarkReverted(context.Context, string) error            { return errBoom }
func (errStore) RecordAction(context.Context, string, time.Time) error { return errBoom }
func (errStore) CountActions(context.Context, string, time.Time) (int, error) {
	return 0, errBoom
}
func (errStore) RecordOutstanding(context.Context, string, state.PendingRevert) error {
	return errBoom
}
func (errStore) Outstanding(context.Context, string) ([]state.PendingRevert, error) {
	return nil, errBoom
}
func (errStore) ClearOutstanding(context.Context, string) error { return errBoom }
func (errStore) Close() error                                   { return nil }

func engineWithStore(st state.Store, opts ...Option) (*Engine, *actuator.Fake) {
	rs, _ := rules.ParseRules([]byte(rule))
	rec, _ := rules.NewReconciler(rs)
	reg := actuator.NewRegistry()
	f := &actuator.Fake{}
	reg.Register(f)
	signer, _ := intent.NewEd25519Signer()
	return New(rec, reg, st, signer, opts...), f
}

func burnSignal() core.Signal {
	return core.Signal{Type: "cost.budget_burn", CorrelationKey: "acme:cost",
		Subject: core.Subject{Alias: "gpt-4o"}, Data: map[string]any{"spend_1h_usd": 9.0}}
}

func TestFailOpenAppliesDespiteStoreError(t *testing.T) {
	e, f := engineWithStore(errStore{}, WithFailMode(FailOpen))
	res, _ := e.Handle(context.Background(), burnSignal())
	if countApplied(res) != 1 || f.Applied != 1 {
		t.Fatalf("fail-open should still actuate, applied=%d fake=%d", countApplied(res), f.Applied)
	}
}

func TestFailClosedRefusesOnStoreError(t *testing.T) {
	e, f := engineWithStore(errStore{}, WithFailMode(FailClosed))
	res, _ := e.Handle(context.Background(), burnSignal())
	if f.Applied != 0 {
		t.Fatalf("fail-closed must not actuate when state is unavailable, fired %d", f.Applied)
	}
	if len(res) != 1 || res[0].Outcome != OutFailed {
		t.Fatalf("expected OutFailed, got %+v", res)
	}
}

func TestMetricsCounted(t *testing.T) {
	m := metrics.New()
	st, _ := state.OpenSQLite(t.TempDir() + "/m.db")
	defer st.Close()
	rs, _ := rules.ParseRules([]byte(rule))
	rec, _ := rules.NewReconciler(rs)
	reg := actuator.NewRegistry()
	reg.Register(&actuator.Fake{})
	signer, _ := intent.NewEd25519Signer()
	e := New(rec, reg, st, signer, WithMetrics(m))
	_, _ = e.Handle(context.Background(), burnSignal())
	s := m.Snapshot()
	if s["signals_handled"] != 1 || s["intents_applied"] != 1 {
		t.Fatalf("metrics not counted: %+v", s)
	}
}
