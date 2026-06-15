package recipe

import (
	"strings"
	"testing"

	"github.com/sloppyjoe/sloppy/rules"
)

func TestRenderCostGuardDefaults(t *testing.T) {
	text, rs, err := Render("cost-guard", nil, Platforms{})
	if err != nil {
		t.Fatal(err)
	}
	if len(rs) != 1 {
		t.Fatalf("want 1 rule, got %d", len(rs))
	}
	if rs[0].On != "cost.budget_burn" {
		t.Fatalf("on = %q", rs[0].On)
	}
	if rs[0].SHA == "" {
		t.Fatal("rendered rule must carry a content-hash SHA")
	}
	// No notify platform enabled => only the primary route_override action.
	if strings.Contains(text, "open_issue") || strings.Contains(text, "page:") {
		t.Fatalf("no notify platforms enabled, but got:\n%s", text)
	}
	if err := rules.Validate(rs[0]); err != nil {
		t.Fatalf("rendered rule should validate: %v", err)
	}
}

func TestRenderCostGuardPlatformAware(t *testing.T) {
	text, rs, err := Render("cost-guard", nil, Platforms{
		Github: Target{Enabled: true, Repo: "acme/ops"},
		Slack:  Target{Enabled: true, Channel: "#oncall"},
	})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(text, "open_issue: { repo: acme/ops }") {
		t.Fatalf("github enabled should add open_issue:\n%s", text)
	}
	if !strings.Contains(text, `page: { slack: "#oncall" }`) {
		t.Fatalf("slack enabled should add page:\n%s", text)
	}
	if len(rs[0].Then) != 3 {
		t.Fatalf("want 3 actions (route_override+open_issue+page), got %d", len(rs[0].Then))
	}
}

func TestRenderDeterministicSHA(t *testing.T) {
	_, a, err := Render("cost-guard", map[string]any{"threshold_usd_1h": 8.0}, Platforms{})
	if err != nil {
		t.Fatal(err)
	}
	_, b, err := Render("cost-guard", map[string]any{"threshold_usd_1h": 8.0}, Platforms{})
	if err != nil {
		t.Fatal(err)
	}
	if a[0].SHA != b[0].SHA {
		t.Fatalf("same params must reproduce the same SHA: %s != %s", a[0].SHA, b[0].SHA)
	}
	_, c, err := Render("cost-guard", map[string]any{"threshold_usd_1h": 9.0}, Platforms{})
	if err != nil {
		t.Fatal(err)
	}
	if a[0].SHA == c[0].SHA {
		t.Fatal("different params should change the rendered SHA")
	}
}

func TestRenderCostRunawayDefaults(t *testing.T) {
	text, rs, err := Render("cost-runaway", nil, Platforms{})
	if err != nil {
		t.Fatal(err)
	}
	if len(rs) != 1 {
		t.Fatalf("want 1 rule, got %d", len(rs))
	}
	if rs[0].On != "cost.budget_burn" {
		t.Fatalf("on = %q", rs[0].On)
	}
	if rs[0].SHA == "" {
		t.Fatal("rendered rule must carry a content-hash SHA")
	}
	// The primary action must be throttle_tenant with a TTL...
	if len(rs[0].Then) != 1 {
		t.Fatalf("want 1 action (throttle_tenant), got %d", len(rs[0].Then))
	}
	if rs[0].Then[0].Kind != "throttle_tenant" {
		t.Fatalf("primary action = %q, want throttle_tenant", rs[0].Then[0].Kind)
	}
	if ttl, _ := rs[0].Then[0].Args["ttl"].(string); ttl != "30m" {
		t.Fatalf("throttle_tenant ttl = %q, want 30m", ttl)
	}
	// ...with a rollback (on_clear) on a spend-over-threshold trigger.
	if rs[0].With.Rollback != "on_clear" {
		t.Fatalf("rollback = %q, want on_clear", rs[0].With.Rollback)
	}
	if !strings.Contains(rs[0].When, "spend_1h_usd >") {
		t.Fatalf("when must be a spend-over-threshold trigger, got %q", rs[0].When)
	}
	// No notify platform enabled => only the primary throttle_tenant action.
	if strings.Contains(text, "open_issue") || strings.Contains(text, "page:") {
		t.Fatalf("no notify platforms enabled, but got:\n%s", text)
	}
	if err := rules.Validate(rs[0]); err != nil {
		t.Fatalf("rendered rule should validate: %v", err)
	}
}

func TestRenderCostRunawayPlatformAware(t *testing.T) {
	text, rs, err := Render("cost-runaway", nil, Platforms{
		Github: Target{Enabled: true, Repo: "acme/ops"},
		Slack:  Target{Enabled: true, Channel: "#oncall"},
	})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(text, "open_issue: { repo: acme/ops }") {
		t.Fatalf("github enabled should add open_issue:\n%s", text)
	}
	if !strings.Contains(text, `page: { slack: "#oncall" }`) {
		t.Fatalf("slack enabled should add page:\n%s", text)
	}
	if len(rs[0].Then) != 3 {
		t.Fatalf("want 3 actions (throttle_tenant+open_issue+page), got %d", len(rs[0].Then))
	}
}

func TestRenderCostRunawayValidateAndParams(t *testing.T) {
	if _, _, err := Render("cost-runaway", map[string]any{"threshold_usd_1h": 0}, Platforms{}); err == nil {
		t.Fatal("threshold 0 should fail validation")
	}
	if _, _, err := Render("cost-runaway", map[string]any{"ttl": ""}, Platforms{}); err == nil {
		t.Fatal("empty ttl should fail validation")
	}
	if _, _, err := Render("cost-runaway", map[string]any{"bogus_key": 1}, Platforms{}); err == nil {
		t.Fatal("unknown param key should fail strict decode")
	}
	// Overriding params still round-trips through ParseRules and validates.
	text, rs, err := Render("cost-runaway", map[string]any{"threshold_usd_1h": 75.0, "ttl": "1h"}, Platforms{})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(text, "ttl: 1h") {
		t.Fatalf("override ttl should render, got:\n%s", text)
	}
	if err := rules.Validate(rs[0]); err != nil {
		t.Fatalf("rendered rule should validate: %v", err)
	}
}

func TestNamesIncludesCostRunaway(t *testing.T) {
	var found bool
	for _, n := range Names() {
		if n == "cost-runaway" {
			found = true
		}
	}
	if !found {
		t.Fatalf("Names() must include cost-runaway, got %v", Names())
	}
}

func TestRenderUnknownRecipe(t *testing.T) {
	if _, _, err := Render("nope", nil, Platforms{}); err == nil {
		t.Fatal("unknown recipe should error")
	}
}

func TestRenderBadParams(t *testing.T) {
	if _, _, err := Render("cost-guard", map[string]any{"threshold_usd_1h": 0}, Platforms{}); err == nil {
		t.Fatal("threshold 0 should fail validation")
	}
	if _, _, err := Render("cost-guard", map[string]any{"bogus_key": 1}, Platforms{}); err == nil {
		t.Fatal("unknown param key should fail strict decode")
	}
}

func TestRenderAllRecipesCompileAndValidate(t *testing.T) {
	plat := Platforms{Github: Target{Enabled: true, Repo: "o/r"}, Slack: Target{Enabled: true, Channel: "#x"}}
	for _, n := range Names() {
		_, rs, err := Render(n, nil, plat)
		if err != nil {
			t.Fatalf("render %s: %v", n, err)
		}
		if _, err := rules.NewReconciler(rs); err != nil {
			t.Fatalf("recipe %s rules should compile: %v", n, err)
		}
		for _, r := range rs {
			if err := rules.Validate(r); err != nil {
				t.Fatalf("recipe %s rule should validate: %v", n, err)
			}
		}
	}
}
