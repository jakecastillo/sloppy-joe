package rules

import (
	"testing"

	"github.com/sloppyjoe/sloppy/core"
)

func TestCELEvaluate(t *testing.T) {
	prog, err := CompileCondition(`signal.tenant == "acme" && signal.data.spend_1h_usd > 5.0`)
	if err != nil {
		t.Fatalf("compile: %v", err)
	}
	hit := core.Signal{Subject: core.Subject{Tenant: "acme"}, Data: map[string]any{"spend_1h_usd": 7.5}}
	miss := core.Signal{Subject: core.Subject{Tenant: "acme"}, Data: map[string]any{"spend_1h_usd": 1.0}}
	if ok, err := prog.Eval(hit, nil); err != nil || !ok {
		t.Fatalf("expected hit, got ok=%v err=%v", ok, err)
	}
	if ok, _ := prog.Eval(miss, nil); ok {
		t.Fatal("expected miss")
	}
}

func TestCELCompileError(t *testing.T) {
	if _, err := CompileCondition(`signal.tenant ==`); err == nil {
		t.Fatal("expected compile error for malformed expression")
	}
}
