// Package intent builds and signs RemediationIntents.
package intent

import (
	"crypto/ed25519"
	"crypto/rand"
	"encoding/base64"
	"fmt"
	"os"
	"strings"
)

// Signer signs and verifies canonical bytes.
type Signer interface {
	Sign(payload []byte) string
	Verify(payload []byte, sig string) bool
	// PublicKey returns the ed25519 public key a third party can use to verify
	// signatures produced by Sign — exported so `sloppy audit --verify-sigs`
	// can check persisted signatures without the private key.
	PublicKey() ed25519.PublicKey
}

type ed25519Signer struct {
	priv ed25519.PrivateKey
	pub  ed25519.PublicKey
}

// NewEd25519Signer generates an ephemeral keypair (v0; persisted keys + Sigstore later).
func NewEd25519Signer() (Signer, error) {
	pub, priv, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		return nil, err
	}
	return &ed25519Signer{priv: priv, pub: pub}, nil
}

func (s *ed25519Signer) Sign(payload []byte) string {
	return base64.StdEncoding.EncodeToString(ed25519.Sign(s.priv, payload))
}

func (s *ed25519Signer) Verify(payload []byte, sig string) bool {
	raw, err := base64.StdEncoding.DecodeString(sig)
	if err != nil {
		return false
	}
	return ed25519.Verify(s.pub, payload, raw)
}

// PublicKey returns the signer's ed25519 public key.
func (s *ed25519Signer) PublicKey() ed25519.PublicKey { return s.pub }

func signerFromPrivate(priv ed25519.PrivateKey) Signer {
	return &ed25519Signer{priv: priv, pub: priv.Public().(ed25519.PublicKey)}
}

// PublicKeyPath returns the conventional path of the exported public key for a
// given private-key path: "<path>.pub" (base64 ed25519 public key).
func PublicKeyPath(privPath string) string { return privPath + ".pub" }

// LoadOrCreateSigner loads a base64 ed25519 private key from path, or generates
// and persists one (0600) if absent — so signatures are stable across restarts.
// It also exports the public key to "<path>.pub" (base64, 0644) so a verify-only
// party can check signatures via `sloppy audit --verify-sigs`; if the private key
// exists but the .pub does not (e.g. a key created before pubkey export), the
// .pub is backfilled.
func LoadOrCreateSigner(path string) (Signer, error) {
	if b, err := os.ReadFile(path); err == nil {
		raw, err := base64.StdEncoding.DecodeString(strings.TrimSpace(string(b)))
		if err != nil {
			return nil, fmt.Errorf("intent: bad key file %s: %w", path, err)
		}
		if len(raw) != ed25519.PrivateKeySize {
			return nil, fmt.Errorf("intent: key file %s has wrong size", path)
		}
		priv := ed25519.PrivateKey(raw)
		if err := exportPublicKey(path, priv.Public().(ed25519.PublicKey)); err != nil {
			return nil, err
		}
		return signerFromPrivate(priv), nil
	}
	_, priv, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		return nil, err
	}
	enc := base64.StdEncoding.EncodeToString(priv)
	if err := os.WriteFile(path, []byte(enc), 0o600); err != nil {
		return nil, err
	}
	if err := exportPublicKey(path, priv.Public().(ed25519.PublicKey)); err != nil {
		return nil, err
	}
	return signerFromPrivate(priv), nil
}

// exportPublicKey writes the base64 public key to PublicKeyPath(path) if it is
// missing or differs, so the .pub always tracks the active private key.
func exportPublicKey(path string, pub ed25519.PublicKey) error {
	pubPath := PublicKeyPath(path)
	enc := base64.StdEncoding.EncodeToString(pub)
	if existing, err := os.ReadFile(pubPath); err == nil && strings.TrimSpace(string(existing)) == enc {
		return nil
	}
	return os.WriteFile(pubPath, []byte(enc), 0o644)
}

// LoadVerifierKey loads a base64 ed25519 public key from path for verify-only
// use (no private key required) — e.g. checking a peer's audit signatures.
func LoadVerifierKey(path string) (ed25519.PublicKey, error) {
	b, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("intent: read public key %s: %w", path, err)
	}
	raw, err := base64.StdEncoding.DecodeString(strings.TrimSpace(string(b)))
	if err != nil {
		return nil, fmt.Errorf("intent: bad public key %s: %w", path, err)
	}
	if len(raw) != ed25519.PublicKeySize {
		return nil, fmt.Errorf("intent: public key %s has wrong size", path)
	}
	return ed25519.PublicKey(raw), nil
}
