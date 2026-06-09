package rules

import (
	"testing"
	"time"
)

func TestParseIntentBudget(t *testing.T) {
	cases := []struct {
		in string
		n  int
		w  time.Duration
		ok bool
	}{
		{"", 0, 0, true}, // unset = unlimited
		{"3/h", 3, time.Hour, true},
		{"10/5m", 10, 5 * time.Minute, true},
		{"5/30s", 5, 30 * time.Second, true},
		{"bad", 0, 0, false},
		{"3/", 0, 0, false},
		{"x/h", 0, 0, false},
		{"3/zzz", 0, 0, false},
	}
	for _, c := range cases {
		n, w, err := ParseIntentBudget(c.in)
		if c.ok != (err == nil) {
			t.Fatalf("%q: ok=%v err=%v", c.in, c.ok, err)
		}
		if c.ok && (n != c.n || w != c.w) {
			t.Fatalf("%q => %d/%v, want %d/%v", c.in, n, w, c.n, c.w)
		}
	}
}

func TestValidate(t *testing.T) {
	good, _ := ParseRules([]byte(`
on: x
when: signal.tenant == "a"
then: [ { page: {} } ]
with: { intent_budget: "3/h" }
`))
	if err := Validate(good[0]); err != nil {
		t.Fatalf("good rule should validate: %v", err)
	}

	badCEL, _ := ParseRules([]byte("on: x\nwhen: signal.tenant ==\nthen: [ { page: {} } ]\n"))
	if Validate(badCEL[0]) == nil {
		t.Fatal("malformed CEL `when` should fail validation")
	}

	badAct, _ := ParseRules([]byte("on: x\nwhen: \"true\"\nthen: [ { frobnicate: {} } ]\n"))
	if Validate(badAct[0]) == nil {
		t.Fatal("unknown action kind should fail validation")
	}

	badBud, _ := ParseRules([]byte("on: x\nwhen: \"true\"\nthen: [ { page: {} } ]\nwith: { intent_budget: \"nope\" }\n"))
	if Validate(badBud[0]) == nil {
		t.Fatal("bad intent_budget should fail validation")
	}
}
