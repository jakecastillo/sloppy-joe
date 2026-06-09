package actuator

import (
	"context"
	"testing"

	"github.com/sloppyjoe/sloppy/core"
)

// Conformance asserts an Actuator satisfies the Apply/Revert/Receipt contract.
// External actuator authors should call this from their own tests:
//
//	func TestMyActuator(t *testing.T) {
//	    actuator.Conformance(t, NewMyActuator(...), core.RemediationIntent{ID: "c1", Target: "x"})
//	}
func Conformance(tb testing.TB, a Actuator, sample core.RemediationIntent) {
	tb.Helper()
	caps := a.Capabilities()
	if len(caps) == 0 {
		tb.Fatalf("conformance: Capabilities() must be non-empty")
	}
	for _, k := range caps {
		s := sample
		s.Kind = k
		rc, err := a.Apply(context.Background(), s)
		if err != nil {
			tb.Fatalf("conformance[%s]: Apply error: %v", k, err)
		}
		if rc.Outcome != core.OutcomeApplied {
			tb.Fatalf("conformance[%s]: Apply must yield OutcomeApplied, got %q", k, rc.Outcome)
		}
		if rc.IntentID != s.ID {
			tb.Fatalf("conformance[%s]: receipt IntentID %q != intent ID %q", k, rc.IntentID, s.ID)
		}
		rv, err := a.Revert(context.Background(), s)
		if err != nil {
			tb.Fatalf("conformance[%s]: Revert error: %v", k, err)
		}
		if rv.Outcome != core.OutcomeReverted {
			tb.Fatalf("conformance[%s]: Revert must yield OutcomeReverted, got %q", k, rv.Outcome)
		}
	}
}
