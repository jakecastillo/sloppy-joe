package core

import (
	"crypto/sha256"
	"encoding/hex"
)

// DeterministicID derives a stable id from parts (used for idempotency keys).
func DeterministicID(parts ...string) string {
	h := sha256.New()
	for _, p := range parts {
		h.Write([]byte(p))
		h.Write([]byte{0})
	}
	return hex.EncodeToString(h.Sum(nil))[:16]
}
