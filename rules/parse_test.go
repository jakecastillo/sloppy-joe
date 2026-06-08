package rules

import "testing"

const sampleRule = `
on: cost.budget_burn
when: signal.tenant == "acme" && signal.data.spend_1h_usd > 5.0
for: 5m
then:
  - route_override: { alias: gpt-4o, to: ollama/llama3, ttl: 30m }
  - open_issue: { repo: acme/ops }
  - page: { slack: "#oncall" }
with: { dry_run: false, intent_budget: "3/h" }
`

func TestParseRule(t *testing.T) {
	rs, err := ParseRules([]byte(sampleRule))
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if len(rs) != 1 {
		t.Fatalf("want 1 rule, got %d", len(rs))
	}
	r := rs[0]
	if r.On != "cost.budget_burn" || r.For.String() != "5m0s" {
		t.Fatalf("bad header: %+v", r)
	}
	if len(r.Then) != 3 {
		t.Fatalf("want 3 actions, got %d", len(r.Then))
	}
	// `then:` order is not guaranteed by YAML maps; assert by lookup.
	byKind := map[string]Action{}
	for _, a := range r.Then {
		byKind[a.Kind] = a
	}
	if byKind["route_override"].Args["to"] != "ollama/llama3" {
		t.Fatalf("bad route_override arg: %+v", byKind["route_override"])
	}
	if byKind["open_issue"].Args["repo"] != "acme/ops" {
		t.Fatalf("bad open_issue arg: %+v", byKind["open_issue"])
	}
	if r.SHA == "" {
		t.Fatal("rule SHA must be computed for provenance")
	}
}

func TestParseRuleRejectsMissingOn(t *testing.T) {
	if _, err := ParseRules([]byte(`when: "true"` + "\n" + `then: [ { page: {} } ]`)); err == nil {
		t.Fatal("expected error for missing on:")
	}
}
