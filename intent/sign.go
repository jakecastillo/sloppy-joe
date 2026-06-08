// Package intent builds and signs RemediationIntents.
package intent

import (
	"crypto/ed25519"
	"crypto/rand"
	"encoding/base64"
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
