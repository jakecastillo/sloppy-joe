package doctor

import (
	"strings"
	"testing"

	"github.com/sloppyjoe/sloppy/config"
)

func TestCheckPlatformsNoneEnabled(t *testing.T) {
	c := CheckPlatforms(config.Resolve(config.Defaults(), false, config.FlagOverrides{}, func(string) string { return "" }))
	if !c.OK {
		t.Fatalf("no platforms should be OK: %+v", c)
	}
}

func TestCheckPlatformsMissingToken(t *testing.T) {
	eff := config.Resolve(config.File{Platforms: map[string]config.Platform{
		"slack": {Enabled: true, Channel: "#ops", TokenEnv: "SLOPPY_TOKEN_SLACK_ABSENT"},
	}}, true, config.FlagOverrides{}, func(string) string { return "" })
	c := CheckPlatforms(eff)
	if c.OK || !strings.Contains(c.Detail, "slack") {
		t.Fatalf("missing token should fail and name slack: %+v", c)
	}
	// The token-env NAME may appear; an actual token VALUE must never be printed.
}

func TestCheckPlatformsTokenPresent(t *testing.T) {
	t.Setenv("SLOPPY_TOKEN_SLACK", "whatever")
	eff := config.Resolve(config.File{Platforms: map[string]config.Platform{
		"slack": {Enabled: true, Channel: "#ops", TokenEnv: "SLOPPY_TOKEN_SLACK"},
	}}, true, config.FlagOverrides{}, func(string) string { return "" })
	c := CheckPlatforms(eff)
	if !c.OK || strings.Contains(c.Detail, "whatever") {
		t.Fatalf("present token should be OK and never print the value: %+v", c)
	}
}
