package intent

import (
	"crypto/ed25519"
	"encoding/base64"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/sloppyjoe/sloppy/core"
)

type pubKeyer interface{ PublicKey() ed25519.PublicKey }

// PublicKey() must return the key that matches the private key used to sign:
// a signature produced by the signer must verify under its reported public key.
func TestPublicKeyMatchesSigner(t *testing.T) {
	s, err := NewEd25519Signer()
	if err != nil {
		t.Fatalf("new signer: %v", err)
	}
	pk, ok := s.(pubKeyer)
	if !ok {
		t.Fatal("Signer must expose PublicKey() ed25519.PublicKey")
	}
	pub := pk.PublicKey()
	if len(pub) != ed25519.PublicKeySize {
		t.Fatalf("PublicKey size = %d, want %d", len(pub), ed25519.PublicKeySize)
	}
	i := core.RemediationIntent{ID: "int-1", Kind: core.ActionRouteOverride, Target: "gpt-4o", RuleSHA: "sha"}
	raw := mustDecode(t, s.Sign(i.CanonicalBytes()))
	if !ed25519.Verify(pub, i.CanonicalBytes(), raw) {
		t.Fatal("PublicKey() must verify a signature made by the same signer")
	}
}

// LoadOrCreateSigner must persist the public key beside the private key so a
// verify-only party can load it; the loaded key must match PublicKey().
func TestLoadOrCreatePersistsPublicKey(t *testing.T) {
	dir := t.TempDir()
	keyPath := filepath.Join(dir, "sloppy.key")
	s, err := LoadOrCreateSigner(keyPath)
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	pubPath := keyPath + ".pub"
	if _, err := os.Stat(pubPath); err != nil {
		t.Fatalf("public key not persisted at %s: %v", pubPath, err)
	}
	pub, err := LoadVerifierKey(pubPath)
	if err != nil {
		t.Fatalf("load verifier key: %v", err)
	}
	want := s.(pubKeyer).PublicKey()
	if string(pub) != string(want) {
		t.Fatal("persisted public key must match the signer's public key")
	}
	// A verify-only holder of the public key can verify a real signature.
	i := core.RemediationIntent{ID: "i", Kind: core.ActionPage, Target: "oncall", RuleSHA: "s"}
	raw := mustDecode(t, s.Sign(i.CanonicalBytes()))
	if !ed25519.Verify(pub, i.CanonicalBytes(), raw) {
		t.Fatal("verify-only public key failed to verify a real signature")
	}
}

// Loading an existing private key that predates pubkey-export must backfill the
// .pub file so verification works for keys created before this change.
func TestLoadBackfillsMissingPublicKey(t *testing.T) {
	dir := t.TempDir()
	keyPath := filepath.Join(dir, "sloppy.key")
	if _, err := LoadOrCreateSigner(keyPath); err != nil {
		t.Fatalf("create: %v", err)
	}
	pubPath := keyPath + ".pub"
	if err := os.Remove(pubPath); err != nil {
		t.Fatalf("remove pub: %v", err)
	}
	// Re-load: the private key exists, the .pub does not — it must be recreated.
	if _, err := LoadOrCreateSigner(keyPath); err != nil {
		t.Fatalf("reload: %v", err)
	}
	if _, err := os.Stat(pubPath); err != nil {
		t.Fatalf("public key not backfilled on load: %v", err)
	}
}

// AppliedAuditDetail must embed enough to recompute the signed bytes and the
// full (non-truncated) signature, and verify clean against the public key.
func TestAuditDetailRoundTripsAndVerifies(t *testing.T) {
	s, err := NewEd25519Signer()
	if err != nil {
		t.Fatalf("new signer: %v", err)
	}
	pub := s.(pubKeyer).PublicKey()
	i := core.RemediationIntent{
		ID: "int-7", Kind: core.ActionThrottleTenant, Target: "acme",
		Args: map[string]any{"to": "ollama", "limit": 5}, RuleSHA: "deadbeef",
	}
	i.Signature = s.Sign(i.CanonicalBytes())

	detail := AppliedAuditDetail(i)
	// The full signature must survive — not a 12-char short() prefix.
	if !strings.Contains(detail, i.Signature) {
		t.Fatalf("detail must persist the FULL signature, got %q", detail)
	}
	ok, found := VerifyAuditDetail(pub, detail)
	if !found {
		t.Fatal("VerifyAuditDetail must find a signature in an applied detail")
	}
	if !ok {
		t.Fatal("a clean applied detail must verify")
	}
}

// Tampering any persisted intent field (here the canonical representation in the
// detail) must make verification fail — the signature no longer matches.
func TestTamperedAuditDetailFailsVerify(t *testing.T) {
	s, err := NewEd25519Signer()
	if err != nil {
		t.Fatalf("new signer: %v", err)
	}
	pub := s.(pubKeyer).PublicKey()
	i := core.RemediationIntent{ID: "int-9", Kind: core.ActionRouteOverride, Target: "gpt-4o", RuleSHA: "sha"}
	i.Signature = s.Sign(i.CanonicalBytes())
	detail := AppliedAuditDetail(i)

	// Forge: re-encode the canonical payload with a changed target, leaving the
	// original signature in place — exactly what an attacker editing the store would do.
	forged := i
	forged.Target = "cheapo-model"
	tampered := strings.Replace(detail,
		base64.StdEncoding.EncodeToString(i.CanonicalBytes()),
		base64.StdEncoding.EncodeToString(forged.CanonicalBytes()), 1)
	if tampered == detail {
		t.Fatal("test setup: tamper did not change the detail")
	}
	ok, found := VerifyAuditDetail(pub, tampered)
	if !found {
		t.Fatal("tampered detail still parses a signature")
	}
	if ok {
		t.Fatal("a tampered intent field must FAIL signature verification")
	}
}

// A detail that carries no signature (e.g. a non-applied audit kind) reports
// found=false so the verifier can skip it rather than count it as a failure.
func TestVerifyAuditDetailNoSignature(t *testing.T) {
	pub, _, err := ed25519.GenerateKey(strings.NewReader("0123456789abcdef0123456789abcdef"))
	if err != nil {
		t.Fatalf("genkey: %v", err)
	}
	if _, found := VerifyAuditDetail(pub, "route_override target=x rule=y"); found {
		t.Fatal("a detail without a signature must report found=false")
	}
}

func mustDecode(t *testing.T, b64 string) []byte {
	t.Helper()
	raw, err := base64.StdEncoding.DecodeString(b64)
	if err != nil {
		t.Fatalf("decode sig: %v", err)
	}
	return raw
}
