package intent

import (
	"testing"

	"github.com/sloppyjoe/sloppy/core"
)

func TestSignAndVerify(t *testing.T) {
	s, err := NewEd25519Signer()
	if err != nil {
		t.Fatalf("new signer: %v", err)
	}
	i := core.RemediationIntent{ID: "int-1", Kind: core.ActionRouteOverride, Target: "gpt-4o", RuleSHA: "sha"}
	sig := s.Sign(i.CanonicalBytes())
	if sig == "" {
		t.Fatal("empty signature")
	}
	if !s.Verify(i.CanonicalBytes(), sig) {
		t.Fatal("valid signature failed to verify")
	}
	if s.Verify([]byte("tampered"), sig) {
		t.Fatal("tampered payload verified — must fail")
	}
}
