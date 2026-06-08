package actuator

import (
	"context"
	"testing"

	"github.com/sloppyjoe/sloppy/core"
)

func TestRegistryDispatchAndCapability(t *testing.T) {
	reg := NewRegistry()
	f := &Fake{}
	reg.Register(f)
	r, err := reg.Apply(context.Background(), core.RemediationIntent{Kind: core.ActionRouteOverride, Target: "gpt-4o"})
	if err != nil || r.Outcome != core.OutcomeApplied {
		t.Fatalf("apply: %+v err=%v", r, err)
	}
	if f.Applied != 1 {
		t.Fatalf("fake should have applied once, got %d", f.Applied)
	}
	if !reg.Supports(core.ActionPage) {
		t.Fatal("fake should support page")
	}
	if _, err := reg.Apply(context.Background(), core.RemediationIntent{Kind: "nope"}); err == nil {
		t.Fatal("expected error for unsupported action kind")
	}
}
