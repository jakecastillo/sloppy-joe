package intent

import (
	"crypto/ed25519"
	"encoding/base64"
	"fmt"
	"strings"

	"github.com/sloppyjoe/sloppy/core"
)

// Audit-detail field markers. The applied-audit detail is a single space-free-key
// line so it stays human-readable in `sloppy audit` while remaining machine-
// verifiable. `canon` is the base64 of the exact bytes that were signed
// (Intent.CanonicalBytes), and `sig` is the FULL base64 signature (no truncation),
// so a verifier can check sig against canon under the persisted public key.
const (
	canonField = "canon="
	sigField   = "sig="
)

// AppliedAuditDetail formats the audit detail for an applied intent so the
// signature is independently verifiable later. It embeds the canonical signed
// bytes and the full signature alongside the human-readable summary. Any later
// edit to a persisted intent field changes the canonical bytes and thus breaks
// verification against the original signature.
func AppliedAuditDetail(i core.RemediationIntent) string {
	canon := base64.StdEncoding.EncodeToString(i.CanonicalBytes())
	return fmt.Sprintf("%s target=%s rule=%s %s%s %s%s",
		i.Kind, i.Target, i.RuleSHA, canonField, canon, sigField, i.Signature)
}

// VerifyAuditDetail extracts the canonical bytes and signature from an audit
// detail produced by AppliedAuditDetail and verifies the signature under pub.
// found reports whether the detail carried a verifiable signature at all (so a
// caller can skip non-signed audit kinds rather than count them as failures).
func VerifyAuditDetail(pub ed25519.PublicKey, detail string) (ok bool, found bool) {
	canonB64, hasCanon := field(detail, canonField)
	sigB64, hasSig := field(detail, sigField)
	if !hasCanon || !hasSig {
		return false, false
	}
	canon, err := base64.StdEncoding.DecodeString(canonB64)
	if err != nil {
		return false, true
	}
	sig, err := base64.StdEncoding.DecodeString(sigB64)
	if err != nil {
		return false, true
	}
	return ed25519.Verify(pub, canon, sig), true
}

// field returns the whitespace-delimited token following marker (e.g. "sig=")
// in detail, and whether the marker was present.
func field(detail, marker string) (string, bool) {
	idx := strings.Index(detail, marker)
	if idx < 0 {
		return "", false
	}
	rest := detail[idx+len(marker):]
	if sp := strings.IndexByte(rest, ' '); sp >= 0 {
		rest = rest[:sp]
	}
	return rest, true
}
