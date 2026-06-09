package rules

import (
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/sloppyjoe/sloppy/core"
)

// ParseIntentBudget parses an `intent_budget` value of the form "N/window"
// (e.g. "3/h", "10/5m"). A bare unit means 1 of it ("h" -> "1h"). Empty = unlimited.
func ParseIntentBudget(s string) (count int, window time.Duration, err error) {
	s = strings.TrimSpace(s)
	if s == "" {
		return 0, 0, nil // unset = no limit
	}
	parts := strings.SplitN(s, "/", 2)
	if len(parts) != 2 {
		return 0, 0, fmt.Errorf("intent_budget %q must be N/window (e.g. 3/h)", s)
	}
	count, err = strconv.Atoi(strings.TrimSpace(parts[0]))
	if err != nil || count < 0 {
		return 0, 0, fmt.Errorf("intent_budget %q: bad count", s)
	}
	w := strings.TrimSpace(parts[1])
	if w != "" && (w[0] < '0' || w[0] > '9') {
		w = "1" + w // bare unit -> 1<unit>
	}
	window, err = time.ParseDuration(w)
	if err != nil || window <= 0 {
		return 0, 0, fmt.Errorf("intent_budget %q: bad window", s)
	}
	return count, window, nil
}

// Validate checks a parsed rule beyond YAML structure: the CEL `when` compiles,
// every action kind is known, and `intent_budget` (if set) parses.
func Validate(r Rule) error {
	if _, err := CompileCondition(r.When); err != nil {
		return err
	}
	for _, a := range r.Then {
		if !core.IsKnownActionKind(core.ActionKind(a.Kind)) {
			return fmt.Errorf("unknown action %q", a.Kind)
		}
	}
	if _, _, err := ParseIntentBudget(r.With.IntentBudget); err != nil {
		return err
	}
	return nil
}
