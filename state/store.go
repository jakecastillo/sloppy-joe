package state

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
	// Close releases the backend.
	Close() error
}
