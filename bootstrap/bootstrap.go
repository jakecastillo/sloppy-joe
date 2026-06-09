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

	"github.com/sloppyjoe/sloppy/actuator"
	"github.com/sloppyjoe/sloppy/config"
	"github.com/sloppyjoe/sloppy/engine"
	"github.com/sloppyjoe/sloppy/intent"
	"github.com/sloppyjoe/sloppy/ledger"
	"github.com/sloppyjoe/sloppy/metrics"
	"github.com/sloppyjoe/sloppy/rules"
	"github.com/sloppyjoe/sloppy/secrets"
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
	rs, err := loadRules(eff.Rules)
	if err != nil {
		return nil, nil, nil, nil, err
	}
	rec, err := rules.NewReconciler(rs)
	if err != nil {
		return nil, nil, nil, nil, err
	}
	st, err := config.OpenStore(eff.Store.Kind, eff.Store.Path, eff.Store.RedisAddr)
	if err != nil {
		return nil, nil, nil, nil, err
	}
	signer, err := intent.LoadOrCreateSigner(eff.Engine.SigningKey)
	if err != nil {
		st.Close()
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
	fm := engine.FailOpen
	if eff.Engine.FailMode.Default == "closed" {
		fm = engine.FailClosed
	}
	e := engine.New(rec, reg, st, signer,
		engine.WithLedger(l), engine.WithMetrics(m), engine.WithFailMode(fm), engine.WithLogger(logger))
	return e, l, m, func() { st.Close() }, nil
}

// loadRules concatenates rules from every configured path.
func loadRules(paths []string) ([]rules.Rule, error) {
	var all []rules.Rule
	for _, p := range paths {
		rs, err := config.LoadRules(p)
		if err != nil {
			return nil, err
		}
		all = append(all, rs...)
	}
	if len(all) == 0 {
		return nil, fmt.Errorf("bootstrap: no rules found in %v", paths)
	}
	return all, nil
}
