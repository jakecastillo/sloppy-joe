package engine

import (
	"bytes"
	"context"
	"log/slog"
	"strings"
	"testing"
	"time"

	"github.com/sloppyjoe/sloppy/actuator"
	"github.com/sloppyjoe/sloppy/core"
	"github.com/sloppyjoe/sloppy/intent"
	"github.com/sloppyjoe/sloppy/rules"
	"github.com/sloppyjoe/sloppy/state"
)

func TestStructuredLoggingOnApply(t *testing.T) {
	var buf bytes.Buffer
	logger := slog.New(slog.NewJSONHandler(&buf, nil))
	rs, _ := rules.ParseRules([]byte(rule))
	rec, _ := rules.NewReconciler(rs)
	st, _ := state.OpenSQLite(t.TempDir() + "/log.db")
	defer st.Close()
	reg := actuator.NewRegistry()
	reg.Register(&actuator.Fake{})
	signer, _ := intent.NewEd25519Signer()
	e := New(rec, reg, st, signer, WithLogger(logger),
		WithClock(func() time.Time { return time.Unix(1749340800, 0).UTC() }))

	sig := core.Signal{Type: "cost.budget_burn", CorrelationKey: "acme:cost",
		Subject: core.Subject{Alias: "gpt-4o"}, Data: map[string]any{"spend_1h_usd": 9.0}}
	if _, err := e.Handle(context.Background(), sig); err != nil {
		t.Fatal(err)
	}

	out := buf.String()
	if !strings.Contains(out, "intent applied") || !strings.Contains(out, "route_override") {
		t.Fatalf("expected a structured 'intent applied' record, got: %s", out)
	}
	if !strings.Contains(out, `"rule"`) {
		t.Fatalf("expected rule SHA attribute in the log record, got: %s", out)
	}
}
