package core

import (
	"encoding/json"
	"testing"
	"time"
)

func TestSignalJSONRoundTrip(t *testing.T) {
	s := Signal{
		ID:             "evt-1",
		Source:         "litellm/otel",
		Type:           "cost.budget_burn",
		Time:           time.Unix(1749340800, 0).UTC(),
		Subject:        Subject{Tenant: "acme", Alias: "gpt-4o"},
		Severity:       SeverityCritical,
		CorrelationKey: "acme:cost",
		DedupWindow:    5 * time.Minute,
		Data:           map[string]any{"spend_1h_usd": 7.5},
	}
	b, err := json.Marshal(s)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	var got Signal
	if err := json.Unmarshal(b, &got); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if got.Type != "cost.budget_burn" || got.Subject.Tenant != "acme" {
		t.Fatalf("round-trip mismatch: %+v", got)
	}
	if got.Data["spend_1h_usd"].(float64) != 7.5 {
		t.Fatalf("data lost: %+v", got.Data)
	}
}
