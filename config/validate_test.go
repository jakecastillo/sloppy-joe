package config

import (
	"strings"
	"testing"
)

func goodFile() File {
	f := Defaults()
	f.Platforms = map[string]Platform{
		"litellm": {Enabled: true, URL: "http://localhost:4000", TokenEnv: "SLOPPY_TOKEN_LITELLM"},
	}
	return f
}

func hasPath(ps []Problem, path string) bool {
	for _, p := range ps {
		if p.Path == path {
			return true
		}
	}
	return false
}

func TestValidateClean(t *testing.T) {
	if ps := Validate(goodFile()); len(ps) != 0 {
		t.Fatalf("clean file had problems: %v", ps)
	}
}

func TestValidateStoreAndVersion(t *testing.T) {
	f := goodFile()
	f.Version = 2
	f.Store.Kind = "mongo"
	ps := Validate(f)
	if !hasPath(ps, "version") || !hasPath(ps, "store.kind") {
		t.Fatalf("want version+store.kind problems, got %v", ps)
	}
}

func TestValidateRedisNeedsAddr(t *testing.T) {
	f := goodFile()
	f.Store.Kind = "redis"
	if !hasPath(Validate(f), "store.redis_addr") {
		t.Fatal("redis without addr should be a problem")
	}
}

func TestValidateBadLogFormatAndFailMode(t *testing.T) {
	f := goodFile()
	f.Engine.LogFormat = "xml"
	f.Engine.FailMode.Default = "maybe"
	ps := Validate(f)
	if !hasPath(ps, "engine.log_format") || !hasPath(ps, "engine.fail_mode.default") {
		t.Fatalf("want log_format+fail_mode problems, got %v", ps)
	}
}

func TestValidateRejectsInlineSlackWebhook(t *testing.T) {
	f := goodFile()
	f.Platforms["slack"] = Platform{
		Enabled:  true,
		Channel:  "https://hooks.slack.com/services/T000/B000/xxxxxxxx",
		TokenEnv: "SLOPPY_TOKEN_SLACK",
	}
	if !hasPath(Validate(f), "platforms.slack.channel") {
		t.Fatalf("inline slack webhook should be rejected: %v", Validate(f))
	}
}

func TestValidateTokenEnvMustBeName(t *testing.T) {
	f := goodFile()
	f.Platforms["github"] = Platform{
		Enabled:  true,
		Repo:     "o/r",
		TokenEnv: "ghp_abcdefghijklmnopqrstuvwxyz0123456789",
	}
	if !hasPath(Validate(f), "platforms.github.token_env") {
		t.Fatalf("inline secret in token_env should be rejected: %v", Validate(f))
	}
}

func TestValidateUnknownPlatform(t *testing.T) {
	f := goodFile()
	f.Platforms["pagerduty"] = Platform{Enabled: true}
	if !hasPath(Validate(f), "platforms.pagerduty") {
		t.Fatal("unknown platform should be flagged")
	}
}

func TestProblemString(t *testing.T) {
	p := Problem{Path: "x.y", Msg: "bad"}
	if s := p.String(); !strings.Contains(s, "x.y") || !strings.Contains(s, "bad") {
		t.Fatalf("Problem.String: %q", s)
	}
}
