package engine

import "testing"

// TestDedupKeyEncoding pins the single (rule, correlation) key encoding shared by
// the `for:` window, the on-clear outstanding set, and rollback lookups. The three
// sites must agree byte-for-byte or a rule's pending window and its recorded
// outstanding set would key differently and never reconcile.
func TestDedupKeyEncoding(t *testing.T) {
	if got := dedupKey("sha9", "acme:cost"); got != "sha9|acme:cost" {
		t.Fatalf("dedupKey = %q, want %q", got, "sha9|acme:cost")
	}
	// The encoding is stable and order-sensitive in its two arguments (rule SHAs
	// are hex hashes that never contain the '|' delimiter, so this is unambiguous
	// in practice).
	if dedupKey("rule", "corr") == dedupKey("corr", "rule") {
		t.Fatal("dedupKey must be order-sensitive in (rule, corr)")
	}
}
