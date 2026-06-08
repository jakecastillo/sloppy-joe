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

func signerFromPrivate(priv ed25519.PrivateKey) Signer {
	return &ed25519Signer{priv: priv, pub: priv.Public().(ed25519.PublicKey)}
}

// LoadOrCreateSigner loads a base64 ed25519 private key from path, or generates
// and persists one (0600) if absent — so signatures are stable across restarts.
func LoadOrCreateSigner(path string) (Signer, error) {
	if b, err := os.ReadFile(path); err == nil {
		raw, err := base64.StdEncoding.DecodeString(strings.TrimSpace(string(b)))
		if err != nil {
			return nil, fmt.Errorf("intent: bad key file %s: %w", path, err)
		}
		if len(raw) != ed25519.PrivateKeySize {
			return nil, fmt.Errorf("intent: key file %s has wrong size", path)
		}
		return signerFromPrivate(ed25519.PrivateKey(raw)), nil
	}
	_, priv, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		return nil, err
	}
	enc := base64.StdEncoding.EncodeToString(priv)
	if err := os.WriteFile(path, []byte(enc), 0o600); err != nil {
		return nil, err
	}
	return signerFromPrivate(priv), nil
}
