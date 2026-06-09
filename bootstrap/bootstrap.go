// Package bootstrap constructs the actuator registry and the engine from the
// effective configuration. It is the single place platforms are wired, replacing
// the env-var side-effect logic that was duplicated across both binaries — and it
// finally makes the Bifrost/Envoy/GitHub/Slack actuators reachable from config.
package bootstrap

import (
	"fmt"
	"io"
	"log/slog"
	"os"
	"sort"
	"strings"

	"github.com/sloppyjoe/sloppy/actuator"
	"github.com/sloppyjoe/sloppy/config"
	"github.com/sloppyjoe/sloppy/engine"
	"github.com/sloppyjoe/sloppy/intent"
	"github.com/sloppyjoe/sloppy/ledger"
	"github.com/sloppyjoe/sloppy/metrics"
	"github.com/sloppyjoe/sloppy/recipe"
	"github.com/sloppyjoe/sloppy/rules"
	"github.com/sloppyjoe/sloppy/secrets"
	"github.com/sloppyjoe/sloppy/state"
)

// BuildRegistry constructs the actuator Registry from the effective platform
// config. The Log actuator is always registered (for visibility); each enabled
// platform is wired with a credential resolved through the SLOPPY_TOKEN_* broker —
// no actuator is ever handed an inline secret.
func BuildRegistry(eff config.Effective, out io.Writer) (*actuator.Registry, error) {
	reg := actuator.NewRegistry()
	reg.Register(&actuator.Log{W: out})

	// Allowlist the broker to exactly the enabled platforms (default-deny).
	var caps []string
	for name, p := range eff.Platforms {
		if p.Enabled {
			caps = append(caps, name)
		}
	}
	broker := secrets.NewEnvBroker(caps)
	tok := func(name string) actuator.TokenFunc {
		return func() (string, error) { return broker.Get(name) }
	}

	for name, p := range eff.Platforms {
		if !p.Enabled {
			continue
		}
		switch name {
		case "litellm":
			reg.Register(actuator.NewLiteLLM(p.URL, tok("litellm")))
		case "bifrost":
			reg.Register(actuator.NewBifrost(p.URL, tok("bifrost")))
		case "envoy":
			reg.Register(actuator.NewEnvoy(p.URL, tok("envoy")))
		case "github":
			reg.Register(actuator.NewGitHub(githubBase(p), tok("github")))
		case "slack":
			reg.Register(actuator.NewSlack(tok("slack")))
		default:
			return nil, fmt.Errorf("bootstrap: unknown platform %q", name)
		}
	}
	return reg, nil
}

func githubBase(p config.Platform) string {
	if p.BaseURL != "" {
		return p.BaseURL
	}
	return "https://api.github.com"
}

// BuildEngine wires the full control loop from effective config and returns the
// engine, cost ledger, metrics registry, and a cleanup closer.
func BuildEngine(eff config.Effective, out io.Writer, logger *slog.Logger) (*engine.Engine, *ledger.CostLedger, *metrics.Registry, func(), error) {
	rs, err := loadRulesLenient(eff.Rules)
	if err != nil {
		return nil, nil, nil, nil, err
	}
	recs, err := RenderRecipes(eff)
	if err != nil {
		return nil, nil, nil, nil, err
	}
	rs = append(rs, recs...)
	if len(rs) == 0 {
		return nil, nil, nil, nil, fmt.Errorf("bootstrap: no rules in %v and no recipes enabled", eff.Rules)
	}
	rec, err := rules.NewReconciler(rs)
	if err != nil {
		return nil, nil, nil, nil, err
	}
	// Load the signer BEFORE opening the store so the same key that signs intents
	// also signs the store's audit checkpoint (the length+head anchor that makes
	// truncation/deletion/replacement detectable, not just edits).
	signer, err := intent.LoadOrCreateSigner(eff.Engine.SigningKey)
	if err != nil {
		return nil, nil, nil, nil, err
	}
	st, err := config.OpenStore(eff.Store.Kind, eff.Store.Path, eff.Store.RedisAddr,
		state.WithCheckpointSigner(signer))
	if err != nil {
		return nil, nil, nil, nil, err
	}
	var pb ledger.PriceBook
	if eff.Engine.Pricebook != "" {
		b, err := os.ReadFile(eff.Engine.Pricebook)
		if err != nil {
			st.Close()
			return nil, nil, nil, nil, err
		}
		if pb, err = ledger.LoadPriceBook(b); err != nil {
			st.Close()
			return nil, nil, nil, nil, err
		}
	}
	l := ledger.New(pb, st)
	m := metrics.New()
	reg, err := BuildRegistry(eff, out)
	if err != nil {
		st.Close()
		return nil, nil, nil, nil, err
	}
	fmDefault := engine.FailOpen
	if eff.Engine.FailMode.Default == "closed" {
		fmDefault = engine.FailClosed
	}
	fmNotify := engine.FailOpen
	if eff.Engine.FailMode.Notify == "closed" {
		fmNotify = engine.FailClosed
	}
	e := engine.New(rec, reg, st, signer,
		engine.WithLedger(l), engine.WithMetrics(m),
		engine.WithFailMode(fmDefault), engine.WithFailModeNotify(fmNotify), engine.WithLogger(logger))
	return e, l, m, func() { st.Close() }, nil
}

// loadRulesLenient concatenates rules from every configured path. A missing or
// empty path is tolerated (recipes may supply the rules); a present, non-empty path
// that fails to parse is a hard error.
func loadRulesLenient(paths []string) ([]rules.Rule, error) {
	var all []rules.Rule
	for _, p := range paths {
		if _, err := os.Stat(p); err != nil {
			if os.IsNotExist(err) {
				continue
			}
			return nil, err
		}
		rs, err := config.LoadRules(p)
		if err != nil {
			if strings.Contains(err.Error(), "no rules found") {
				continue
			}
			return nil, err
		}
		all = append(all, rs...)
	}
	return all, nil
}

// RenderRecipes renders every enabled recipe in the config into rules, applying the
// config's enabled notify platforms (github/slack) for platform-aware actions.
func RenderRecipes(eff config.Effective) ([]rules.Rule, error) {
	plat := recipePlatforms(eff)
	names := make([]string, 0, len(eff.Recipes))
	for n := range eff.Recipes {
		names = append(names, n)
	}
	sort.Strings(names)
	var all []rules.Rule
	for _, n := range names {
		rc := eff.Recipes[n]
		if !rc.Enabled {
			continue
		}
		_, rs, err := recipe.Render(n, rc.Params, plat)
		if err != nil {
			return nil, err
		}
		all = append(all, rs...)
	}
	return all, nil
}

// RenderRecipeText renders one recipe to its YAML text + content-hash SHA, for
// `sloppy recipe show`. The recipe need not be enabled.
func RenderRecipeText(eff config.Effective, name string) (string, string, error) {
	var raw map[string]any
	if rc, ok := eff.Recipes[name]; ok {
		raw = rc.Params
	}
	text, rs, err := recipe.Render(name, raw, recipePlatforms(eff))
	if err != nil {
		return "", "", err
	}
	sha := ""
	if len(rs) > 0 {
		sha = rs[0].SHA
	}
	return text, sha, nil
}

func recipePlatforms(eff config.Effective) recipe.Platforms {
	var p recipe.Platforms
	if g, ok := eff.Platforms["github"]; ok {
		p.Github = recipe.Target{Enabled: g.Enabled, Repo: g.Repo}
	}
	if s, ok := eff.Platforms["slack"]; ok {
		p.Slack = recipe.Target{Enabled: s.Enabled, Channel: s.Channel}
	}
	return p
}
