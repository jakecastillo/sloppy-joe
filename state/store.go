package state

import "time"

// PendingRevert is a durable record of an applied, reversible intent awaiting TTL expiry.
type PendingRevert struct {
	IntentID string
	Kind     string
	Target   string
	ArgsJSON string
	DueAt    time.Time
}

// Store is the durable backend. SQLite for v0; Redis/Postgres later behind this same interface.
type Store interface {
	// IsIntentApplied reports whether an intent id was already applied (idempotency / crash-resume).
	IsIntentApplied(intentID string) (bool, error)
	// MarkIntentApplied records an intent id as applied (idempotent).
	MarkIntentApplied(intentID string) error
	// AppendAudit links a new entry onto the tamper-evident chain.
	AppendAudit(kind, detail string) (AuditEntry, error)
	// Audit returns all entries in order.
	Audit() ([]AuditEntry, error)
	// VerifyAudit recomputes and validates the whole chain.
	VerifyAudit() bool
	// ScheduleRevert durably records a reversible intent to revert at DueAt (idempotent on IntentID).
	ScheduleRevert(r PendingRevert) error
	// DueReverts returns scheduled reverts with DueAt <= now.
	DueReverts(now time.Time) ([]PendingRevert, error)
	// MarkReverted removes a pending revert once reverted.
	MarkReverted(intentID string) error
	// Close releases the backend.
	Close() error
}
