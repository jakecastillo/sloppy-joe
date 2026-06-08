package core

import "testing"

func TestIntentCanonicalBytesStable(t *testing.T) {
	i := RemediationIntent{
		ID:      "int-1",
		Kind:    ActionRouteOverride,
		Target:  "gpt-4o",
		Args:    map[string]any{"to": "ollama/llama3", "ttl": "30m"},
		RuleSHA: "abc123",
	}
	a := i.CanonicalBytes()
	b := i.CanonicalBytes()
	if string(a) != string(b) {
		t.Fatal("CanonicalBytes must be deterministic for signing")
	}
	if len(a) == 0 {
		t.Fatal("CanonicalBytes empty")
	}
	// Signature field must not affect canonical bytes.
	i.Signature = "sig"
	if string(i.CanonicalBytes()) != string(a) {
		t.Fatal("Signature must be excluded from CanonicalBytes")
	}
}
