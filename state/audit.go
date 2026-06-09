// Package state holds Sloppy Joe's durable control-plane state.
package state

import (
	"crypto/ed25519"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"time"
)

// AuditEntry is one record in the tamper-evident audit chain.
type AuditEntry struct {
	Seq      int       `json:"seq"`
	TS       time.Time `json:"ts"`
	Kind     string    `json:"kind"`
	Detail   string    `json:"detail"`
	PrevHash string    `json:"prev_hash"`
	Hash     string    `json:"hash"`
}

// ChainHash is the single canonical hash function for the audit chain.
// Used by both append and verify so the two can never silently diverge.
func ChainHash(ts, kind, detail, prev string) string {
	h := sha256.New()
	h.Write([]byte(ts))
	h.Write([]byte{0})
	h.Write([]byte(kind))
	h.Write([]byte{0})
	h.Write([]byte(detail))
	h.Write([]byte{0})
	h.Write([]byte(prev))
	return hex.EncodeToString(h.Sum(nil))
}

// VerifyChain validates a full, ordered audit slice against the hash chain.
// Backend-agnostic so SQLite and Redis can never drift in how they verify.
//
// LIMITATION: VerifyChain has no length anchor. Any valid prefix of a chain is
// itself a valid chain, so truncation, full deletion (empty slice), and a freshly
// re-chained wholesale replacement all PASS here. Detecting those requires the
// signed Checkpoint compared against the recomputed head — see VerifyAgainstCheckpoint.
func VerifyChain(es []AuditEntry) bool {
	prev := ""
	for _, e := range es {
		ts := e.TS.UTC().Format(time.RFC3339Nano)
		if e.PrevHash != prev || e.Hash != ChainHash(ts, e.Kind, e.Detail, prev) {
			return false
		}
		prev = e.Hash
	}
	return true
}

// CheckpointSigner signs canonical checkpoint bytes and exposes the public key
// that verifies them. It is satisfied structurally by intent.Signer, so the
// store can be handed the production signer without importing intent (which would
// create a state->intent->core dependency the store doesn't otherwise need).
type CheckpointSigner interface {
	Sign(payload []byte) string
	PublicKey() ed25519.PublicKey
}

// Checkpoint is the signed length+head anchor over the audit chain. It is what
// turns the chain from "tamper-evident against edits" into "tamper-evident against
// truncation/deletion/replacement too": a writer who drops or rewrites entries
// without re-signing the checkpoint (which needs the signing key) is detected.
type Checkpoint struct {
	Count    int    `json:"count"`     // number of audit entries the chain had
	HeadHash string `json:"head_hash"` // hash of the last entry ("" iff Count==0)
	Sig      string `json:"sig"`       // base64 signature over CheckpointPayload
	PubKey   string `json:"pub_key"`   // base64 ed25519 public key the sig verifies under
}

// CheckpointPayload is the canonical, backend-agnostic byte string that gets
// signed: count and head hash separated by a NUL (same framing discipline as
// ChainHash). Both backends MUST sign and verify these exact bytes so SQLite and
// Redis checkpoints can never silently diverge.
func CheckpointPayload(count int, headHash string) []byte {
	h := sha256.New()
	h.Write([]byte("sloppy-audit-checkpoint-v1"))
	h.Write([]byte{0})
	h.Write([]byte(itoa(count)))
	h.Write([]byte{0})
	h.Write([]byte(headHash))
	return h.Sum(nil)
}

// itoa is a tiny dependency-free int->decimal (avoids pulling strconv into the
// canonical-payload path for a single conversion).
func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	neg := n < 0
	if neg {
		n = -n
	}
	var buf [20]byte
	i := len(buf)
	for n > 0 {
		i--
		buf[i] = byte('0' + n%10)
		n /= 10
	}
	if neg {
		i--
		buf[i] = '-'
	}
	return string(buf[i:])
}

// MakeCheckpoint builds a signed checkpoint for the given entry count and head
// hash using signer. Backends call this after a successful append (or on demand)
// and persist the result alongside the chain.
func MakeCheckpoint(signer CheckpointSigner, count int, headHash string) Checkpoint {
	return Checkpoint{
		Count:    count,
		HeadHash: headHash,
		Sig:      signer.Sign(CheckpointPayload(count, headHash)),
		PubKey:   base64.StdEncoding.EncodeToString(signer.PublicKey()),
	}
}

// VerifyAgainstCheckpoint is the backend-agnostic honest tamper check. Given the
// full ordered chain and the persisted checkpoint (cpFound reports whether one
// exists), it returns true only if:
//   - the chain links verify (VerifyChain), AND
//   - a checkpoint exists when one is required (requireCheckpoint is true for a
//     checkpoint-enabled store with entries; stripping it then reads as tamper), AND
//   - the recomputed count and head hash equal the checkpoint's (fewer rows =>
//     truncation; different head => replacement), AND
//   - the checkpoint signature verifies under its persisted public key.
//
// requireCheckpoint lets a store that was NEVER given a signer keep the legacy
// chain-only guarantee (backward compatible), while a checkpoint-enabled store
// treats a missing checkpoint over a non-empty chain as tampering. When a
// checkpoint IS present it is always verified, regardless of requireCheckpoint.
func VerifyAgainstCheckpoint(es []AuditEntry, cp Checkpoint, cpFound, requireCheckpoint bool) bool {
	if !VerifyChain(es) {
		return false
	}
	if !cpFound {
		// No checkpoint persisted. A checkpoint-enabled store with entries means
		// the anchor was stripped => tamper. Otherwise (legacy/no-signer store, or
		// a genuinely fresh empty store) fall back to chain-only.
		if requireCheckpoint && len(es) > 0 {
			return false
		}
		return true
	}
	head := ""
	if len(es) > 0 {
		head = es[len(es)-1].Hash
	}
	if cp.Count != len(es) || cp.HeadHash != head {
		return false
	}
	pub, err := base64.StdEncoding.DecodeString(cp.PubKey)
	if err != nil || len(pub) != ed25519.PublicKeySize {
		return false
	}
	sig, err := base64.StdEncoding.DecodeString(cp.Sig)
	if err != nil {
		return false
	}
	return ed25519.Verify(ed25519.PublicKey(pub), CheckpointPayload(cp.Count, cp.HeadHash), sig)
}
