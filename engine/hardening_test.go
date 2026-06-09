package engine

import (
	"context"
	"testing"
	"time"

	"github.com/sloppyjoe/sloppy/actuator"
	"github.com/sloppyjoe/sloppy/core"
	"github.com/sloppyjoe/sloppy/intent"
	"github.com/sloppyjoe/sloppy/metrics"
	"github.com/sloppyjoe/sloppy/rules"
	"github.com/sloppyjoe/sloppy/state"
)

const ttlRule = `
on: cost.budget_burn
when: signal.data.spend_1h_usd > 5.0
then: [ { route_override: { alias: gpt-4o, to: ollama/llama3, ttl: 30m } } ]
`

func burnSig() core.Signal {
	return core.Signal{Type: "cost.budget_burn", CorrelationKey: "acme:cost",
		Subject: core.Subject{Alias: "gpt-4o"}, Data: map[string]any{"spend_1h_usd": 9.0}}
}

func auditHas(t *testing.T, st state.Store, kind string) bool {
	t.Helper()
	es, _ := st.Audit(context.Background())
	for _, e := range es {
		if e.Kind == kind {
			return true
		}
	}
	return false
}

// A failed Revert must NOT mark the intent reverted — it must stay pending and retry.
func TestRevertFailureKeepsPendingAndRetries(t *testing.T) {
	base := time.Unix(1749340800, 0).UTC()
	now := base
	rs, _ := rules.ParseRules([]byte(ttlRule))
	rec, _ := rules.NewReconciler(rs)
	st, _ := state.OpenSQLite(t.TempDir() + "/h.db")
	defer st.Close()
	reg := actuator.NewRegistry()
	f := &actuator.Fake{RevertErr: errBoom}
	reg.Register(f)
	signer, _ := intent.NewEd25519Signer()
	e := New(rec, reg, st, signer, WithClock(func() time.Time { return now }))

	if res, _ := e.Handle(context.Background(), burnSig()); countApplied(res) != 1 {
		t.Fatal("expected apply")
	}
	// Revert fails → not counted, audited, still pending.
	if n, _ := e.ProcessDueReverts(context.Background(), base.Add(31*time.Minute)); n != 0 {
		t.Fatalf("failed revert must not count, got %d", n)
	}
	if f.Reverted != 1 {
		t.Fatalf("revert should have been attempted once, got %d", f.Reverted)
	}
	if !auditHas(t, st, "intent.revert_failed") {
		t.Fatal("expected intent.revert_failed audit entry")
	}
	// Now it succeeds → still due, gets reverted exactly once.
	f.RevertErr = nil
	if n, _ := e.ProcessDueReverts(context.Background(), base.Add(32*time.Minute)); n != 1 {
		t.Fatalf("retry should revert once, got %d", n)
	}
}

// A store whose MarkReverted fails must not silently loop or count as reverted.
type markFailStore struct {
	state.Store
}

func (m markFailStore) MarkReverted(context.Context, string) error { return errBoom }

func TestMarkRevertedFailureSurfaced(t *testing.T) {
	base := time.Unix(1749340800, 0).UTC()
	now := base
	rs, _ := rules.ParseRules([]byte(ttlRule))
	rec, _ := rules.NewReconciler(rs)
	inner, _ := state.OpenSQLite(t.TempDir() + "/m.db")
	defer inner.Close()
	st := markFailStore{Store: inner}
	reg := actuator.NewRegistry()
	reg.Register(&actuator.Fake{})
	signer, _ := intent.NewEd25519Signer()
	e := New(rec, reg, st, signer, WithClock(func() time.Time { return now }))

	if res, _ := e.Handle(context.Background(), burnSig()); countApplied(res) != 1 {
		t.Fatal("expected apply")
	}
	n, _ := e.ProcessDueReverts(context.Background(), base.Add(31*time.Minute))
	if n != 0 {
		t.Fatalf("MarkReverted failure must not count as reverted, got %d", n)
	}
	if !auditHas(t, inner, "intent.revert_mark_failed") {
		t.Fatal("expected intent.revert_mark_failed audit entry")
	}
}

// A store whose ReleaseIntent fails AFTER the actuator errors must surface the
// stuck-claimed id (at-most-once is preserved, but the intent won't auto-retry
// until its retention lapses — never silently).
type releaseFailStore struct {
	state.Store
}

func (releaseFailStore) ReleaseIntent(context.Context, string) error { return errBoom }

func TestClaimReleaseFailureSurfaced(t *testing.T) {
	now := time.Unix(1749340800, 0).UTC()
	rs, _ := rules.ParseRules([]byte(ttlRule))
	rec, _ := rules.NewReconciler(rs)
	inner, _ := state.OpenSQLite(t.TempDir() + "/rf.db")
	defer inner.Close()
	st := releaseFailStore{Store: inner}
	reg := actuator.NewRegistry()
	f := &actuator.Fake{ApplyErr: errBoom} // actuation fails → engine tries to release the claim
	reg.Register(f)
	m := metrics.New()
	signer, _ := intent.NewEd25519Signer()
	e := New(rec, reg, st, signer, WithMetrics(m), WithClock(func() time.Time { return now }))

	res, _ := e.Handle(context.Background(), burnSig())
	if len(res) != 1 || res[0].Outcome != OutFailed {
		t.Fatalf("apply failure should yield OutFailed, got %+v", res)
	}
	if m.Snapshot()["claim_release_failed"] != 1 {
		t.Fatalf("claim_release_failed metric must fire, got %d", m.Snapshot()["claim_release_failed"])
	}
	if !auditHas(t, inner, "intent.claim_release_failed") {
		t.Fatal("expected intent.claim_release_failed audit entry")
	}
}

// When actuation fails, the engine must RELEASE the idempotency claim so a
// later retry of the same signal re-attempts the intent (claim is a gate, not a
// tombstone). With a real store, the first apply fails; once the actuator
// recovers, a replay must actuate exactly once.
func TestFailedApplyReleasesClaimForRetry(t *testing.T) {
	now := time.Unix(1749340800, 0).UTC()
	rs, _ := rules.ParseRules([]byte(ttlRule))
	rec, _ := rules.NewReconciler(rs)
	st, _ := state.OpenSQLite(t.TempDir() + "/rr.db")
	defer st.Close()
	reg := actuator.NewRegistry()
	f := &actuator.Fake{ApplyErr: errBoom}
	reg.Register(f)
	signer, _ := intent.NewEd25519Signer()
	e := New(rec, reg, st, signer, WithClock(func() time.Time { return now }))

	// First attempt: actuator fails → OutFailed, claim released.
	if res, _ := e.Handle(context.Background(), burnSig()); res[0].Outcome != OutFailed {
		t.Fatalf("first attempt should fail, got %+v", res)
	}
	// Actuator recovers; the SAME signal replays and must actuate (not skip).
	f.ApplyErr = nil
	res, _ := e.Handle(context.Background(), burnSig())
	if countApplied(res) != 1 || f.Applied != 2 {
		t.Fatalf("retry after recovery must actuate once more, applied=%d fake=%d", countApplied(res), f.Applied)
	}
}

// A one-shot spike that never satisfies `for:` must not leak the pending map.
func TestPendingWindowDoesNotLeak(t *testing.T) {
	base := time.Unix(1749340800, 0).UTC()
	now := base
	e, _, st := mustEngine(t, `
on: cost.budget_burn
when: signal.data.spend_1h_usd > 5.0
for: 1m
then: [ { page: { slack: "#x" } } ]
`, WithClock(func() time.Time { return now }))
	defer st.Close()

	if _, err := e.Handle(context.Background(), burnSig()); err != nil {
		t.Fatal(err)
	}
	if len(e.pending) != 1 {
		t.Fatalf("expected 1 pending window, got %d", len(e.pending))
	}
	// Long after the window, a different incident triggers the sweep.
	now = base.Add(10 * time.Minute)
	other := burnSig()
	other.CorrelationKey = "other:cost"
	if _, err := e.Handle(context.Background(), other); err != nil {
		t.Fatal(err)
	}
	if len(e.pending) != 1 {
		t.Fatalf("stale pending window should have been swept; got %d entries", len(e.pending))
	}
}
