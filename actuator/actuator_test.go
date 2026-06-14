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

// onlyNotify supports only open_issue, to exercise graceful degrade.
type onlyNotify struct {
	applied, reverted int
	lastKind          core.ActionKind
	lastArgs          map[string]any
}

func (o *onlyNotify) Capabilities() []core.ActionKind { return []core.ActionKind{core.ActionOpenIssue} }
func (o *onlyNotify) Apply(_ context.Context, i core.RemediationIntent) (core.Receipt, error) {
	o.applied++
	o.lastKind = i.Kind
	o.lastArgs = i.Args
	return core.Receipt{IntentID: i.ID, Actuator: "notify", Outcome: core.OutcomeApplied}, nil
}

func (o *onlyNotify) Revert(_ context.Context, i core.RemediationIntent) (core.Receipt, error) {
	o.reverted++
	return core.Receipt{IntentID: i.ID, Actuator: "notify", Outcome: core.OutcomeReverted}, nil
}

func TestRegistryGracefulDegrade(t *testing.T) {
	reg := NewRegistry()
	n := &onlyNotify{}
	reg.Register(n)

	// A known but unsupported kind degrades to the notify fallback (open_issue).
	rc, err := reg.Apply(context.Background(), core.RemediationIntent{ID: "x", Kind: core.ActionDisableDeployment, Target: "gpt-4o"})
	if err != nil || rc.Outcome != core.OutcomeApplied {
		t.Fatalf("degrade apply: %+v err=%v", rc, err)
	}
	if n.applied != 1 || n.lastKind != core.ActionOpenIssue {
		t.Fatalf("should degrade to open_issue, got kind=%s applied=%d", n.lastKind, n.applied)
	}
	if n.lastArgs["degraded_from"] != "disable_deployment" {
		t.Fatalf("should record degraded_from, got %+v", n.lastArgs)
	}

	// An unknown (garbage) kind still errors — no silent escalation.
	if _, err := reg.Apply(context.Background(), core.RemediationIntent{ID: "y", Kind: "nope"}); err == nil {
		t.Fatal("unknown kind must error even with a fallback present")
	}

	// With no fallback registered at all, a known unsupported kind errors.
	empty := NewRegistry()
	if _, err := empty.Apply(context.Background(), core.RemediationIntent{ID: "z", Kind: core.ActionDisableDeployment}); err == nil {
		t.Fatal("no fallback should error")
	}
}
