// Package engine wires reconcile → sign → govern → actuate → audit, with
// `for:` windowing, ledger-driven state, intent_budget + rollback:on_clear,
// durable TTL auto-revert, a fail-open/closed knob, and self-metrics.
package engine

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"sync"
	"time"

	"github.com/sloppyjoe/sloppy/actuator"
	"github.com/sloppyjoe/sloppy/core"
	"github.com/sloppyjoe/sloppy/intent"
	"github.com/sloppyjoe/sloppy/ledger"
	"github.com/sloppyjoe/sloppy/metrics"
	"github.com/sloppyjoe/sloppy/rules"
	"github.com/sloppyjoe/sloppy/state"
)

// Outcome is the engine-level disposition of one intent.
type Outcome string

const (
	OutApplied   Outcome = "applied"
	OutDryRun    Outcome = "dry_run"
	OutSkipped   Outcome = "skipped_idempotent"
	OutFailed    Outcome = "failed"
	OutPending   Outcome = "pending_for_window"
	OutThrottled Outcome = "throttled"
)

// FailMode decides behaviour when the state store is unreachable.
type FailMode int

const (
	// FailOpen proceeds with remediation when state can't be read (availability over strictness).
	FailOpen FailMode = iota
	// FailClosed refuses to act when state can't be read (strictness over availability).
	FailClosed
)

// Result reports what happened to one intent (or a pending/throttled rule) in a Handle pass.
type Result struct {
	Intent  core.RemediationIntent
	Outcome Outcome
	Receipt core.Receipt
	Err     string
}

// Engine is the off-hot-path control loop core.
type Engine struct {
	rec       *rules.Reconciler
	reg       *actuator.Registry
	store     state.Store
	signer    intent.Signer
	led       *ledger.CostLedger
	now       func() time.Time
	failMode  FailMode
	met       *metrics.Registry
	immediate bool
	log       *slog.Logger

	mu      sync.Mutex
	pending map[string]time.Time // ruleSHA|correlationKey -> first-seen, for `for:` gating
}

// Option configures an Engine.
type Option func(*Engine)

// WithLedger supplies a cost ledger whose StateFor() feeds CEL `state.*`.
func WithLedger(l *ledger.CostLedger) Option { return func(e *Engine) { e.led = l } }

// WithClock overrides the clock (for tests / determinism).
func WithClock(fn func() time.Time) Option { return func(e *Engine) { e.now = fn } }

// WithFailMode sets store-unreachable behaviour (default FailOpen).
func WithFailMode(m FailMode) Option { return func(e *Engine) { e.failMode = m } }

// WithMetrics attaches a self-metrics registry.
func WithMetrics(m *metrics.Registry) Option { return func(e *Engine) { e.met = m } }

// WithImmediate fires matching rules without waiting for their `for:` window.
// Used for one-shot CLI injection; the daemon evaluates `for:` across the live stream.
func WithImmediate() Option { return func(e *Engine) { e.immediate = true } }

// WithLogger attaches a structured logger for decision/error events (default: discard).
func WithLogger(l *slog.Logger) Option {
	return func(e *Engine) {
		if l != nil {
			e.log = l
		}
	}
}

// New builds an engine. Extra behaviour is opt-in via Options (back-compatible).
func New(rec *rules.Reconciler, reg *actuator.Registry, store state.Store, signer intent.Signer, opts ...Option) *Engine {
	e := &Engine{
		rec: rec, reg: reg, store: store, signer: signer,
		now:      func() time.Time { return time.Now().UTC() },
		failMode: FailOpen,
		log:      slog.New(slog.DiscardHandler),
		pending:  map[string]time.Time{},
	}
	for _, o := range opts {
		o(e)
	}
	return e
}

// Handle runs one signal through the loop and returns per-intent results.
func (e *Engine) Handle(ctx context.Context, sig core.Signal) ([]Result, error) {
	e.met.Inc("signals_handled")
	now := e.now()
	var st map[string]any
	if e.led != nil {
		st, _ = e.led.StateFor(ctx, sig.Subject.Tenant, now)
	}
	var results []Result
	for _, m := range e.rec.EvaluateMatches(sig, st) {
		if m.Rule.For > 0 && !e.immediate && !e.forWindowSatisfied(m.Rule.SHA, sig.CorrelationKey, m.Rule.For, now) {
			results = append(results, Result{Intent: core.RemediationIntent{RuleSHA: m.Rule.SHA}, Outcome: OutPending})
			continue
		}
		if e.throttled(ctx, m.Rule, now) {
			results = append(results, Result{Intent: core.RemediationIntent{RuleSHA: m.Rule.SHA}, Outcome: OutThrottled})
			continue
		}
		var applied []core.RemediationIntent
		for _, in := range m.Intents {
			r := e.applyIntent(ctx, in, now)
			results = append(results, r)
			if r.Outcome == OutApplied {
				applied = append(applied, r.Intent)
			}
		}
		if m.Rule.With.Rollback == "on_clear" && len(applied) > 0 {
			key := m.Rule.SHA + "|" + sig.CorrelationKey
			for _, in := range applied {
				args, _ := json.Marshal(in.Args)
				_ = e.store.RecordOutstanding(ctx, key, state.PendingRevert{
					IntentID: in.ID, Kind: string(in.Kind), Target: in.Target, ArgsJSON: string(args),
				})
			}
		}
	}
	// rollback:on_clear — revert outstanding intents for rules whose condition cleared.
	for _, r := range e.rec.Cleared(sig, st) {
		e.rollbackOnClear(ctx, r, sig.CorrelationKey)
	}
	return results, nil
}

// throttled enforces intent_budget per (rule, window): true when the budget is
// exhausted, otherwise it records this firing.
func (e *Engine) throttled(ctx context.Context, r rules.Rule, now time.Time) bool {
	count, window, err := rules.ParseIntentBudget(r.With.IntentBudget)
	if err != nil || count == 0 {
		return false // unset / unlimited
	}
	used, _ := e.store.CountActions(ctx, r.SHA, now.Add(-window))
	if used >= count {
		_, _ = e.store.AppendAudit(ctx, "intent.throttled", fmt.Sprintf("rule=%s budget=%s used=%d", r.SHA, r.With.IntentBudget, used))
		e.met.Inc("intents_throttled")
		e.log.Warn("rule throttled", "rule", r.SHA, "budget", r.With.IntentBudget)
		return true
	}
	_ = e.store.RecordAction(ctx, r.SHA, now)
	return false
}

// rollbackOnClear reverts and clears a rule's outstanding on-clear intents.
func (e *Engine) rollbackOnClear(ctx context.Context, r rules.Rule, corr string) {
	key := r.SHA + "|" + corr
	outs, _ := e.store.Outstanding(ctx, key)
	if len(outs) == 0 {
		return
	}
	for _, pr := range outs {
		var args map[string]any
		if pr.ArgsJSON != "" {
			_ = json.Unmarshal([]byte(pr.ArgsJSON), &args)
		}
		in := core.RemediationIntent{ID: pr.IntentID, Kind: core.ActionKind(pr.Kind), Target: pr.Target, Args: args}
		if _, err := e.reg.Revert(ctx, in); err != nil {
			_, _ = e.store.AppendAudit(ctx, "intent.rollback_failed", fmt.Sprintf("%s err=%v", pr.IntentID, err))
			continue
		}
		_, _ = e.store.AppendAudit(ctx, "intent.rolled_back", fmt.Sprintf("%s target=%s (condition cleared)", pr.Kind, pr.Target))
	}
	_ = e.store.ClearOutstanding(ctx, key)
	e.met.Inc("intents_rolled_back")
	e.log.Info("rule rolled back on clear", "rule", r.SHA, "corr", corr, "count", len(outs))
}

func (e *Engine) forWindowSatisfied(ruleSHA, corr string, dur time.Duration, now time.Time) bool {
	key := ruleSHA + "|" + corr
	e.mu.Lock()
	defer e.mu.Unlock()
	// Opportunistically expire stale windows so a one-shot spike that never
	// satisfies `for:` can't leak the pending map in a long-running daemon.
	for k, first := range e.pending {
		if now.Sub(first) > 2*dur {
			delete(e.pending, k)
		}
	}
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
		_, _ = e.store.AppendAudit(ctx, "intent.dry_run", auditDetail(in))
		e.met.Inc("intents_dry_run")
		return Result{Intent: in, Outcome: OutDryRun}
	}
	done, err := e.store.IsIntentApplied(ctx, in.ID)
	if err != nil {
		if e.failMode == FailClosed {
			_, _ = e.store.AppendAudit(ctx, "intent.fail_closed", auditDetail(in))
			e.met.Inc("intents_failed")
			return Result{Intent: in, Outcome: OutFailed, Err: "state unavailable (fail-closed)"}
		}
		done = false // fail-open: proceed as if not applied
	}
	if done {
		e.met.Inc("intents_skipped")
		return Result{Intent: in, Outcome: OutSkipped}
	}
	rcpt, err := e.reg.Apply(ctx, in)
	if err != nil {
		_, _ = e.store.AppendAudit(ctx, "intent.failed", fmt.Sprintf("%s err=%v", auditDetail(in), err))
		e.met.Inc("intents_failed")
		e.log.Warn("intent failed", "intent", in.ID, "kind", string(in.Kind), "target", in.Target, "err", err)
		return Result{Intent: in, Outcome: OutFailed, Err: err.Error()}
	}
	rcpt.Signature = e.signer.Sign([]byte(rcpt.IntentID + string(rcpt.Outcome) + rcpt.Actuator))
	if err := e.store.MarkIntentApplied(ctx, in.ID); err != nil {
		// A lost idempotency record silently breaks at-most-once — surface it.
		e.met.Inc("state_write_failed")
		_, _ = e.store.AppendAudit(ctx, "intent.mark_failed", auditDetail(in)+fmt.Sprintf(" err=%v", err))
	}
	if _, err := e.store.AppendAudit(ctx, "intent.applied", auditDetail(in)+" sig="+short(in.Signature)); err != nil {
		e.met.Inc("audit_write_failed")
	}
	if in.TTL > 0 {
		args, _ := json.Marshal(in.Args)
		if err := e.store.ScheduleRevert(ctx, state.PendingRevert{
			IntentID: in.ID, Kind: string(in.Kind), Target: in.Target,
			ArgsJSON: string(args), DueAt: now.Add(in.TTL),
		}); err != nil {
			// The TTL auto-revert safety net wasn't armed — make it observable.
			e.met.Inc("reverts_unscheduled")
			_, _ = e.store.AppendAudit(ctx, "intent.revert_unscheduled", auditDetail(in)+fmt.Sprintf(" err=%v", err))
		}
	}
	e.met.Inc("intents_applied")
	e.log.Info("intent applied", "intent", in.ID, "kind", string(in.Kind), "target", in.Target, "rule", in.RuleSHA)
	return Result{Intent: in, Outcome: OutApplied, Receipt: rcpt}
}

// ProcessDueReverts reverts intents whose TTL has elapsed. Returns count reverted.
func (e *Engine) ProcessDueReverts(ctx context.Context, now time.Time) (int, error) {
	due, err := e.store.DueReverts(ctx, now)
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
			_, _ = e.store.AppendAudit(ctx, "intent.revert_failed", fmt.Sprintf("%s err=%v", pr.IntentID, err))
			continue // leave it pending so the next tick retries
		}
		if err := e.store.MarkReverted(ctx, pr.IntentID); err != nil {
			// Don't count as reverted or it re-fires forever — surface the stuck row.
			e.met.Inc("reverts_unmarked")
			_, _ = e.store.AppendAudit(ctx, "intent.revert_mark_failed", fmt.Sprintf("%s err=%v", pr.IntentID, err))
			continue
		}
		_, _ = e.store.AppendAudit(ctx, "intent.reverted", fmt.Sprintf("%s target=%s", pr.Kind, pr.Target))
		e.met.Inc("intents_reverted")
		e.log.Info("intent reverted", "intent", pr.IntentID, "kind", pr.Kind, "target", pr.Target)
		n++
	}
	return n, nil
}

// PruneUsage deletes usage events older than `before` (called by the daemon ticker).
func (e *Engine) PruneUsage(ctx context.Context, before time.Time) error {
	return e.store.PruneUsage(ctx, before)
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
