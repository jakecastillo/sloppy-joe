package actuator

import (
	"io"
	"testing"

	"github.com/sloppyjoe/sloppy/core"
)

func TestFakeConformance(t *testing.T) {
	Conformance(t, &Fake{}, core.RemediationIntent{ID: "c1", Target: "gpt-4o"})
}

func TestLogConformance(t *testing.T) {
	Conformance(t, &Log{W: io.Discard}, core.RemediationIntent{ID: "c2", Target: "gpt-4o"})
}
