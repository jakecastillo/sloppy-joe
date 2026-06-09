package secrets

import (
	"os"
	"path/filepath"
	"testing"
)

func TestEnvBrokerFileBackedPrecedence(t *testing.T) {
	dir := t.TempDir()
	p := filepath.Join(dir, "tok")
	if err := os.WriteFile(p, []byte("  file-secret\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	// Both set: the *_FILE form wins and is trimmed.
	t.Setenv("SLOPPY_TOKEN_LITELLM", "env-secret")
	t.Setenv("SLOPPY_TOKEN_LITELLM_FILE", p)
	b := NewEnvBroker([]string{"litellm"})
	got, err := b.Get("litellm")
	if err != nil {
		t.Fatal(err)
	}
	if got != "file-secret" {
		t.Fatalf("want trimmed file-backed secret, got %q", got)
	}
}

func TestEnvBrokerFileMissing(t *testing.T) {
	t.Setenv("SLOPPY_TOKEN_LITELLM_FILE", filepath.Join(t.TempDir(), "nope"))
	b := NewEnvBroker([]string{"litellm"})
	if _, err := b.Get("litellm"); err == nil {
		t.Fatal("missing *_FILE path should error")
	}
}
