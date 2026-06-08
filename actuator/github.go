package actuator

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	"github.com/sloppyjoe/sloppy/core"
)

// GitHub opens issues via the REST API (baseURL overridable for tests).
type GitHub struct {
	baseURL string
	token   TokenFunc
	client  *http.Client
}

// NewGitHub builds the adapter.
func NewGitHub(baseURL string, token TokenFunc) *GitHub {
	return &GitHub{baseURL: baseURL, token: token, client: &http.Client{Timeout: 10 * time.Second}}
}

func (g *GitHub) Capabilities() []core.ActionKind { return []core.ActionKind{core.ActionOpenIssue} }

// Apply opens an issue carrying the intent id + rule SHA as provenance.
func (g *GitHub) Apply(ctx context.Context, i core.RemediationIntent) (core.Receipt, error) {
	repo, _ := i.Args["repo"].(string)
	tok, err := g.token()
	if err != nil {
		return core.Receipt{IntentID: i.ID, Actuator: "github", Outcome: core.OutcomeFailed}, err
	}
	payload := map[string]any{
		"title": fmt.Sprintf("Sloppy Joe: auto-mitigation for %s", i.Target),
		"body":  fmt.Sprintf("Automated remediation fired.\n\nIntent: `%s`\nRule SHA: `%s`", i.ID, i.RuleSHA),
	}
	buf, _ := json.Marshal(payload)
	req, _ := http.NewRequestWithContext(ctx, http.MethodPost, fmt.Sprintf("%s/repos/%s/issues", g.baseURL, repo), bytes.NewReader(buf))
	req.Header.Set("Authorization", "Bearer "+tok)
	req.Header.Set("Accept", "application/vnd.github+json")
	resp, err := g.client.Do(req)
	if err != nil {
		return core.Receipt{IntentID: i.ID, Actuator: "github", Outcome: core.OutcomeFailed}, err
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 300 {
		return core.Receipt{IntentID: i.ID, Actuator: "github", Outcome: core.OutcomeFailed}, fmt.Errorf("github: %d", resp.StatusCode)
	}
	return core.Receipt{IntentID: i.ID, Actuator: "github", AppliedAt: time.Now().UTC(), Outcome: core.OutcomeApplied}, nil
}

// Revert is a no-op: an opened issue stays as the incident record.
func (g *GitHub) Revert(_ context.Context, i core.RemediationIntent) (core.Receipt, error) {
	return core.Receipt{IntentID: i.ID, Actuator: "github", Outcome: core.OutcomeReverted}, nil
}
