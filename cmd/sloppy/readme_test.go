package main

import (
	"os"
	"path/filepath"
	"regexp"
	"testing"
)

// TestReadmeCommandsAllResolve guards against the README documenting a `sloppy`
// subcommand that does not exist (the `sloppy up` drift this work fixed). It scans
// inline-backticked `sloppy <cmd>` references and asserts each is a real command.
func TestReadmeCommandsAllResolve(t *testing.T) {
	b, err := os.ReadFile(filepath.Join("..", "..", "README.md"))
	if err != nil {
		t.Skipf("README not found: %v", err)
	}
	known := map[string]bool{
		"init": true, "version": true, "inject": true, "rules": true, "audit": true,
		"test": true, "doctor": true, "config": true, "platform": true, "recipe": true,
		"report": true,
	}
	re := regexp.MustCompile("`sloppy ([a-z]+)")
	for _, m := range re.FindAllStringSubmatch(string(b), -1) {
		if !known[m[1]] {
			t.Errorf("README documents `sloppy %s`, which is not a known command", m[1])
		}
	}
}
