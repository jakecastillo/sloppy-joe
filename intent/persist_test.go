package intent

import (
	"path/filepath"
	"testing"

	"github.com/sloppyjoe/sloppy/core"
)

func TestPersistedKeyStableAcrossLoads(t *testing.T) {
	path := filepath.Join(t.TempDir(), "signing.key")
	s1, err := LoadOrCreateSigner(path)
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	s2, err := LoadOrCreateSigner(path) // should load the same key
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	i := core.RemediationIntent{ID: "i", Kind: core.ActionRouteOverride, Target: "m", RuleSHA: "s"}
	sig := s1.Sign(i.CanonicalBytes())
	if !s2.Verify(i.CanonicalBytes(), sig) {
		t.Fatal("a persisted key must verify a signature made by a prior load")
	}
}
