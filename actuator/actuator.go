// Package actuator applies RemediationIntents to the outside world.
package actuator

import (
	"context"
	"fmt"
	"io"
	"time"

	"github.com/sloppyjoe/sloppy/core"
)

// Actuator executes (and can revert) one or more action kinds.
type Actuator interface {
	Capabilities() []core.ActionKind
	Apply(ctx context.Context, i core.RemediationIntent) (core.Receipt, error)
	Revert(ctx context.Context, i core.RemediationIntent) (core.Receipt, error)
}

// Registry routes intents to the actuator that declares their kind.
type Registry struct{ byKind map[core.ActionKind]Actuator }

// NewRegistry creates an empty registry.
func NewRegistry() *Registry { return &Registry{byKind: map[core.ActionKind]Actuator{}} }

// Register wires an actuator's declared capabilities (last registration wins per kind).
func (r *Registry) Register(a Actuator) {
	for _, k := range a.Capabilities() {
		r.byKind[k] = a
	}
}

// Supports reports whether any actuator handles the kind.
func (r *Registry) Supports(k core.ActionKind) bool { _, ok := r.byKind[k]; return ok }

// Apply dispatches one intent.
func (r *Registry) Apply(ctx context.Context, i core.RemediationIntent) (core.Receipt, error) {
	a, ok := r.byKind[i.Kind]
	if !ok {
		return core.Receipt{IntentID: i.ID, Outcome: core.OutcomeFailed}, fmt.Errorf("actuator: no actuator for kind %q", i.Kind)
	}
	return a.Apply(ctx, i)
}

// Revert dispatches a revert.
func (r *Registry) Revert(ctx context.Context, i core.RemediationIntent) (core.Receipt, error) {
	a, ok := r.byKind[i.Kind]
	if !ok {
		return core.Receipt{IntentID: i.ID, Outcome: core.OutcomeFailed}, fmt.Errorf("actuator: no actuator for kind %q", i.Kind)
	}
	return a.Revert(ctx, i)
}

// Fake is a test actuator covering all kinds; counts calls.
type Fake struct{ Applied, Reverted int }

func (f *Fake) Capabilities() []core.ActionKind {
	return []core.ActionKind{core.ActionRouteOverride, core.ActionOpenIssue, core.ActionPage}
}
func (f *Fake) Apply(_ context.Context, i core.RemediationIntent) (core.Receipt, error) {
	f.Applied++
	return core.Receipt{IntentID: i.ID, Actuator: "fake", Outcome: core.OutcomeApplied}, nil
}
func (f *Fake) Revert(_ context.Context, i core.RemediationIntent) (core.Receipt, error) {
	f.Reverted++
	return core.Receipt{IntentID: i.ID, Actuator: "fake", Outcome: core.OutcomeReverted}, nil
}

// Log is a side-effect-free actuator that records actions to a writer.
// Used as the default for the CLI demo when no live gateway is configured.
type Log struct{ W io.Writer }

func (l *Log) Capabilities() []core.ActionKind {
	return []core.ActionKind{core.ActionRouteOverride, core.ActionOpenIssue, core.ActionPage}
}
func (l *Log) Apply(_ context.Context, i core.RemediationIntent) (core.Receipt, error) {
	fmt.Fprintf(l.W, "  → %s target=%s args=%v\n", i.Kind, i.Target, i.Args)
	return core.Receipt{IntentID: i.ID, Actuator: "log", AppliedAt: time.Now().UTC(), Outcome: core.OutcomeApplied}, nil
}
func (l *Log) Revert(_ context.Context, i core.RemediationIntent) (core.Receipt, error) {
	fmt.Fprintf(l.W, "  ↩ revert %s target=%s\n", i.Kind, i.Target)
	return core.Receipt{IntentID: i.ID, Actuator: "log", Outcome: core.OutcomeReverted}, nil
}
