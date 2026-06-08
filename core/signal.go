// Package core holds Sloppy Joe's shared vocabulary types.
package core

import "time"

// Severity classifies a Signal's operational urgency.
type Severity string

const (
	SeverityInfo     Severity = "info"
	SeverityWarning  Severity = "warning"
	SeverityCritical Severity = "critical"
)

// Subject identifies what a Signal is about.
type Subject struct {
	Tenant     string `json:"tenant,omitempty"`
	Deployment string `json:"deployment,omitempty"`
	Alias      string `json:"alias,omitempty"`
}

// Evidence is a pointer to supporting data (otel span link, metric snapshot).
type Evidence struct {
	Kind string `json:"kind"`
	Ref  string `json:"ref"`
}

// Signal is an OTel-GenAI event wrapped in a CloudEvents-shaped envelope.
type Signal struct {
	ID             string         `json:"id"`
	Source         string         `json:"source"`
	Type           string         `json:"type"`
	Time           time.Time      `json:"time"`
	Subject        Subject        `json:"subject"`
	Severity       Severity       `json:"severity"`
	CorrelationKey string         `json:"correlation_key"`
	DedupWindow    time.Duration  `json:"dedup_window"`
	Evidence       []Evidence     `json:"evidence,omitempty"`
	Data           map[string]any `json:"data,omitempty"`
}
