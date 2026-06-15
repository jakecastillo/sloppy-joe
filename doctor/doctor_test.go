package doctor

import (
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestCheckRulesAndDB(t *testing.T) {
	dir := t.TempDir()
	rulesDir := filepath.Join(dir, "rules")
	os.MkdirAll(rulesDir, 0o755)
	os.WriteFile(filepath.Join(rulesDir, "r.yaml"), []byte(`
on: cost.budget_burn
when: signal.data.spend_1h_usd > 5.0
then: [ { page: { slack: "#x" } } ]
`), 0o644)

	if c := CheckRules(rulesDir); !c.OK {
		t.Fatalf("rules check should pass: %+v", c)
	}
	if c := CheckRules(filepath.Join(dir, "nope")); c.OK {
		t.Fatal("missing rules should fail")
	}
	if c := CheckDB(filepath.Join(dir, "d.db")); !c.OK {
		t.Fatalf("db check should pass: %+v", c)
	}
}

func TestCheckRulesMissingDirFriendlyMessage(t *testing.T) {
	missing := filepath.Join(t.TempDir(), "rules")

	c := CheckRules(missing)
	if c.OK {
		t.Fatal("missing rules dir should fail the check")
	}
	if !strings.Contains(c.Detail, missing) {
		t.Errorf("detail should name the missing path %q: %q", missing, c.Detail)
	}
	if !strings.Contains(c.Detail, "--rules") {
		t.Errorf("detail should mention the --rules remedy: %q", c.Detail)
	}
	// No raw OS syscall text should leak through.
	for _, raw := range []string{"GetFileAttributesEx", "system cannot find", "no such file or directory"} {
		if strings.Contains(c.Detail, raw) {
			t.Errorf("detail should not contain raw syscall text %q: %q", raw, c.Detail)
		}
	}
}

// A rules directory that exists but holds no rule files (what `sloppy init`
// scaffolds) must pass doctor as informational, not fail like a missing path.
func TestCheckRulesEmptyDirIsOK(t *testing.T) {
	empty := filepath.Join(t.TempDir(), "rules")
	if err := os.MkdirAll(empty, 0o755); err != nil {
		t.Fatal(err)
	}
	c := CheckRules(empty)
	if !c.OK {
		t.Fatalf("empty rules dir should be OK (informational): %+v", c)
	}
	if !strings.Contains(c.Detail, empty) {
		t.Errorf("detail should name the dir %q: %q", empty, c.Detail)
	}
	// Must not regress the missing-path remedy onto the empty-dir case.
	if strings.Contains(c.Detail, "not found") {
		t.Errorf("empty dir should not use the not-found message: %q", c.Detail)
	}
}

func TestCheckLiteLLM(t *testing.T) {
	if c := CheckLiteLLM(true, ""); !c.OK {
		t.Fatal("empty url should skip-ok")
	}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()
	if c := CheckLiteLLM(true, srv.URL); !c.OK {
		t.Fatalf("reachable server should pass: %+v", c)
	}
	if c := CheckLiteLLM(true, "http://127.0.0.1:1"); c.OK {
		t.Fatal("unreachable server should fail")
	}
}

// A disabled LiteLLM must not fail doctor even if its URL is set but unreachable:
// the fresh `sloppy init` scaffold ships litellm.enabled=false with a localhost URL.
func TestCheckLiteLLMDisabledIsInformational(t *testing.T) {
	c := CheckLiteLLM(false, "http://127.0.0.1:1")
	if !c.OK {
		t.Fatalf("disabled litellm must be OK even with an unreachable url: %+v", c)
	}
	if !strings.Contains(c.Detail, "disabled") {
		t.Errorf("detail should note litellm is disabled: %q", c.Detail)
	}
}
