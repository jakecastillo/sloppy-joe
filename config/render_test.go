package config

import (
	"bytes"
	"strings"
	"testing"
)

func TestRenderEffectiveDoesNotLeakTokens(t *testing.T) {
	f := Defaults()
	f.Platforms = map[string]Platform{
		"litellm": {Enabled: true, URL: "http://localhost:4000", TokenEnv: "SLOPPY_TOKEN_LITELLM"},
	}
	// Even with a secret in the environment, show must never resolve/print it.
	getenv := func(k string) string {
		if k == "SLOPPY_TOKEN_LITELLM" {
			return "super-secret-token"
		}
		return ""
	}
	eff := Resolve(f, true, FlagOverrides{}, getenv)
	var buf bytes.Buffer
	if err := RenderEffective(&buf, eff, false); err != nil {
		t.Fatal(err)
	}
	out := buf.String()
	if strings.Contains(out, "super-secret-token") {
		t.Fatalf("render leaked a resolved token:\n%s", out)
	}
	if !strings.Contains(out, "SLOPPY_TOKEN_LITELLM") {
		t.Fatalf("render should show the token_env NAME:\n%s", out)
	}
}

func TestRenderProvenanceHighlightsOverrides(t *testing.T) {
	getenv := func(k string) string {
		if k == "SLOPPY_LITELLM_URL" {
			return "http://gw:4000"
		}
		return ""
	}
	eff := Resolve(Defaults(), true, FlagOverrides{}, getenv)
	var buf bytes.Buffer
	if err := RenderEffective(&buf, eff, true); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(buf.String(), "overrides file") {
		t.Fatalf("provenance should highlight the env override:\n%s", buf.String())
	}
}
