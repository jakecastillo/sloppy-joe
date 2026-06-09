package config

import "testing"

func TestResolveFlagBeatsFileBeatsDefault(t *testing.T) {
	f := Defaults()
	f.Store.Path = "from-file.db"
	store := "redis"
	eff := Resolve(f, true, FlagOverrides{Store: &store}, func(string) string { return "" })
	if eff.Store.Kind != "redis" {
		t.Fatalf("flag should win for store.kind: %q", eff.Store.Kind)
	}
	if eff.Source("store.kind") != SourceFlag {
		t.Fatalf("provenance store.kind = %q, want flag", eff.Source("store.kind"))
	}
	if eff.Store.Path != "from-file.db" || eff.Source("store.path") != SourceFile {
		t.Fatalf("file value/provenance wrong: %q/%q", eff.Store.Path, eff.Source("store.path"))
	}
}

func TestResolveZeroConfigDefaults(t *testing.T) {
	eff := Resolve(Defaults(), false, FlagOverrides{}, func(string) string { return "" })
	if eff.Source("store.kind") != SourceDefault {
		t.Fatalf("zero-config base should be default, got %q", eff.Source("store.kind"))
	}
}

func TestResolveLegacyLitellmEnv(t *testing.T) {
	getenv := func(k string) string {
		if k == "SLOPPY_LITELLM_URL" {
			return "http://gw:4000"
		}
		return ""
	}
	eff := Resolve(Defaults(), false, FlagOverrides{}, getenv)
	p, ok := eff.Platforms["litellm"]
	if !ok || !p.Enabled || p.URL != "http://gw:4000" {
		t.Fatalf("legacy litellm env not wired: %+v", p)
	}
	if eff.Source("platforms.litellm.url") != SourceEnv {
		t.Fatalf("litellm url provenance = %q, want env", eff.Source("platforms.litellm.url"))
	}
}

func TestResolveRevertIntervalParses(t *testing.T) {
	f := Defaults()
	f.Server.RevertInterval = "45s"
	eff := Resolve(f, true, FlagOverrides{}, nil)
	d, err := eff.RevertInterval()
	if err != nil || d.String() != "45s" {
		t.Fatalf("revert interval parse: %v %v", d, err)
	}
}
