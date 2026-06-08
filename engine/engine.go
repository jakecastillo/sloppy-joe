// Package engine wires reconcile → sign → govern → actuate → audit.
package engine

import (
	"context"
	"fmt"

	"github.com/sloppyjoe/sloppy/actuator"
	"github.com/sloppyjoe/sloppy/core"
	"github.com/sloppyjoe/sloppy/intent"
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
)

// Result reports what happened to one intent in a Handle pass.
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
}

// New builds an engine.
func New(rec *rules.Reconciler, reg *actuator.Registry, store state.Store, signer intent.Signer) *Engine {
	return &Engine{rec: rec, reg: reg, store: store, signer: signer}
}

// Handle runs one signal through the loop and returns per-intent results.
func (e *Engine) Handle(ctx context.Context, sig core.Signal) ([]Result, error) {
	intents := e.rec.Reconcile(sig, nil)
	var results []Result
	for _, in := range intents {
		// Sign for provenance before any decision/action.
		in.Signature = e.signer.Sign(in.CanonicalBytes())

		if in.DryRun {
			_, _ = e.store.AppendAudit("intent.dry_run", auditDetail(in))
			results = append(results, Result{Intent: in, Outcome: OutDryRun})
			continue
		}
		// Idempotency / crash-resume: skip already-applied intents.
		if done, _ := e.store.IsIntentApplied(in.ID); done {
			results = append(results, Result{Intent: in, Outcome: OutSkipped})
			continue
		}
		rcpt, err := e.reg.Apply(ctx, in)
		if err != nil {
			_, _ = e.store.AppendAudit("intent.failed", fmt.Sprintf("%s err=%v", auditDetail(in), err))
			results = append(results, Result{Intent: in, Outcome: OutFailed, Err: err.Error()})
			continue
		}
		rcpt.Signature = e.signer.Sign([]byte(rcpt.IntentID + string(rcpt.Outcome) + rcpt.Actuator))
		_ = e.store.MarkIntentApplied(in.ID)
		_, _ = e.store.AppendAudit("intent.applied", auditDetail(in)+" sig="+short(in.Signature))
		results = append(results, Result{Intent: in, Outcome: OutApplied, Receipt: rcpt})
	}
	return results, nil
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
