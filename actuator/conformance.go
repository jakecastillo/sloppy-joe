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
	sample.Kind = caps[0]

	rc, err := a.Apply(context.Background(), sample)
	if err != nil {
		tb.Fatalf("conformance: Apply error: %v", err)
	}
	if rc.Outcome != core.OutcomeApplied {
		tb.Fatalf("conformance: Apply must yield OutcomeApplied, got %q", rc.Outcome)
	}
	if rc.IntentID != sample.ID {
		tb.Fatalf("conformance: receipt IntentID %q != intent ID %q", rc.IntentID, sample.ID)
	}

	rv, err := a.Revert(context.Background(), sample)
	if err != nil {
		tb.Fatalf("conformance: Revert error: %v", err)
	}
	if rv.Outcome != core.OutcomeReverted {
		tb.Fatalf("conformance: Revert must yield OutcomeReverted, got %q", rv.Outcome)
	}
}
