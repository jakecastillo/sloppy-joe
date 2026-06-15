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

func TestCheckLiteLLM(t *testing.T) {
	if c := CheckLiteLLM(""); !c.OK {
		t.Fatal("empty url should skip-ok")
	}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()
	if c := CheckLiteLLM(srv.URL); !c.OK {
		t.Fatalf("reachable server should pass: %+v", c)
	}
	if c := CheckLiteLLM("http://127.0.0.1:1"); c.OK {
		t.Fatal("unreachable server should fail")
	}
}
