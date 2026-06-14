package engine

import (
	"context"
	"sync"
	"sync/atomic"
	"testing"

	"github.com/alicebob/miniredis/v2"

	"github.com/sloppyjoe/sloppy/actuator"
	"github.com/sloppyjoe/sloppy/core"
	"github.com/sloppyjoe/sloppy/intent"
	"github.com/sloppyjoe/sloppy/metrics"
	"github.com/sloppyjoe/sloppy/rules"
	"github.com/sloppyjoe/sloppy/state"
)

// atomicFake counts Apply calls with an atomic so the concurrency test below is
// safe under -race (the shared actuator.Fake increments a plain int).
type atomicFake struct{ applied int64 }

func (f *atomicFake) Capabilities() []core.ActionKind {
	return []core.ActionKind{core.ActionRouteOverride, core.ActionOpenIssue, core.ActionPage, core.ActionThrottleTenant, core.ActionDisableDeployment}
}

func (f *atomicFake) Apply(_ context.Context, i core.RemediationIntent) (core.Receipt, error) {
	atomic.AddInt64(&f.applied, 1)
	return core.Receipt{IntentID: i.ID, Actuator: "atomic-fake", Outcome: core.OutcomeApplied}, nil
}

func (f *atomicFake) Revert(_ context.Context, i core.RemediationIntent) (core.Receipt, error) {
	return core.Receipt{IntentID: i.ID, Actuator: "atomic-fake", Outcome: core.OutcomeReverted}, nil
}

// concurrentEngine builds an engine over the supplied store with an atomic fake.
func concurrentEngine(t *testing.T, st state.Store, m *metrics.Registry) (*Engine, *atomicFake) {
	t.Helper()
	rs, err := rules.ParseRules([]byte(rule))
	if err != nil {
		t.Fatal(err)
	}
	rec, err := rules.NewReconciler(rs)
	if err != nil {
		t.Fatal(err)
	}
	reg := actuator.NewRegistry()
	f := &atomicFake{}
	reg.Register(f)
	signer, _ := intent.NewEd25519Signer()
	return New(rec, reg, st, signer, WithMetrics(m)), f
}

// fireSameSignalConcurrently drives N goroutines through Handle with the SAME
// signal (so every goroutine derives the identical deterministic intent ID) and
// asserts the store-level claim gate let exactly one win: one Apply, exactly one
// OutApplied, the rest OutSkipped. This is the core TOCTOU regression test — it
// must hold under -race against a real backend, not an in-process mutex.
func fireSameSignalConcurrently(t *testing.T, st state.Store) {
	t.Helper()
	m := metrics.New()
	e, f := concurrentEngine(t, st, m)

	const n = 24
	var (
		wg      sync.WaitGroup
		mu      sync.Mutex
		applied int
		skipped int
	)
	start := make(chan struct{})
	for i := 0; i < n; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			<-start // release all goroutines at once to maximize the race
			res, err := e.Handle(context.Background(), burnSignal())
			if err != nil {
				t.Errorf("handle: %v", err)
				return
			}
			mu.Lock()
			for _, r := range res {
				switch r.Outcome {
				case OutApplied:
					applied++
				case OutSkipped:
					skipped++
				}
			}
			mu.Unlock()
		}()
	}
	close(start)
	wg.Wait()

	if got := atomic.LoadInt64(&f.applied); got != 1 {
		t.Fatalf("actuator must fire exactly once across %d concurrent identical signals, fired %d", n, got)
	}
	if applied != 1 {
		t.Fatalf("exactly one OutApplied expected, got %d", applied)
	}
	if skipped != n-1 {
		t.Fatalf("the other %d signals must be OutSkipped, got %d", n-1, skipped)
	}
	if snap := m.Snapshot(); snap["intents_applied"] != 1 {
		t.Fatalf("intents_applied metric must be 1, got %d", snap["intents_applied"])
	}
}

func TestConcurrentDoubleApplyClosedSQLite(t *testing.T) {
	st, err := state.OpenSQLite(t.TempDir() + "/toctou.db")
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()
	fireSameSignalConcurrently(t, st)
}

func TestConcurrentDoubleApplyClosedRedis(t *testing.T) {
	mr, err := miniredis.Run()
	if err != nil {
		t.Fatal(err)
	}
	defer mr.Close()
	st, err := state.OpenRedis(mr.Addr())
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()
	fireSameSignalConcurrently(t, st)
}
