package engine

import (
	"context"
	"testing"

	"github.com/sloppyjoe/sloppy/core"
	"github.com/sloppyjoe/sloppy/intent"
)

const rule = `
on: cost.budget_burn
when: signal.data.spend_1h_usd > 5.0
then:
  - route_override: { alias: gpt-4o, to: ollama/llama3, ttl: 30m }
with: { dry_run: false }
`

func countApplied(rs []Result) int {
	n := 0
	for _, r := range rs {
		if r.Outcome == OutApplied {
			n++
		}
	}
	return n
}

func TestEngineClosesLoopSignsAndIsIdempotent(t *testing.T) {
	e, f, st, signer := mustEngine(t, rule)
	defer st.Close()
	sig := core.Signal{Type: "cost.budget_burn", CorrelationKey: "acme:cost",
		Subject: core.Subject{Alias: "gpt-4o"}, Data: map[string]any{"spend_1h_usd": 9.0}}

	res, err := e.Handle(context.Background(), sig)
	if err != nil || countApplied(res) != 1 {
		t.Fatalf("first handle applied=%d err=%v", countApplied(res), err)
	}
	got := res[0]
	if got.Intent.Signature == "" || !signer.Verify(got.Intent.CanonicalBytes(), got.Intent.Signature) {
		t.Fatal("applied intent must carry a verifiable signature")
	}

	res2, _ := e.Handle(context.Background(), sig)
	if countApplied(res2) != 0 || res2[0].Outcome != OutSkipped {
		t.Fatalf("replay should skip, got %+v", res2)
	}
	if f.Applied != 1 {
		t.Fatalf("actuator should fire exactly once, fired %d", f.Applied)
	}
	if !st.VerifyAudit(context.Background()) {
		t.Fatal("audit chain invalid")
	}
}

// The applied-audit entry must persist the FULL signature plus the signed
// canonical bytes, so a verifier holding only the public key can confirm the
// intent was authentically signed — and detect any later edit to a field.
func TestAppliedAuditPersistsVerifiableSignature(t *testing.T) {
	e, _, st, signer := mustEngine(t, rule)
	defer st.Close()
	sig := core.Signal{Type: "cost.budget_burn", CorrelationKey: "acme:cost",
		Subject: core.Subject{Alias: "gpt-4o"}, Data: map[string]any{"spend_1h_usd": 9.0}}
	if _, err := e.Handle(context.Background(), sig); err != nil {
		t.Fatalf("handle: %v", err)
	}
	entries, err := st.Audit(context.Background())
	if err != nil {
		t.Fatalf("audit: %v", err)
	}
	pub := signer.PublicKey()
	verified := 0
	for _, en := range entries {
		if en.Kind != "intent.applied" {
			continue
		}
		ok, found := intent.VerifyAuditDetail(pub, en.Detail)
		if !found {
			t.Fatalf("intent.applied entry carries no verifiable signature: %q", en.Detail)
		}
		if !ok {
			t.Fatalf("persisted signature failed to verify: %q", en.Detail)
		}
		verified++
	}
	if verified != 1 {
		t.Fatalf("expected exactly 1 verifiable applied entry, got %d", verified)
	}
}

func TestEngineDryRunDoesNotActuate(t *testing.T) {
	e, f, st, _ := mustEngine(t, `
on: cost.budget_burn
when: signal.data.spend_1h_usd > 5.0
then: [ { route_override: { alias: gpt-4o, to: ollama/llama3 } } ]
with: { dry_run: true }
`)
	defer st.Close()
	res, _ := e.Handle(context.Background(), core.Signal{Type: "cost.budget_burn",
		Subject: core.Subject{Alias: "gpt-4o"}, Data: map[string]any{"spend_1h_usd": 9.0}})
	if f.Applied != 0 {
		t.Fatalf("dry_run must not actuate, fired %d", f.Applied)
	}
	if len(res) != 1 || res[0].Outcome != OutDryRun {
		t.Fatalf("expected dry_run outcome, got %+v", res)
	}
}
