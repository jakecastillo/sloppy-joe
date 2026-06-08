package core

import "time"

// Outcome records the result of applying an intent.
type Outcome string

const (
	OutcomeApplied  Outcome = "applied"
	OutcomeReverted Outcome = "reverted"
	OutcomeFailed   Outcome = "failed"
)

// Receipt is the audited proof of an actuator action.
type Receipt struct {
	IntentID  string    `json:"intent_id"`
	Actuator  string    `json:"actuator"`
	AppliedAt time.Time `json:"applied_at"`
	Before    any       `json:"before,omitempty"`
	After     any       `json:"after,omitempty"`
	Outcome   Outcome   `json:"outcome"`
	Signature string    `json:"signature,omitempty"`
}
