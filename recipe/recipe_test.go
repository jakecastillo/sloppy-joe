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
