// Package recipe renders a small, curated set of configurable "workflows" into
// ordinary rules.Rule values. Recipes are embedded YAML templates over the existing
// rule schema (not compiled-in rule logic): rendering a recipe with its typed
// params produces reviewable YAML that flows through rules.ParseRules, so each
// recipe gets a real content-hash SHA and rides the same engine/replay/audit path
// as a hand-written rule.
package recipe

import (
	"bytes"
	"embed"
	"fmt"
	"io"
	"sort"
	"text/template"

	"gopkg.in/yaml.v3"

	"github.com/sloppyjoe/sloppy/rules"
)

//go:embed templates/*.yaml.tmpl
var templatesFS embed.FS

var tmpls = template.Must(template.New("recipes").ParseFS(templatesFS, "templates/*.yaml.tmpl"))

// Target is a non-secret notify destination. It is the narrow contract recipes use
// to render platform-aware actions — recipes never see the full platform config or
// any secret.
type Target struct {
	Enabled bool
	Repo    string
	Channel string
}

// Platforms carries the notify targets a recipe may render actions for.
type Platforms struct {
	Github Target
	Slack  Target
}

type notify struct {
	OpenIssue bool `yaml:"open_issue"`
	Page      bool `yaml:"page"`
}

type recipeDef struct {
	template string
	summary  string
	build    func() any // a fresh typed params struct with defaults
	validate func(any) error
}

type renderData struct {
	Params any
	Github Target
	Slack  Target
}

var registry = map[string]recipeDef{
	"cost-guard": {
		template: "cost-guard.yaml.tmpl",
		summary:  "Cost burn over a $/hr threshold -> fail over to a cheaper/local model (+ optional issue/page).",
		build:    costGuardDefaults,
		validate: costGuardValidate,
	},
	"cost-runaway": {
		template: "cost-runaway.yaml.tmpl",
		summary:  "Extreme spend over a $/hr threshold -> throttle the tenant (TTL, auto-revert on_clear) (+ optional issue/page).",
		build:    costRunawayDefaults,
		validate: costRunawayValidate,
	},
	"fallback-storm": {
		template: "fallback-storm.yaml.tmpl",
		summary:  "Critical provider-fallback storm -> page on-call (+ optional issue).",
		build:    fallbackStormDefaults,
		validate: fallbackStormValidate,
	},
	"latency-guard": {
		template: "latency-guard.yaml.tmpl",
		summary:  "p95 latency regression over a threshold -> page on-call (+ optional issue).",
		build:    latencyGuardDefaults,
		validate: latencyGuardValidate,
	},
}

// Names returns the known recipe names, sorted.
func Names() []string {
	ns := make([]string, 0, len(registry))
	for n := range registry {
		ns = append(ns, n)
	}
	sort.Strings(ns)
	return ns
}

// Known reports whether a recipe name exists.
func Known(name string) bool { _, ok := registry[name]; return ok }

// Summary returns a one-line description of a recipe ("" if unknown).
func Summary(name string) string { return registry[name].summary }

// Render decodes raw params (with defaults), validates them, renders the recipe's
// embedded template with the enabled notify targets, and parses the result into
// rules. The returned text is the exact rendered YAML; the rules carry a SHA over
// those bytes, so the same (params, platforms) always reproduce the same SHA.
func Render(name string, raw map[string]any, p Platforms) (string, []rules.Rule, error) {
	def, ok := registry[name]
	if !ok {
		return "", nil, fmt.Errorf("recipe: unknown recipe %q", name)
	}
	params := def.build()
	if len(raw) > 0 {
		b, err := yaml.Marshal(raw)
		if err != nil {
			return "", nil, err
		}
		dec := yaml.NewDecoder(bytes.NewReader(b))
		dec.KnownFields(true)
		if err := dec.Decode(params); err != nil && err != io.EOF {
			return "", nil, fmt.Errorf("recipe %s: bad params: %w", name, err)
		}
	}
	if def.validate != nil {
		if err := def.validate(params); err != nil {
			return "", nil, fmt.Errorf("recipe %s: %w", name, err)
		}
	}
	// The page action always needs a channel string; default it so a recipe is
	// valid even when Slack is not enabled (the Log actuator handles page).
	if p.Slack.Channel == "" {
		p.Slack.Channel = "#alerts"
	}
	var buf bytes.Buffer
	if err := tmpls.ExecuteTemplate(&buf, def.template, renderData{Params: params, Github: p.Github, Slack: p.Slack}); err != nil {
		return "", nil, fmt.Errorf("recipe %s: render: %w", name, err)
	}
	rs, err := rules.ParseRules(buf.Bytes())
	if err != nil {
		return "", nil, fmt.Errorf("recipe %s: parse rendered rule: %w", name, err)
	}
	return buf.String(), rs, nil
}

// --- cost-guard ---

type costGuardParams struct {
	On             string  `yaml:"on"`
	ThresholdUSD1h float64 `yaml:"threshold_usd_1h"`
	For            string  `yaml:"for"`
	Failover       struct {
		Alias string `yaml:"alias"`
		To    string `yaml:"to"`
		TTL   string `yaml:"ttl"`
	} `yaml:"failover"`
	IntentBudget string `yaml:"intent_budget"`
	Notify       notify `yaml:"notify"`
}

func costGuardDefaults() any {
	p := &costGuardParams{
		On: "cost.budget_burn", ThresholdUSD1h: 5.0, For: "5m",
		IntentBudget: "3/h", Notify: notify{OpenIssue: true, Page: true},
	}
	p.Failover.Alias = "gpt-4o"
	p.Failover.To = "ollama/llama3"
	p.Failover.TTL = "30m"
	return p
}

func costGuardValidate(a any) error {
	p := a.(*costGuardParams)
	if p.On == "" {
		return fmt.Errorf("on: required")
	}
	if p.ThresholdUSD1h <= 0 {
		return fmt.Errorf("threshold_usd_1h must be > 0")
	}
	if p.Failover.Alias == "" || p.Failover.To == "" {
		return fmt.Errorf("failover.alias and failover.to are required")
	}
	return nil
}

// --- cost-runaway ---

type costRunawayParams struct {
	On             string  `yaml:"on"`
	ThresholdUSD1h float64 `yaml:"threshold_usd_1h"`
	For            string  `yaml:"for"`
	TTL            string  `yaml:"ttl"`
	Notify         notify  `yaml:"notify"`
}

func costRunawayDefaults() any {
	return &costRunawayParams{
		On: "cost.budget_burn", ThresholdUSD1h: 50.0, For: "5m", TTL: "30m",
		Notify: notify{OpenIssue: true, Page: true},
	}
}

func costRunawayValidate(a any) error {
	p := a.(*costRunawayParams)
	if p.On == "" {
		return fmt.Errorf("on: required")
	}
	if p.ThresholdUSD1h <= 0 {
		return fmt.Errorf("threshold_usd_1h must be > 0")
	}
	if p.TTL == "" {
		return fmt.Errorf("ttl: required")
	}
	return nil
}

// --- fallback-storm ---

type fallbackStormParams struct {
	On       string `yaml:"on"`
	Severity string `yaml:"severity"`
	Notify   notify `yaml:"notify"`
}

func fallbackStormDefaults() any {
	return &fallbackStormParams{On: "fallback.fired", Severity: "critical", Notify: notify{Page: true}}
}

func fallbackStormValidate(a any) error {
	p := a.(*fallbackStormParams)
	if p.On == "" {
		return fmt.Errorf("on: required")
	}
	if p.Severity == "" {
		return fmt.Errorf("severity: required")
	}
	return nil
}

// --- latency-guard ---

type latencyGuardParams struct {
	On             string  `yaml:"on"`
	ThresholdP95Ms float64 `yaml:"threshold_p95_ms"`
	Notify         notify  `yaml:"notify"`
}

func latencyGuardDefaults() any {
	return &latencyGuardParams{On: "latency.regression", ThresholdP95Ms: 2000, Notify: notify{OpenIssue: true, Page: true}}
}

func latencyGuardValidate(a any) error {
	p := a.(*latencyGuardParams)
	if p.On == "" {
		return fmt.Errorf("on: required")
	}
	if p.ThresholdP95Ms <= 0 {
		return fmt.Errorf("threshold_p95_ms must be > 0")
	}
	return nil
}
