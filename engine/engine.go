// Package engine wires reconcile → sign → govern → actuate → audit, with
// `for:` windowing, ledger-driven state, and durable TTL auto-revert.
package engine

import (
	"context"
	"encoding/json"
	"fmt"
	"sync"
	"time"

	"github.com/sloppyjoe/sloppy/actuator"
	"github.com/sloppyjoe/sloppy/core"
	"github.com/sloppyjoe/sloppy/intent"
	"github.com/sloppyjoe/sloppy/ledger"
	"github.com/sloppyjoe/sloppy/rules"
	"github.com/sloppyjoe/sloppy/state"
)

// Outcome is the engine-level disposition of one intent.
type Outcome string

const (
	OutApplied Outcome = "applied"
	OutDryRun  Outcome = "dry_run"
	OutSkipped Outcome = "skipped_idempotent"
	OutFailed  Outcome = "failed"
	OutPending Outcome = "pending_for_window"
)

// Result reports what happened to one intent (or a pending rule) in a Handle pass.
type Result struct {
	Intent  core.RemediationIntent
	Outcome Outcome
	Receipt core.Receipt
	Err     string
}

// Engine is the off-hot-path control loop core.
type Engine struct {
	rec    *rules.Reconciler
	reg    *actuator.Registry
	store  state.Store
	signer intent.Signer
	led    *ledger.CostLedger
	now    func() time.Time

	mu      sync.Mutex
	pending map[string]time.Time // ruleSHA|correlationKey -> first-seen, for `for:` gating
}

// Option configures an Engine.
type Option func(*Engine)

// WithLedger supplies a cost ledger whose StateFor() feeds CEL `state.*`.
func WithLedger(l *ledger.CostLedger) Option { return func(e *Engine) { e.led = l } }

// WithClock overrides the clock (for tests / determinism).
func WithClock(fn func() time.Time) Option { return func(e *Engine) { e.now = fn } }

// New builds an engine. Extra behaviour is opt-in via Options (back-compatible).
func New(rec *rules.Reconciler, reg *actuator.Registry, store state.Store, signer intent.Signer, opts ...Option) *Engine {
	e := &Engine{
		rec: rec, reg: reg, store: store, signer: signer,
		now:     func() time.Time { return time.Now().UTC() },
		pending: map[string]time.Time{},
	}
	for _, o := range opts {
		o(e)
	}
	return e
}

// Handle runs one signal through the loop and returns per-intent results.
func (e *Engine) Handle(ctx context.Context, sig core.Signal) ([]Result, error) {
	now := e.now()
	var st map[string]any
	if e.led != nil {
		st = e.led.StateFor(sig.Subject.Tenant, now)
	}
	matches := e.rec.EvaluateMatches(sig, st)
	var results []Result
	for _, m := range matches {
		if m.Rule.For > 0 && !e.forWindowSatisfied(m.Rule.SHA, sig.CorrelationKey, m.Rule.For, now) {
			results = append(results, Result{Intent: core.RemediationIntent{RuleSHA: m.Rule.SHA}, Outcome: OutPending})
			continue
		}
		for _, in := range m.Intents {
			results = append(results, e.applyIntent(ctx, in, now))
		}
	}
	return results, nil
}

// forWindowSatisfied is true once a rule's condition has held for >= dur.
func (e *Engine) forWindowSatisfied(ruleSHA, corr string, dur time.Duration, now time.Time) bool {
	key := ruleSHA + "|" + corr
	e.mu.Lock()
	defer e.mu.Unlock()
	first, ok := e.pending[key]
	if !ok {
		e.pending[key] = now
		return false
	}
	if now.Sub(first) >= dur {
		delete(e.pending, key) // re-arm for the next incident
		return true
	}
	return false
}

func (e *Engine) applyIntent(ctx context.Context, in core.RemediationIntent, now time.Time) Result {
	in.Signature = e.signer.Sign(in.CanonicalBytes())
	if in.DryRun {
		_, _ = e.store.AppendAudit("intent.dry_run", auditDetail(in))
		return Result{Intent: in, Outcome: OutDryRun}
	}
	if done, _ := e.store.IsIntentApplied(in.ID); done {
		return Result{Intent: in, Outcome: OutSkipped}
	}
	rcpt, err := e.reg.Apply(ctx, in)
	if err != nil {
		_, _ = e.store.AppendAudit("intent.failed", fmt.Sprintf("%s err=%v", auditDetail(in), err))
		return Result{Intent: in, Outcome: OutFailed, Err: err.Error()}
	}
	rcpt.Signature = e.signer.Sign([]byte(rcpt.IntentID + string(rcpt.Outcome) + rcpt.Actuator))
	_ = e.store.MarkIntentApplied(in.ID)
	_, _ = e.store.AppendAudit("intent.applied", auditDetail(in)+" sig="+short(in.Signature))
	if in.TTL > 0 {
		args, _ := json.Marshal(in.Args)
		_ = e.store.ScheduleRevert(state.PendingRevert{
			IntentID: in.ID, Kind: string(in.Kind), Target: in.Target,
			ArgsJSON: string(args), DueAt: now.Add(in.TTL),
		})
	}
	return Result{Intent: in, Outcome: OutApplied, Receipt: rcpt}
}

// ProcessDueReverts reverts intents whose TTL has elapsed. Returns count reverted.
func (e *Engine) ProcessDueReverts(ctx context.Context, now time.Time) (int, error) {
	due, err := e.store.DueReverts(now)
	if err != nil {
		return 0, err
	}
	n := 0
	for _, pr := range due {
		var args map[string]any
		if pr.ArgsJSON != "" {
			_ = json.Unmarshal([]byte(pr.ArgsJSON), &args)
		}
		in := core.RemediationIntent{ID: pr.IntentID, Kind: core.ActionKind(pr.Kind), Target: pr.Target, Args: args}
		if _, err := e.reg.Revert(ctx, in); err != nil {
			_, _ = e.store.AppendAudit("intent.revert_failed", fmt.Sprintf("%s err=%v", pr.IntentID, err))
			continue
		}
		_ = e.store.MarkReverted(pr.IntentID)
		_, _ = e.store.AppendAudit("intent.reverted", fmt.Sprintf("%s target=%s", pr.Kind, pr.Target))
		n++
	}
	return n, nil
}

func auditDetail(i core.RemediationIntent) string {
	return fmt.Sprintf("%s target=%s rule=%s", i.Kind, i.Target, i.RuleSHA)
}

func short(s string) string {
	if len(s) > 12 {
		return s[:12]
	}
	return s
}
