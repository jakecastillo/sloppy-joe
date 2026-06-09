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
	IsIntentApplied(ctx context.Context, intentID string) (bool, error)
	MarkIntentApplied(ctx context.Context, intentID string) error
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
	Close() error
}
