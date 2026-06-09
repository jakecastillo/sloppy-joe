package engine

import (
	"context"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"

	"github.com/sloppyjoe/sloppy/actuator"
	"github.com/sloppyjoe/sloppy/core"
	"github.com/sloppyjoe/sloppy/intent"
	"github.com/sloppyjoe/sloppy/rules"
	"github.com/sloppyjoe/sloppy/state"
)

// Exercises the clean-restart path: a process applies a remediation, exits, and
// on restart replays the same signal. The honest durability model is
// at-least-once + idempotent — the persisted intent id prevents re-application
// on a clean restart. (A crash strictly between the actuator call and the mark
// is the at-least-once boundary; route_override is naturally idempotent, and
// incident-scoped dedup for non-idempotent actions like open_issue is Plan 2.)
func TestClosedLoopAgainstMockLiteLLM_Restart(t *testing.T) {
	var calls int64
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt64(&calls, 1)
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"ok":true}`))
	}))
	defer srv.Close()

	rs, _ := rules.ParseRules([]byte(rule))
	rec, _ := rules.NewReconciler(rs)
	reg := actuator.NewRegistry()
	reg.Register(actuator.NewLiteLLM(srv.URL, func() (string, error) { return "admin-xyz", nil }))
	signer, _ := intent.NewEd25519Signer()

	dbpath := t.TempDir() + "/loop.db"
	sig := core.Signal{
		Type: "cost.budget_burn", CorrelationKey: "acme:cost",
		Subject: core.Subject{Alias: "gpt-4o"}, Data: map[string]any{"spend_1h_usd": 9.0},
	}

	st1, _ := state.OpenSQLite(dbpath)
	e1 := New(rec, reg, st1, signer)
	res, err := e1.Handle(context.Background(), sig)
	if err != nil || countApplied(res) != 1 {
		t.Fatalf("first run applied=%d err=%v", countApplied(res), err)
	}
	st1.Close() // clean shutdown

	st2, _ := state.OpenSQLite(dbpath)
	defer st2.Close()
	e2 := New(rec, reg, st2, signer)
	res2, _ := e2.Handle(context.Background(), sig)
	if countApplied(res2) != 0 {
		t.Fatalf("after restart, replay must apply 0, applied %d", countApplied(res2))
	}
	if got := atomic.LoadInt64(&calls); got != 1 {
		t.Fatalf("LiteLLM admin must be called exactly once across restart+replay, got %d", got)
	}
	if !st2.VerifyAudit(context.Background()) {
		t.Fatal("audit chain invalid after resume")
	}
}
