package config

import (
	"fmt"
	"regexp"
	"strings"
	"time"
)

// Problem is a single validation finding: a config path plus a human message.
type Problem struct {
	Path string
	Msg  string
}

func (p Problem) String() string { return fmt.Sprintf("%s: %s", p.Path, p.Msg) }

// knownPlatforms are the actuator/gateway/notifier names the bootstrap builder wires.
var knownPlatforms = map[string]bool{
	"litellm": true, "bifrost": true, "envoy": true, "github": true, "slack": true,
}

// secretPatterns match values that look like live credentials and must NEVER be
// inline in the Git-reviewed config — they belong behind token_env + the broker.
var secretPatterns = []*regexp.Regexp{
	regexp.MustCompile(`hooks\.slack\.com/services/`),   // Slack incoming webhook
	regexp.MustCompile(`xox[baprs]-`),                   // Slack tokens
	regexp.MustCompile(`gh[pousr]_[A-Za-z0-9]{20,}`),    // GitHub tokens
	regexp.MustCompile(`sk-[A-Za-z0-9]{16,}`),           // OpenAI-style keys
	regexp.MustCompile(`-----BEGIN [A-Z ]*PRIVATE KEY`), // PEM private key
}

func looksSecret(s string) bool {
	for _, re := range secretPatterns {
		if re.MatchString(s) {
			return true
		}
	}
	return false
}

// Validate checks a File for structural, enum, version, and secret-hygiene
// problems. An empty result means the file is valid. It never resolves secrets.
func Validate(f File) []Problem {
	var ps []Problem
	add := func(path, msg string) { ps = append(ps, Problem{Path: path, Msg: msg}) }

	if f.Version != 1 {
		add("version", fmt.Sprintf("unsupported version %d (want 1)", f.Version))
	}

	switch f.Store.Kind {
	case "sqlite":
	case "redis":
		if f.Store.RedisAddr == "" {
			add("store.redis_addr", "required when store.kind=redis")
		}
	case "":
		add("store.kind", "required (sqlite|redis)")
	default:
		add("store.kind", fmt.Sprintf("unknown store %q (want sqlite|redis)", f.Store.Kind))
	}

	if f.Server.RevertInterval != "" {
		if _, err := time.ParseDuration(f.Server.RevertInterval); err != nil {
			add("server.revert_interval", fmt.Sprintf("bad duration %q", f.Server.RevertInterval))
		}
	}

	switch f.Engine.LogFormat {
	case "", "text", "json":
	default:
		add("engine.log_format", fmt.Sprintf("unknown %q (want text|json)", f.Engine.LogFormat))
	}

	for name, v := range map[string]string{
		"engine.fail_mode.default": f.Engine.FailMode.Default,
		"engine.fail_mode.notify":  f.Engine.FailMode.Notify,
	} {
		switch v {
		case "", "open", "closed":
		default:
			add(name, fmt.Sprintf("unknown %q (want open|closed)", v))
		}
	}

	if f.Auth.Enabled && f.Auth.KeysEnv == "" {
		add("auth.keys_env", "required when auth.enabled=true")
	}

	for name, p := range f.Platforms {
		base := "platforms." + name
		if !knownPlatforms[name] {
			add(base, fmt.Sprintf("unknown platform %q (want litellm|bifrost|envoy|github|slack)", name))
		}
		// Secret hygiene: non-secret identifier fields must not carry inline credentials.
		for field, val := range map[string]string{
			base + ".url":      p.URL,
			base + ".base_url": p.BaseURL,
			base + ".repo":     p.Repo,
			base + ".channel":  p.Channel,
		} {
			if looksSecret(val) {
				add(field, "looks like an inline secret; reference it via token_env + SLOPPY_TOKEN_* instead")
			}
		}
		if looksSecret(p.TokenEnv) {
			add(base+".token_env", "must be an env-var NAME, not an inline secret value")
		} else if p.Enabled && p.TokenEnv != "" && !strings.HasPrefix(p.TokenEnv, "SLOPPY_TOKEN_") {
			add(base+".token_env", fmt.Sprintf("%q should name a SLOPPY_TOKEN_* broker capability", p.TokenEnv))
		}
	}
	return ps
}
