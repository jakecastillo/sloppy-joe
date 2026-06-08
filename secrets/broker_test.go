package secrets

import "testing"

func TestEnvBrokerGetAndDeny(t *testing.T) {
	t.Setenv("SLOPPY_TOKEN_LITELLM", "admin-xyz")
	b := NewEnvBroker([]string{"litellm"})
	tok, err := b.Get("litellm")
	if err != nil || tok != "admin-xyz" {
		t.Fatalf("expected admin-xyz, got %q err=%v", tok, err)
	}
	if _, err := b.Get("github"); err == nil {
		t.Fatal("expected default-deny for unregistered capability")
	}
}
