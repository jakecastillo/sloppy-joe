package config

import (
	"bytes"
	"fmt"
	"io"
	"os"

	"gopkg.in/yaml.v3"
)

// File is the on-disk sloppy.yaml schema: the single declarative source of truth
// for both the sloppy CLI and the sloppyd daemon. It is hand-edited and
// Git-reviewed; the CLI never writes it (except one-time `sloppy init`).
type File struct {
	Version   int                     `yaml:"version"`
	Server    ServerConfig            `yaml:"server"`
	Store     StoreConfig             `yaml:"store"`
	Engine    EngineConfig            `yaml:"engine"`
	Auth      AuthConfig              `yaml:"auth"`
	Rules     []string                `yaml:"rules"`
	Platforms map[string]Platform     `yaml:"platforms,omitempty"`
	Recipes   map[string]RecipeConfig `yaml:"recipes,omitempty"`
}

// ServerConfig holds sloppyd-only listen/loop knobs. Durations are strings here
// (e.g. "30s") and parsed in the Effective layer.
type ServerConfig struct {
	Addr           string `yaml:"addr"`
	RevertInterval string `yaml:"revert_interval"`
}

// StoreConfig selects the state backend.
type StoreConfig struct {
	Kind      string `yaml:"kind"`       // sqlite | redis
	Path      string `yaml:"path"`       // sqlite db path
	RedisAddr string `yaml:"redis_addr"` // host:port when kind=redis
}

// EngineConfig holds engine-level knobs.
type EngineConfig struct {
	SigningKey string         `yaml:"signing_key"`
	LogFormat  string         `yaml:"log_format"` // text | json
	Pricebook  string         `yaml:"pricebook"`  // optional path
	FailMode   FailModeConfig `yaml:"fail_mode"`
}

// FailModeConfig sets per-capability fail behavior on state-store errors:
// `default` applies to mutating actions, `notify` to open_issue/page. open|closed.
type FailModeConfig struct {
	Default string `yaml:"default"`
	Notify  string `yaml:"notify"`
}

// AuthConfig toggles API-key RBAC; keys come from keys_env (never inline).
type AuthConfig struct {
	Enabled bool   `yaml:"enabled"`
	KeysEnv string `yaml:"keys_env"`
}

// Platform configures one actuator/gateway/notifier. Non-secret identifiers may be
// inline; bearer-equivalent secrets (tokens, Slack webhook URLs) are ALWAYS a
// token_env reference resolved by the SLOPPY_TOKEN_* broker, never inline.
type Platform struct {
	Enabled      bool   `yaml:"enabled"`
	Experimental bool   `yaml:"experimental,omitempty"`
	URL          string `yaml:"url,omitempty"`
	BaseURL      string `yaml:"base_url,omitempty"`
	Repo         string `yaml:"repo,omitempty"`
	Channel      string `yaml:"channel,omitempty"`
	TokenEnv     string `yaml:"token_env,omitempty"`
}

// RecipeConfig enables a recipe and carries its typed params inline. Deep param
// validation lives in the recipe package (Phase C); here Params is captured raw.
type RecipeConfig struct {
	Enabled bool           `yaml:"enabled"`
	Params  map[string]any `yaml:",inline"`
}

// LoadFile reads sloppy.yaml from path. A missing file yields the built-in
// Defaults with existed=false (zero-config). Unknown keys are rejected so typos in
// a hand-edited file surface immediately.
func LoadFile(path string) (File, bool, error) {
	b, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return Defaults(), false, nil
		}
		return File{}, false, err
	}
	var f File
	dec := yaml.NewDecoder(bytes.NewReader(b))
	dec.KnownFields(true)
	if err := dec.Decode(&f); err != nil && err != io.EOF {
		return File{}, true, fmt.Errorf("config: %s: %w", path, err)
	}
	applyDefaults(&f)
	return f, true, nil
}

// Defaults returns the built-in configuration: today's zero-config behavior plus
// the fail-closed-for-mutating policy.
func Defaults() File {
	var f File
	applyDefaults(&f)
	return f
}

// applyDefaults fills zero-valued fields with built-in defaults, in place.
func applyDefaults(f *File) {
	if f.Version == 0 {
		f.Version = 1
	}
	if f.Server.Addr == "" {
		f.Server.Addr = ":8723"
	}
	if f.Server.RevertInterval == "" {
		f.Server.RevertInterval = "30s"
	}
	if f.Store.Kind == "" {
		f.Store.Kind = "sqlite"
	}
	if f.Store.Path == "" {
		f.Store.Path = "sloppy.db"
	}
	if f.Engine.SigningKey == "" {
		f.Engine.SigningKey = "sloppy.key"
	}
	if f.Engine.LogFormat == "" {
		f.Engine.LogFormat = "text"
	}
	if f.Engine.FailMode.Default == "" {
		// Mutating gateway actions (route_override/throttle_tenant/disable_deployment)
		// fail CLOSED by default: on a state-store error the riskiest moment must not
		// bypass dedup/budget/audit. Notify stays open (best-effort). This is a
		// behavior change vs the pre-config daemon, gated in the CHANGELOG.
		f.Engine.FailMode.Default = "closed"
	}
	if f.Engine.FailMode.Notify == "" {
		f.Engine.FailMode.Notify = "open"
	}
	if f.Auth.KeysEnv == "" {
		f.Auth.KeysEnv = "SLOPPY_API_KEYS"
	}
	if len(f.Rules) == 0 {
		f.Rules = []string{"rules"}
	}
}
