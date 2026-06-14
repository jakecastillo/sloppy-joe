package rules

import (
	"errors"
	"fmt"

	"github.com/google/cel-go/cel"

	"github.com/sloppyjoe/sloppy/core"
)

// Condition is a compiled CEL program over {signal, state}.
type Condition struct{ prog cel.Program }

func celEnv() (*cel.Env, error) {
	return cel.NewEnv(
		cel.Variable("signal", cel.DynType),
		cel.Variable("state", cel.DynType),
	)
}

// CompileCondition compiles a CEL boolean expression over signal+state.
func CompileCondition(expr string) (*Condition, error) {
	env, err := celEnv()
	if err != nil {
		return nil, err
	}
	ast, iss := env.Compile(expr)
	if iss != nil && iss.Err() != nil {
		return nil, fmt.Errorf("rules: cel compile: %w", iss.Err())
	}
	prog, err := env.Program(ast)
	if err != nil {
		return nil, err
	}
	return &Condition{prog: prog}, nil
}

// Eval evaluates the condition. `signal` is flattened so `signal.tenant`,
// `signal.alias`, `signal.severity`, `signal.type`, and `signal.data.*` are addressable.
func (c *Condition) Eval(sig core.Signal, state map[string]any) (bool, error) {
	if state == nil {
		state = map[string]any{}
	}
	sigMap := map[string]any{
		"tenant":     sig.Subject.Tenant,
		"deployment": sig.Subject.Deployment,
		"alias":      sig.Subject.Alias,
		"severity":   string(sig.Severity),
		"type":       sig.Type,
		"data":       sig.Data,
	}
	out, _, err := c.prog.Eval(map[string]any{"signal": sigMap, "state": state})
	if err != nil {
		return false, err
	}
	b, ok := out.Value().(bool)
	if !ok {
		return false, errors.New("rules: condition did not return bool")
	}
	return b, nil
}
