# Autonomous Multi-Agent Delivery Pipeline — Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Stand up and prove (one supervised run) an autonomous pipeline that drains `bd` ready beads into CI-gated, reviewer-approved, merged-and-pushed changes on `main`, with no human in today's loop.

**Architecture:** Beads = durable state; a `Workflow` script = the deterministic engine (sweep → discover → implement[worktrees] → deterministic path-guard → adversarial review → serialized merge); GitHub CI = the green-bar. Orchestration artifacts live in a **stealth `.sloppy-pm/` dir** (git-excluded like `.beads`) so the product tree is untouched.

**Tech Stack:** `bd` (beads) CLI, `git` worktrees, `gh` CLI, the Claude `Workflow` tool (pure-JS scripts spawning subagents), Go 1.26 toolchain for the product changes the pipeline ships.

**Canonical spec:** [`docs/superpowers/specs/2026-06-14-autonomous-delivery-pipeline-design.md`](../specs/2026-06-14-autonomous-delivery-pipeline-design.md) (decisions in §11). Read it before executing.

---

## File / artifact map

| Path | New? | Responsibility |
|---|---|---|
| `.sloppy-pm/delivery.workflow.js` | new (git-excluded) | The Workflow orchestration script (the engine). Self-contained; carries the forbidden-path list as a const. |
| `.sloppy-pm/README.md` | new (git-excluded) | What `.sloppy-pm/` is, how to run/promote it, the safety model. |
| `.git/info/exclude` | modify | Add `.sloppy-pm/` (stealth, local-only — never committed). |
| `bd` tracker (`.beads`, local) | mutate | Lifecycle labels, theme epics, deferred forbidden beads, run-one beads (incl. webhook actuator + infra). |
| `actuator/webhook.go` + `actuator/webhook_test.go` | new (**shipped by the pipeline**, not by hand) | The first bounded breadth actuator — generic-webhook `route_override`. Touches NO forbidden surface; NOT wired into `bootstrap` (that wiring is a separate `needs-human` bead). |

> Product changes (the webhook actuator, the `go-idiom` beads) are **outputs of running the pipeline**, not tasks you hand-code here. This plan builds + runs the harness; the harness writes the product code through PRs.

---

## Phase 0 — Preconditions

### Task 0: Verify the environment

**Files:** none.

- [ ] **Step 1: Verify toolchain + repo state**

Run (PowerShell, from repo root `C:/Users/i3uph/OneDrive/Documents/GitHub/sloppy-joe`):

```powershell
go version                       # expect go1.26.x
gh auth status                   # expect Logged in to github.com (repo, workflow scopes)
bd version                       # expect bd version 1.0.x
git status -sb                   # expect: ## main...origin/main  (clean, in sync)
git rev-parse --abbrev-ref HEAD  # expect: main
```

Expected: all succeed; working tree clean; on `main`, synced with `origin/main`.

- [ ] **Step 2: Snapshot the starting backlog** (for the run-five report)

Run: `bd stats; bd ready` — record counts; expect `bd ready` to include `sloppy-joe-merge-slot` and `sloppy-joe-pir` (these get filtered/deferred in Phase 1).

---

## Phase 1 — Bootstrap backlog & lifecycle (bd only; no repo files)

> All `bd` commands run from the repo root so `bd` auto-discovers `.beads`. Use `--actor sloppy-pm` for attribution. `.beads` is local-only (stealth) — these mutations are not committed.

### Task 1: Lifecycle labels + theme epics

**Files:** none (bd state).

- [ ] **Step 1: Create the three theme epics**

```powershell
bd create "THEME: breadth — popular AI integration configs" -t epic -p 1 -l "theme:breadth" `
  -d "Standing theme. Bounded per spec 8.1: new actuators for EXISTING ActionKinds via httpRouteActuator only; new ActionKind = deferred." --actor sloppy-pm
bd create "THEME: easy-to-execute/integrate workflows" -t epic -p 1 -l "theme:workflows" `
  -d "Standing theme. Adoption UX: recipes/templates/config init/integration guides + dev ergonomics." --actor sloppy-pm
bd create "THEME: memory + token optimization" -t epic -p 1 -l "theme:tokens" `
  -d "Standing theme. Orchestration efficiency + non-invariant product paths ONLY. Must not touch CanonicalBytes/audit/credential/schema (spec 8.3)." --actor sloppy-pm
bd create "THEME: demand validation (Phase-0)" -t epic -p 2 -l "theme:validation" `
  -d "Surface the demand-gating tension in the tracker (spec 8.4). Not silently skipped." --actor sloppy-pm
```

Expected: four new epic ids printed.

- [ ] **Step 2: Verify**

Run: `bd list -t epic` → expect the four theme epics.

### Task 2: Defer the known forbidden-surface beads

**Files:** none.

- [ ] **Step 1: Defer `pir` and `6e6` (they edit forbidden surfaces)**

```powershell
bd update sloppy-joe-pir --add-label "stage:deferred" --add-label "needs-human" `
  --status blocked --actor sloppy-pm
bd update sloppy-joe-6e6 --add-label "stage:deferred" --add-label "needs-human" `
  --status blocked --actor sloppy-pm
```

Rationale: `pir` → `state/audit.go` (checkpoint), `6e6` → `secrets/broker.go` interface (credential-fetch). Both forbidden (spec §5.1).

- [ ] **Step 2: Verify they leave the ready set**

Run: `bd ready` → expect neither `sloppy-joe-pir` nor `sloppy-joe-6e6` present.

### Task 3: Create the new breadth-actuator bead (run-one breadth)

**Files:** none (the bead; the code ships via the pipeline).

- [ ] **Step 1: Create with full acceptance criteria**

```powershell
bd create "Add generic-webhook route_override actuator (bounded breadth)" -t feature -p 2 `
  -l "theme:breadth,stage:ready,go-idiom" --parent <breadth-epic-id> --estimate 90 --actor sloppy-pm `
  -d "Files: actuator/webhook.go, actuator/webhook_test.go ONLY. Reuse newHTTPRoute + routeDest + TokenFunc (see actuator/httproute.go). Capabilities = [core.ActionRouteOverride] (existing kind; do NOT add an ActionKind). Do NOT touch bootstrap/bootstrap.go (broker allowlist = forbidden; wiring is a separate needs-human bead). Doc-comment the exported NewWebhook (revive)." `
  --acceptance "actuator/webhook.go defines func NewWebhook(baseURL string, token TokenFunc) Actuator via newHTTPRoute with caps [ActionRouteOverride]; POST /route body {model,to}. Tests: TestWebhookRouteOverride (httptest asserts path=/route and body.to) + TestWebhookConformance (actuator.Conformance against a 200 httptest server). go test ./actuator/... green; golangci-lint clean; coverage not reduced below 72%. NOT wired into bootstrap."
```

Reference implementation the pipeline must produce (adapt to verified signatures in `actuator/httproute.go`):

```go
// actuator/webhook.go
package actuator

import "github.com/sloppyjoe/sloppy/core"

// NewWebhook returns an Actuator driving a generic JSON webhook gateway for
// route_override intents. Apply POSTs the destination route; Revert restores the
// self-route. Endpoint contract: POST <baseURL>/route {"model":<target>,"to":<dest>}.
func NewWebhook(baseURL string, token TokenFunc) Actuator {
	return newHTTPRoute("webhook", baseURL, token,
		[]core.ActionKind{core.ActionRouteOverride},
		func(i core.RemediationIntent, isRevert bool) (string, map[string]any) {
			return "/route", map[string]any{"model": i.Target, "to": routeDest(i, isRevert)}
		})
}
```

```go
// actuator/webhook_test.go
package actuator

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/sloppyjoe/sloppy/core"
)

func TestWebhookRouteOverride(t *testing.T) {
	var gotPath string
	var gotBody map[string]any
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		_ = json.NewDecoder(r.Body).Decode(&gotBody)
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	a := NewWebhook(srv.URL, func() (string, error) { return "tok", nil })
	i := core.RemediationIntent{ID: "i1", Kind: core.ActionRouteOverride, Target: "gpt-4o", Args: map[string]any{"to": "gpt-4o-mini"}}
	rec, err := a.Apply(context.Background(), i)
	if err != nil {
		t.Fatalf("apply: %v", err)
	}
	if rec.Outcome != core.OutcomeApplied {
		t.Fatalf("outcome = %v", rec.Outcome)
	}
	if gotPath != "/route" {
		t.Fatalf("path = %q", gotPath)
	}
	if gotBody["to"] != "gpt-4o-mini" {
		t.Fatalf("body.to = %v", gotBody["to"])
	}
}

func TestWebhookConformance(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(http.StatusOK) }))
	defer srv.Close()
	a := NewWebhook(srv.URL, func() (string, error) { return "tok", nil })
	Conformance(t, a, core.RemediationIntent{ID: "c1", Kind: core.ActionRouteOverride, Target: "m", Args: map[string]any{"to": "n"}})
}
```

- [ ] **Step 2: Create the deferred bootstrap-wiring follow-up bead**

```powershell
bd create "Wire webhook actuator into bootstrap (config + broker allowlist)" -t feature -p 3 `
  -l "theme:breadth,stage:deferred,needs-human" --actor sloppy-pm `
  -d "Touches bootstrap/bootstrap.go BuildRegistry allowlist (FORBIDDEN surface) + config schema. Human-supervised only."
```

### Task 4: Create the supervised-only infra bead + classify the run-one batch

**Files:** none.

- [ ] **Step 1: Create the infra bead (this plan's Phase 2–3 work)**

```powershell
bd create "Stand up .sloppy-pm orchestration harness + guard (supervised)" -t task -p 1 `
  -l "theme:workflows,stage:ready" --estimate 120 --actor sloppy-pm `
  -d "Build .sloppy-pm/delivery.workflow.js + README, git-exclude .sloppy-pm/, verify the deterministic path-guard. Supervised; not shipped via the pipeline itself."
```

- [ ] **Step 2: Mark the known-safe `go-idiom` beads ready; verify their `Files:`**

For each candidate, read `bd show <id>` and extract the `Files:` line. Mark `stage:ready` ONLY if every file is OUTSIDE the forbidden set (spec §5.1). Known classification (verified 2026-06-14):

```powershell
# SAFE (non-forbidden files) -> ready:
bd update sloppy-joe-pve --add-label "stage:ready" --actor sloppy-pm   # metrics/metrics.go
bd update sloppy-joe-okf --add-label "stage:ready" --actor sloppy-pm   # ingest/ (functional options)
# CLASSIFY at refine (read Files: first; ready only if non-forbidden):
#   sloppy-joe-dv7 (errors.New) · sloppy-joe-4d5 (actuator doc) · sloppy-joe-25p (enum doc comments)
# DEFER (forbidden file):
bd update sloppy-joe-akf --add-label "stage:deferred" --add-label "needs-human" --status blocked --actor sloppy-pm  # ee/auth.go
```

> For `dv7`/`4d5`/`25p`: if `bd show` reveals a forbidden file (`core/intent.go` ActionKind enum for `25p`; the `actuator.Actuator` interface for `4d5`), defer it instead. The Workflow's guard is the backstop, but classify here to avoid wasted CI.

- [ ] **Step 3: Verify the prepared ready set**

Run: `bd ready -l stage:ready` → expect: the infra bead, the webhook actuator bead, `pve`, `okf`, plus any of `dv7/4d5/25p` cleared as safe. NOT `akf`, `pir`, `6e6`, `merge-slot`.

---

## Phase 2 — Author the orchestration harness

### Task 5: Create the stealth `.sloppy-pm/` dir and git-exclude it

**Files:** Create `.sloppy-pm/README.md`; Modify `.git/info/exclude`.

- [ ] **Step 1: Exclude `.sloppy-pm/` (stealth, never committed)**

```powershell
Add-Content -Path ".git/info/exclude" -Value ".sloppy-pm/" -Encoding utf8
New-Item -ItemType Directory -Force ".sloppy-pm" | Out-Null
git status -sb   # expect: .sloppy-pm/ NOT shown as untracked
```

- [ ] **Step 2: Write `.sloppy-pm/README.md`**

```markdown
# .sloppy-pm — autonomous delivery harness (local, git-excluded)

Stealth (like `.beads`): in `.git/info/exclude`, never committed. Adds zero files to the
product tree (spec "surfaces, not scope").

- `delivery.workflow.js` — the Workflow script. Run via the Claude `Workflow` tool
  (`{scriptPath: ".sloppy-pm/delivery.workflow.js"}`) or inline.
- Safety: implementers only push PR branches; only the serialized Merge stage writes `main`,
  via `gh pr merge` after the deterministic path-guard + adversarial review + CI-wait.
- Branch protection on `main` is intentionally OFF today (spec §7.1) — the guard + kill-criteria
  are the compensating controls. Revisit before any cron promotion.
```

### Task 6: Write `.sloppy-pm/delivery.workflow.js`

**Files:** Create `.sloppy-pm/delivery.workflow.js`.

- [ ] **Step 1: Write the script (full content)**

```javascript
export const meta = {
  name: 'sloppy-joe-delivery',
  description: 'Autonomous sweep→discover→implement→guard→review→merge pipeline draining bd ready beads to main',
  phases: [
    { title: 'Sweep' }, { title: 'Discover' }, { title: 'Deliver' }, { title: 'Report' },
  ],
}

const REPO = 'C:/Users/i3uph/OneDrive/Documents/GitHub/sloppy-joe'
const BEADS = REPO + '/.beads'
const ACTOR = 'sloppy-pm'
const BOT = 'Sloppy Joe Bot <bot@users.noreply.github.com>'
const MAX_BEADS = 6          // first-run budget (spec §7.5)
const FORBIDDEN = [          // spec §5.1 — file-level deterministic guard
  'intent/sign.go', 'intent/verify.go', 'core/intent.go',
  'state/audit.go', 'state/sqlite.go', 'state/redis.go', 'state/store.go',
  'secrets/broker.go', 'ee/auth.go', 'bootstrap/bootstrap.go',
  'engine/engine.go', 'actuator/actuator.go',
]
const CONTROL_LABELS = ['gt:slot', 'coordination']

const IMPL = {
  type: 'object', additionalProperties: false,
  required: ['beadId', 'ok', 'branch', 'prNumber', 'touchedFiles', 'localGate', 'note'],
  properties: {
    beadId: { type: 'string' }, ok: { type: 'boolean' }, branch: { type: 'string' },
    prNumber: { type: ['number', 'null'] }, touchedFiles: { type: 'array', items: { type: 'string' } },
    localGate: { type: 'string' }, note: { type: 'string' },
  },
}
const REVIEW = {
  type: 'object', additionalProperties: false,
  required: ['verdict', 'reasons'],
  properties: { verdict: { type: 'string', enum: ['approve', 'request-changes'] }, reasons: { type: 'array', items: { type: 'string' } } },
}
const MERGE = {
  type: 'object', additionalProperties: false,
  required: ['merged', 'mergeSha', 'beadClosed', 'slotReleased', 'note'],
  properties: {
    merged: { type: 'boolean' }, mergeSha: { type: ['string', 'null'] },
    beadClosed: { type: 'boolean' }, slotReleased: { type: 'boolean' }, note: { type: 'string' },
  },
}
const DISCOVER = {
  type: 'object', additionalProperties: false,
  required: ['batch', 'excluded'],
  properties: {
    batch: { type: 'array', items: { type: 'object', additionalProperties: false, required: ['id', 'title', 'files'], properties: { id: { type: 'string' }, title: { type: 'string' }, files: { type: 'array', items: { type: 'string' } } } } },
    excluded: { type: 'array', items: { type: 'string' } },
  },
}

phase('Sweep')
const sweep = await agent(
  `Startup sweep for the autonomous delivery run (idempotent). From ${REPO} (PowerShell): ` +
  `(1) git fetch origin; (2) git worktree prune; (3) list git worktrees and remove any under ${REPO}/.sloppy-pm/wt-* whose branch is merged or stale; ` +
  `(4) run "bd -C ${REPO} merge-slot check" — if held by a dead/old run, "bd -C ${REPO} merge-slot release"; ` +
  `(5) "bd -C ${REPO} list --status in_progress --json" — if any bead is claimed with no live worktree, reset it: "bd -C ${REPO} update <id> --status open --remove-label in_progress --actor ${ACTOR}". ` +
  `Report what you swept in one short paragraph as your final text.`,
  { label: 'sweep', phase: 'Sweep', agentType: 'general-purpose' }
)
log('sweep: ' + (sweep || 'n/a').slice(0, 280))

phase('Discover')
const disc = await agent(
  `Discover the implementable batch. From ${REPO}: run "bd -C ${REPO} ready -l stage:ready --json". ` +
  `Build "batch" = up to ${MAX_BEADS} beads, EXCLUDING any bead that: (a) has a label in ${JSON.stringify(CONTROL_LABELS)}; ` +
  `(b) has label stage:deferred/needs-human/stage:stuck; (c) whose "Files:" line (from bd show) includes any of ${JSON.stringify(FORBIDDEN)}. ` +
  `For each kept bead, extract its touched files from the "Files:" line into "files". Put every excluded bead id + reason into "excluded". Do NOT claim anything.`,
  { label: 'discover', phase: 'Discover', schema: DISCOVER, agentType: 'general-purpose' }
)
const batch = (disc && disc.batch) ? disc.batch.slice(0, MAX_BEADS) : []
log(`discover: ${batch.length} beads -> ${batch.map(b => b.id).join(', ')}`)

phase('Deliver')
const results = await pipeline(
  batch,
  // Stage 1 — implement in an isolated worktree, open a PR (never touch main).
  (bead) => agent(
    `Implement bead ${bead.id} ("${bead.title}") end-to-end. STRICT RULES: you may push ONLY your PR branch, NEVER main. ` +
    `Steps (PowerShell, from ${REPO}): ` +
    `(1) "bd -C ${REPO} update ${bead.id} --claim --actor ${ACTOR}"; ` +
    `(2) git fetch origin; $wt="${REPO}/.sloppy-pm/wt-${bead.id}"; git worktree add -b "pm/${bead.id}" $wt origin/main; ` +
    `(3) $env:BEADS_DIR="${BEADS}"; Set-Location $wt; read full criteria via "bd show ${bead.id}"; implement EXACTLY to the acceptance criteria; ` +
    `(4) local gate (prepend C:/Program Files/Go/bin if needed): go build ./... ; go vet ./... ; "C:/Program Files/Go/bin/go.exe" test ./... ; golangci-lint run ./... ; golangci-lint fmt --diff (must be empty). Fix until all pass; ` +
    `(5) commit: git add -A; git -c user.name="Sloppy Joe Bot" -c user.email="bot@users.noreply.github.com" commit -s -m "<conventional subject>" -m "Bead-Id: ${bead.id}" (NO AI/Co-Authored-By trailer); ` +
    `(6) git push origin "HEAD:refs/heads/pm/${bead.id}"; gh pr create --base main --head "pm/${bead.id}" --title "<subject>" --body "Closes bead ${bead.id}". ` +
    `Return touchedFiles = "git diff --name-only origin/main...HEAD". On any failure set ok=false with the reason in note; still report touchedFiles if a branch exists.`,
    { label: `impl:${bead.id}`, phase: 'Deliver', schema: IMPL, agentType: 'general-purpose' }
  ),
  // Stage 2 — DETERMINISTIC path-guard (pure JS) then adversarial review.
  async (impl, bead) => {
    if (!impl || !impl.ok || !impl.prNumber) return { bead, impl, status: 'impl-failed' }
    const hit = (impl.touchedFiles || []).find(f => FORBIDDEN.some(p => f.replace(/\\/g, '/').includes(p)))
    if (hit) {
      await agent(
        `GUARD TRIP: bead ${bead.id} PR #${impl.prNumber} touches forbidden surface "${hit}". From ${REPO}: ` +
        `gh pr close ${impl.prNumber} --delete-branch; "bd -C ${REPO} update ${bead.id} --status blocked --add-label stage:deferred --add-label needs-human --remove-label in_progress --actor ${ACTOR}"; ` +
        `git worktree remove --force "${REPO}/.sloppy-pm/wt-${bead.id}". Confirm done.`,
        { label: `defer:${bead.id}`, phase: 'Deliver', agentType: 'general-purpose' }
      )
      return { bead, impl, status: 'deferred-forbidden', hit }
    }
    const rev = await agent(
      `Adversarially review PR #${impl.prNumber} for bead ${bead.id}. From ${REPO}: read "gh pr diff ${impl.prNumber}" and the bead's acceptance criteria ("bd show ${bead.id}"). ` +
      `Default to request-changes if uncertain. Approve ONLY if: the diff fully meets the acceptance criteria; touches no forbidden surface (${JSON.stringify(FORBIDDEN)}); adds/keeps tests; flips no fail-closed default to fail-open; no hardcoded token/Bearer literal; no new ActionKind outside core/intent.go; commit is conventional + DCO -s with no AI trailer.`,
      { label: `review:${bead.id}`, phase: 'Deliver', schema: REVIEW, agentType: 'general-purpose' }
    )
    return { bead, impl, status: rev && rev.verdict === 'approve' ? 'approved' : 'changes', review: rev }
  },
  // Stage 3 — serialized merge via merge-slot (only approved).
  async (res) => {
    if (!res || res.status !== 'approved') return res
    const m = await agent(
      `Merge PR #${res.impl.prNumber} for bead ${res.bead.id}, SERIALIZED. From ${REPO}: ` +
      `(1) "bd -C ${REPO} merge-slot acquire --actor ${ACTOR}" (wait for it); ` +
      `(2) ensure PR up to date: "gh pr update-branch ${res.impl.prNumber}" (ignore if already up to date); ` +
      `(3) wait for CI: "gh pr checks ${res.impl.prNumber} --watch --fail-fast" (bounded ~20 min). If any required check fails, do NOT merge; ` +
      `(4) if all green: "gh pr merge ${res.impl.prNumber} --squash --delete-branch"; then git fetch origin; verify the merge commit on origin/main; "bd -C ${REPO} close ${res.bead.id} --actor ${ACTOR}"; git worktree remove --force "${REPO}/.sloppy-pm/wt-${res.bead.id}"; ` +
      `(5) ALWAYS "bd -C ${REPO} merge-slot release --actor ${ACTOR}" at the end, even on failure. ` +
      `Set merged=true only if the squash merge succeeded and CI was green.`,
      { label: `merge:${res.bead.id}`, phase: 'Deliver', schema: MERGE, agentType: 'general-purpose' }
    )
    return { ...res, merge: m, status: m && m.merged ? 'merged' : 'merge-failed' }
  }
)

phase('Report')
const clean = (results || []).filter(Boolean)
const merged = clean.filter(r => r.status === 'merged')
const deferred = clean.filter(r => r.status === 'deferred-forbidden')
const failed = clean.filter(r => r.status && r.status.endsWith('failed'))
const changes = clean.filter(r => r.status === 'changes')
log(`DONE: ${merged.length} merged, ${deferred.length} deferred(forbidden), ${changes.length} needs-changes, ${failed.length} failed`)
return {
  swept: (sweep || '').slice(0, 500),
  discovered: batch.map(b => b.id),
  excluded: (disc && disc.excluded) || [],
  merged: merged.map(r => ({ id: r.bead.id, sha: r.merge && r.merge.mergeSha })),
  deferredForbidden: deferred.map(r => ({ id: r.bead.id, hit: r.hit })),
  needsChanges: changes.map(r => r.bead.id),
  failed: failed.map(r => ({ id: r.bead.id, status: r.status, note: r.impl && r.impl.note })),
}
```

- [ ] **Step 2: Syntax-check the script is valid JS**

Run: `node --check .sloppy-pm/delivery.workflow.js` (if `node` present) OR visually confirm it parses. Expected: no syntax errors.

---

## Phase 3 — Verify the deterministic path-guard (before any live run)

### Task 7: Prove the guard defers a forbidden diff and passes a safe one

**Files:** none (a throwaway probe).

- [ ] **Step 1: Unit-check the guard predicate in isolation**

Run (PowerShell — replicates the JS `FORBIDDEN.some(...)` logic):

```powershell
$FORBIDDEN = @('intent/sign.go','intent/verify.go','core/intent.go','state/audit.go','state/sqlite.go','state/redis.go','state/store.go','secrets/broker.go','ee/auth.go','bootstrap/bootstrap.go','engine/engine.go','actuator/actuator.go')
function Guard($files){ foreach($f in $files){ $n=$f -replace '\\','/'; foreach($p in $FORBIDDEN){ if($n -like "*$p*"){ return "DEFER ($p)" } } }; return "PASS" }
Guard @('actuator/webhook.go','actuator/webhook_test.go')   # expect PASS
Guard @('ee/auth.go')                                       # expect DEFER (ee/auth.go)
Guard @('metrics/metrics.go')                               # expect PASS
Guard @('state/audit.go')                                   # expect DEFER (state/audit.go)
```

Expected output: `PASS`, `DEFER (ee/auth.go)`, `PASS`, `DEFER (state/audit.go)`. If any differs, fix `FORBIDDEN` in the workflow script before proceeding.

---

## Phase 4 — Supervised proven run

### Task 8: Run the pipeline (supervised; watch `/workflows`)

**Files:** none (the run produces PRs + merges).

- [ ] **Step 1: Launch the Workflow**

Invoke the `Workflow` tool with `{scriptPath: "C:/Users/i3uph/OneDrive/Documents/GitHub/sloppy-joe/.sloppy-pm/delivery.workflow.js"}`. It runs in the background; watch live with `/workflows`.

- [ ] **Step 2: Supervise against kill-criteria (spec §7.5)**

Halt the run (TaskStop) immediately if you observe: a merge of a forbidden-surface diff; any red commit landing on `origin/main`; the merge-slot held with no progress > ~25 min; > 2 beads going `stage:stuck`. Investigate before resuming.

- [ ] **Step 3: Collect the structured result**

When the `<task-notification>` arrives, read the result: `merged[]`, `deferredForbidden[]`, `needsChanges[]`, `failed[]`.

---

## Phase 5 — Verify promotion criteria & report (NO auto-cron)

### Task 9: Check promotion criteria and write the run report

**Files:** Create `.sloppy-pm/run-2026-06-14.md` (git-excluded).

- [ ] **Step 1: Verify each promotion criterion (spec §9)**

```powershell
Set-Location "C:/Users/i3uph/OneDrive/Documents/GitHub/sloppy-joe"
git fetch origin; git log --oneline origin/main -8     # confirm merged beads present, no junk
gh run list --branch main --limit 8                    # confirm CI green on the merge commits
bd -C . merge-slot check                               # expect: free (released)
git worktree list                                      # expect: no leftover .sloppy-pm/wt-* worktrees
bd -C . list --status in_progress                      # expect: none orphaned
```

Promotion bar (all must hold): ≥1 bead cleanly merged; **zero** red pushes to `main`; **zero** forbidden-surface merges (the guard deferred them instead); merge-slot released; no orphaned worktrees/claims.

- [ ] **Step 2: Write the report** to `.sloppy-pm/run-2026-06-14.md` — merged beads + SHAs, deferred (with the forbidden file that tripped the guard), needs-changes, failures, and a GO/NO-GO on cron promotion. Surface anything that needed a human.

- [ ] **Step 3: STOP. Do not schedule cron.** Promotion to a recurring cron is a **separate, explicitly-approved step** (spec §9): it requires the criteria above to hold AND re-evaluating branch protection (spec §7.1). Report results and await the user's go for promotion.

---

## Self-review (completed by author)

- **Spec coverage:** lifecycle labels/DoR/ready-filter (§4 → Tasks 1–4); Constitution path-guard (§5.1/§6.2 → Task 6 `FORBIDDEN` + Stage 2, verified Task 7); merge gate CI-wait + serialized slot + up-to-date (§6 → Stage 3); bot identity + DCO `-s` + no-AI-trailer (§5.3/§7.2 → Stage 1 commit); `BEADS_DIR` wiring (§7.4 → Stage 1); startup-sweep (§7.7 → Sweep); kill-criteria (§7.5 → Task 8 Step 2); file-collision (§7.6 → discover caps `MAX_BEADS`, one worktree per bead, serialized merge); bounded breadth (§8.1 → Task 3, NOT wired to bootstrap); honest reframe / first-run size (§2/§11.4 → `MAX_BEADS=6`, safe anchors); no-auto-promotion (§9 → Task 9 Step 3). Branch protection deferred (§7.1) — compensating controls present (guard + implementers-never-touch-main + kill-criteria + post-merge re-verify).
- **Placeholder scan:** none — concrete commands, full workflow script, full actuator reference code.
- **Type/name consistency:** `FORBIDDEN`, `MAX_BEADS`, `stage:ready`, `BEADS_DIR`, `pm/<id>` branch, `.sloppy-pm/wt-<id>` worktree, `Bead-Id:` trailer used identically across tasks and the script. `NewWebhook` / `newHTTPRoute` / `routeDest` / `Conformance` match grounding Appendix A2.
- **Open risk to watch at execution:** `go test -race` can't run locally (spec §6.1) — local gate uses plain `go test`; the race+coverage truth is the PR's CI, so Stage 3 waits on `gh pr checks`.
