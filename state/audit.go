// Package state holds Sloppy Joe's durable control-plane state.
package state

import (
	"crypto/sha256"
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
