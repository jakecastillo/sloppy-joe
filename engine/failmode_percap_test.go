package engine

import (
	"context"
	"testing"

	"github.com/sloppyjoe/sloppy/actuator"
	"github.com/sloppyjoe/sloppy/core"
	"github.com/sloppyjoe/sloppy/intent"
	"github.com/sloppyjoe/sloppy/metrics"
	"github.com/sloppyjoe/sloppy/rules"
)

// On a store outage, mutating actuations must fail closed while notify actuations
// still fire — the Phase D per-capability fail-mode posture.
func TestPerCapabilityFailMode(t *testing.T) {
	rl := "on: cost.budget_burn\n" +
		"when: signal.data.spend_1h_usd > 5.0\n" +
		"then:\n" +
		"  - route_override: { alias: gpt-4o, to: ollama/llama3 }\n" +
		"  - page: { slack: \"#x\" }\n"
	rs, err := rules.ParseRules([]byte(rl))
	if err != nil {
		t.Fatal(err)
	}
	rec, _ := rules.NewReconciler(rs)
	reg := actuator.NewRegistry()
	f := &actuator.Fake{}
	reg.Register(f)
	signer, _ := intent.NewEd25519Signer()
	e := New(rec, reg, errStore{}, signer,
		WithMetrics(metrics.New()), WithFailMode(FailClosed), WithFailModeNotify(FailOpen))

	res, _ := e.Handle(context.Background(), burnSignal())

	var ro, page Outcome
	for _, r := range res {
		switch r.Intent.Kind {
		case core.ActionRouteOverride:
			ro = r.Outcome
		case core.ActionPage:
			page = r.Outcome
		}
	}
	if ro != OutFailed {
		t.Fatalf("mutating route_override must fail-closed on store error, got %q", ro)
	}
	if page != OutApplied {
		t.Fatalf("notify page must fail-open and proceed, got %q", page)
	}
	if f.Applied != 1 {
		t.Fatalf("only the notify action should have actuated, Applied=%d", f.Applied)
	}
}
