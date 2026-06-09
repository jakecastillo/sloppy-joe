package state

import (
	"context"
	"time"
)

// PendingRevert is a durable record of an applied, reversible intent awaiting TTL expiry.
type PendingRevert struct {
	IntentID string
	Kind     string
	Target   string
	ArgsJSON string
	DueAt    time.Time
}

// Store is the durable backend. SQLite for v0; Redis for multi-replica.
// Every I/O method takes a context so callers' cancellation/deadlines reach the
// backend (Close is teardown and intentionally context-free).
type Store interface {
	// ClaimIntent atomically records the idempotency key for intentID and reports
	// whether THIS caller created it. It is the at-most-once gate that closes the
	// apply-twice TOCTOU: with N concurrent callers for one id, exactly one gets
	// claimed==true (it then actuates); the rest get false (they skip). A losing
	// claim is not an error. Backed by a single conditional write (SQLite
	// INSERT + PK conflict; Redis SET NX), never an in-process lock, so the gate
	// holds across replicas.
	ClaimIntent(ctx context.Context, intentID string) (claimed bool, err error)
	// ReleaseIntent removes a previously claimed idempotency key so the intent can
	// be re-attempted. The engine calls it only when actuation FAILS after a
	// winning claim, so a transient actuator error doesn't permanently poison the
	// id (without it, claim-before-apply would make every failed apply un-retryable).
	// Releasing an absent key is a no-op.
	ReleaseIntent(ctx context.Context, intentID string) error
	AppendAudit(ctx context.Context, kind, detail string) (AuditEntry, error)
	Audit(ctx context.Context) ([]AuditEntry, error)
	VerifyAudit(ctx context.Context) bool
	ScheduleRevert(ctx context.Context, r PendingRevert) error
	DueReverts(ctx context.Context, now time.Time) ([]PendingRevert, error)
	MarkReverted(ctx context.Context, intentID string) error
	RecordAction(ctx context.Context, ruleSHA string, at time.Time) error
	CountActions(ctx context.Context, ruleSHA string, since time.Time) (int, error)
	RecordOutstanding(ctx context.Context, key string, r PendingRevert) error
	Outstanding(ctx context.Context, key string) ([]PendingRevert, error)
	ClearOutstanding(ctx context.Context, key string) error
	RecordUsage(ctx context.Context, tenant, model string, cost float64, at time.Time) error
	SpendSince(ctx context.Context, tenant string, since time.Time) (float64, error)
	PruneUsage(ctx context.Context, before time.Time) error
	Close() error
}
