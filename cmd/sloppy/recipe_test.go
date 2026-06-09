package main

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestRecipeListAndShow(t *testing.T) {
	var out bytes.Buffer
	if rc := run([]string{"recipe", "list"}, &out); rc != 0 {
		t.Fatalf("recipe list rc=%d", rc)
	}
	if !strings.Contains(out.String(), "cost-guard") {
		t.Fatalf("recipe list should include cost-guard:\n%s", out.String())
	}

	out.Reset()
	if rc := run([]string{"recipe", "show", "cost-guard"}, &out); rc != 0 {
		t.Fatalf("recipe show rc=%d out=%s", rc, out.String())
	}
	s := out.String()
	if !strings.Contains(s, "route_override") || !strings.Contains(s, "rendered rule sha:") {
		t.Fatalf("recipe show output unexpected:\n%s", s)
	}
}

func TestConfigValidateCatchesBadRecipe(t *testing.T) {
	dir := t.TempDir()
	cfg := filepath.Join(dir, "sloppy.yaml")
	// cost-guard with a non-positive threshold should fail recipe validation.
	content := "version: 1\n" +
		"recipes:\n" +
		"  cost-guard: { enabled: true, threshold_usd_1h: 0 }\n"
	if err := os.WriteFile(cfg, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
	var out bytes.Buffer
	if rc := run([]string{"config", "validate", "--config", cfg}, &out); rc == 0 {
		t.Fatalf("bad recipe param should fail validate:\n%s", out.String())
	}
	if !strings.Contains(out.String(), "recipe") {
		t.Fatalf("expected a recipe problem:\n%s", out.String())
	}
}
