package config

import (
	"sort"
	"time"
)

// Source records where an effective value came from. Precedence, highest first:
// flag > env > file > default.
type Source string

const (
	SourceDefault Source = "default"
	SourceFile    Source = "file"
	SourceEnv     Source = "env"
	SourceFlag    Source = "flag"
)

// FlagOverrides carries CLI flag values that override the file. A nil pointer means
// the flag was not set, so the file/default value stands.
type FlagOverrides struct {
	Addr           *string
	Store          *string
	DBPath         *string
	RedisAddr      *string
	Rules          *string
	SigningKey     *string
	LogFormat      *string
	Pricebook      *string
	RevertInterval *string
	Auth           *bool
	FailClosed     *bool
}

// Effective is the fully-resolved configuration the binaries consume: a resolved
// File (defaults < file < env < flags applied) plus per-field provenance.
type Effective struct {
	File
	prov map[string]Source
}

// Source returns where a dotted key's effective value came from ("" if untracked).
func (e Effective) Source(key string) Source { return e.prov[key] }

// Keys returns all tracked provenance keys, sorted.
func (e Effective) Keys() []string {
	ks := make([]string, 0, len(e.prov))
	for k := range e.prov {
		ks = append(ks, k)
	}
	sort.Strings(ks)
	return ks
}

// RevertInterval parses the resolved server.revert_interval duration string.
func (e Effective) RevertInterval() (time.Duration, error) {
	return time.ParseDuration(e.Server.RevertInterval)
}

// Resolve merges defaults < file < env < flags into an Effective view. `existed`
// reports whether the file was present, so base provenance is file vs default.
func Resolve(f File, existed bool, flags FlagOverrides, getenv func(string) string) Effective {
	if getenv == nil {
		getenv = func(string) string { return "" }
	}
	applyDefaults(&f) // idempotent; ensures a bare File still resolves
	base := SourceFile
	if !existed {
		base = SourceDefault
	}
	e := Effective{File: f, prov: map[string]Source{}}
	for _, k := range []string{
		"server.addr", "server.revert_interval", "store.kind", "store.path",
		"store.redis_addr", "engine.signing_key", "engine.log_format",
		"engine.pricebook", "engine.fail_mode.default", "engine.fail_mode.notify",
		"auth.enabled", "auth.keys_env", "rules",
	} {
		e.prov[k] = base
	}

	// Env layer: legacy LiteLLM wiring. The URL is non-secret; the token is resolved
	// elsewhere through the SLOPPY_TOKEN_* broker, never read here.
	if v := getenv("SLOPPY_LITELLM_URL"); v != "" {
		if e.Platforms == nil {
			e.Platforms = map[string]Platform{}
		}
		p := e.Platforms["litellm"]
		p.Enabled = true
		p.URL = v
		if p.TokenEnv == "" {
			p.TokenEnv = "SLOPPY_TOKEN_LITELLM"
		}
		e.Platforms["litellm"] = p
		e.prov["platforms.litellm.enabled"] = SourceEnv
		e.prov["platforms.litellm.url"] = SourceEnv
	}

	// Flag layer (highest precedence).
	if flags.Addr != nil {
		e.Server.Addr = *flags.Addr
		e.prov["server.addr"] = SourceFlag
	}
	if flags.RevertInterval != nil {
		e.Server.RevertInterval = *flags.RevertInterval
		e.prov["server.revert_interval"] = SourceFlag
	}
	if flags.Store != nil {
		e.Store.Kind = *flags.Store
		e.prov["store.kind"] = SourceFlag
	}
	if flags.DBPath != nil {
		e.Store.Path = *flags.DBPath
		e.prov["store.path"] = SourceFlag
	}
	if flags.RedisAddr != nil {
		e.Store.RedisAddr = *flags.RedisAddr
		e.prov["store.redis_addr"] = SourceFlag
	}
	if flags.Rules != nil {
		e.Rules = []string{*flags.Rules}
		e.prov["rules"] = SourceFlag
	}
	if flags.SigningKey != nil {
		e.Engine.SigningKey = *flags.SigningKey
		e.prov["engine.signing_key"] = SourceFlag
	}
	if flags.LogFormat != nil {
		e.Engine.LogFormat = *flags.LogFormat
		e.prov["engine.log_format"] = SourceFlag
	}
	if flags.Pricebook != nil {
		e.Engine.Pricebook = *flags.Pricebook
		e.prov["engine.pricebook"] = SourceFlag
	}
	if flags.Auth != nil {
		e.Auth.Enabled = *flags.Auth
		e.prov["auth.enabled"] = SourceFlag
	}
	if flags.FailClosed != nil && *flags.FailClosed {
		e.Engine.FailMode.Default = "closed"
		e.prov["engine.fail_mode.default"] = SourceFlag
	}
	return e
}
