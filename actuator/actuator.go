// Package actuator applies RemediationIntents to the outside world.
package actuator

import (
	"context"
	"fmt"
	"io"
	"sort"
	"time"

	"github.com/sloppyjoe/sloppy/core"
)

// knownKind is the set of defined action kinds (for graceful-degrade decisions).
var knownKind = func() map[core.ActionKind]bool {
	m := map[core.ActionKind]bool{}
	for _, k := range core.KnownActionKinds() {
		m[k] = true
	}
	return m
}()

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

// Apply dispatches one intent. A known-but-unsupported kind gracefully degrades
// to a notify fallback (open_issue/page) so the incident is escalated to a human
// instead of silently failing (spec §11). Unknown kinds still error.
func (r *Registry) Apply(ctx context.Context, i core.RemediationIntent) (core.Receipt, error) {
	if a, ok := r.byKind[i.Kind]; ok {
		return a.Apply(ctx, i)
	}
	if a, fk, ok := r.fallback(i.Kind); ok {
		return a.Apply(ctx, degrade(i, fk))
	}
	return core.Receipt{IntentID: i.ID, Outcome: core.OutcomeFailed}, fmt.Errorf("actuator: no actuator for kind %q (no fallback)", i.Kind)
}

// Revert dispatches a revert, degrading the same way as Apply.
func (r *Registry) Revert(ctx context.Context, i core.RemediationIntent) (core.Receipt, error) {
	if a, ok := r.byKind[i.Kind]; ok {
		return a.Revert(ctx, i)
	}
	if a, fk, ok := r.fallback(i.Kind); ok {
		return a.Revert(ctx, degrade(i, fk))
	}
	return core.Receipt{IntentID: i.ID, Outcome: core.OutcomeFailed}, fmt.Errorf("actuator: no actuator for kind %q (no fallback)", i.Kind)
}

// fallback returns a notify actuator (open_issue, else page) to escalate a
// known-but-unsupported kind. Unknown kinds get no fallback.
func (r *Registry) fallback(k core.ActionKind) (Actuator, core.ActionKind, bool) {
	if !knownKind[k] {
		return nil, "", false
	}
	for _, fk := range []core.ActionKind{core.ActionOpenIssue, core.ActionPage} {
		if a, ok := r.byKind[fk]; ok {
			return a, fk, true
		}
	}
	return nil, "", false
}

// degrade rewrites an intent to a fallback kind, recording the original kind
// (without mutating the caller's Args map).
func degrade(i core.RemediationIntent, fk core.ActionKind) core.RemediationIntent {
	d := i
	d.Kind = fk
	na := make(map[string]any, len(i.Args)+1)
	for k, v := range i.Args {
		na[k] = v
	}
	na["degraded_from"] = string(i.Kind)
	d.Args = na
	return d
}

// Kinds returns all action kinds the registry can handle, sorted.
func (r *Registry) Kinds() []core.ActionKind {
	out := make([]core.ActionKind, 0, len(r.byKind))
	for k := range r.byKind {
		out = append(out, k)
	}
	sort.Slice(out, func(a, b int) bool { return out[a] < out[b] })
	return out
}

// Fake is a test actuator covering all kinds; counts calls and can inject errors.
type Fake struct {
	Applied, Reverted   int
	ApplyErr, RevertErr error
}

func (f *Fake) Capabilities() []core.ActionKind {
	return []core.ActionKind{core.ActionRouteOverride, core.ActionOpenIssue, core.ActionPage, core.ActionThrottleTenant, core.ActionDisableDeployment}
}

func (f *Fake) Apply(_ context.Context, i core.RemediationIntent) (core.Receipt, error) {
	f.Applied++
	if f.ApplyErr != nil {
		return core.Receipt{IntentID: i.ID, Actuator: "fake", Outcome: core.OutcomeFailed}, f.ApplyErr
	}
	return core.Receipt{IntentID: i.ID, Actuator: "fake", Outcome: core.OutcomeApplied}, nil
}

func (f *Fake) Revert(_ context.Context, i core.RemediationIntent) (core.Receipt, error) {
	f.Reverted++
	if f.RevertErr != nil {
		return core.Receipt{IntentID: i.ID, Actuator: "fake", Outcome: core.OutcomeFailed}, f.RevertErr
	}
	return core.Receipt{IntentID: i.ID, Actuator: "fake", Outcome: core.OutcomeReverted}, nil
}

// Log is a side-effect-free actuator that records actions to a writer.
// Used as the default for the CLI demo when no live gateway is configured.
type Log struct{ W io.Writer }

func (l *Log) Capabilities() []core.ActionKind {
	return []core.ActionKind{core.ActionRouteOverride, core.ActionOpenIssue, core.ActionPage, core.ActionThrottleTenant, core.ActionDisableDeployment}
}

func (l *Log) Apply(_ context.Context, i core.RemediationIntent) (core.Receipt, error) {
	fmt.Fprintf(l.W, "  → %s target=%s args=%v\n", i.Kind, i.Target, i.Args)
	return core.Receipt{IntentID: i.ID, Actuator: "log", AppliedAt: time.Now().UTC(), Outcome: core.OutcomeApplied}, nil
}

func (l *Log) Revert(_ context.Context, i core.RemediationIntent) (core.Receipt, error) {
	fmt.Fprintf(l.W, "  ↩ revert %s target=%s\n", i.Kind, i.Target)
	return core.Receipt{IntentID: i.ID, Actuator: "log", Outcome: core.OutcomeReverted}, nil
}
