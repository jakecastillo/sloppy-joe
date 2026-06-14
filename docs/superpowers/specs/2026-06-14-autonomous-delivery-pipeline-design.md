# Sloppy Joe — Autonomous Multi-Agent Delivery Pipeline (Design)

> **Status:** design, awaiting approval. Written 2026-06-14.
> A self-running PM/BA → research → refine → implement → review → merge pipeline that
> turns the three standing themes and the existing backlog into governed, audited,
> CI-gated changes on `main` **with no human in today's loop** — built on the existing
> `bd` (beads) tracker + git worktrees, driven by a `Workflow` script, promotable to cron.
>
> Every non-obvious claim below is grounded in verified repo facts (see Appendix A:
> file:line citations from the 2026-06-14 grounding pass). This spec deliberately
> *contradicts* an earlier draft where the draft contradicted reality — those
> corrections are called out inline as **[reality check]**.

---

## 1. Purpose & charter

Stand up an autonomous system in which agents act as **project manager / business
analyst**, **researcher**, **refiner (BA)**, **implementer**, and **reviewer**, draining
a `bd` backlog into merged-and-pushed work on `main` without the user having to
interfere. Four decisions are locked (user-confirmed):

| Decision | Value |
|---|---|
| **Charter** | *Steward + sanctioned breadth* — honor the load-bearing invariant and "surfaces, not scope"; work the existing backlog **and** pursue breadth-of-integrations as a now-authorized, **bounded** goal. |
| **Merge gate** | Full CI green **AND** an independent adversarial reviewer-agent approval. No human review. |
| **Cadence** | One **supervised proven run** now → then **promote to cron** (the same Workflow draining `ready` beads). |
| **Push rights** | Agents are authorized to push to `origin` and land all work on `main`. |

**No human in the loop *today*.** The Constitution (§5) is the *autonomy boundary*: items
that would cross it are **deferred** (parked beads), never "wait for a human" and never
auto-merged unreviewed. Override on record: the user may later widen the boundary to put
the security-core in autonomous scope; until then it is out of scope.

### 1.1 Three standing themes (PM/BA decomposes them; user does not hand-spec)

1. **Breadth** — "all popular AI integration configs." **Bounded** (see §8.1): new actuators
   for **existing** `ActionKind`s behind the **existing** `Actuator` interface only.
2. **Easy-to-execute / integrate workflows** — adoption UX (recipes, templates,
   `config init`, integration guides) and developer ergonomics.
3. **Memory + token optimization** — (a) orchestration efficiency (baked into §10) and
   (b) non-invariant product code paths only.

---

## 2. The honest reframe (what the first run can safely do)

**[reality check]** The earlier draft assumed the headline work (integrity gaps, breadth)
was sitting ready in the backlog. Verified reality:

- The four **integrity gaps are already fixed and merged to `main`** (`ad17aef`). There is
  nothing left to "work" there.
- The **one open security follow-up, `sloppy-joe-pir`** ("Harden audit checkpoint against
  signed-checkpoint replay/rollback"), edits `state/audit.go` — a **Constitution-forbidden
  surface**. It is **deferred**, not worked.
- `bd ready` currently returns 14 beads including the **`merge-slot` coordination bead
  itself** and **`pir`** — both must be filtered out (§4.3).

Therefore the **safely-autonomous first-run work is modest and real**: a handful of P2/P3
`go-idiom`/doc/refactor beads, **one new bounded breadth actuator** (created + refined this
run), and **one supervised-only infra bead** that installs the safety net (§7). Breadth
proceeds — but as a *first bounded actuator*, not a sweep. This expectation is set on
purpose: the value of run one is **proving the governed pipeline end-to-end**, not volume.

---

## 3. Architecture

Three pillars:

- **Beads = durable shared state / memory.** Work items, their lifecycle stage, claims,
  dependencies, and history live in `bd`. This is the answer to theme (3) at the
  architecture level: agents are **stateless roles re-instantiated per run**, reading state
  from beads, so no agent carries a giant growing context.
  **[reality check]** `.beads/` is in `.git/info/exclude` (stealth, **local-only**, not
  shared via git). Worktrees do **not** inherit it. *Every* spawned agent that touches the
  tracker MUST be launched with `BEADS_DIR=<main-checkout>/.beads` (or `bd -C <main>`), or it
  silently forks an empty tracker. This is a hard per-agent precondition (§7.4).
- **Workflow script = deterministic execution engine.** One run = one pipeline pass
  (discover → research → refine → gate → implement → review → merge). Pipelined, structured
  outputs, token-disciplined. "Recurring" = re-run the same script under cron.
- **GitHub + CI = the enforcement substrate.** Branch protection + required status checks
  make the merge gate **server-enforced**, not honor-system (§6, §7.1).

### 3.1 Roles

| Role | Does | Key output |
|---|---|---|
| **PM/BA** | Reads vision/roadmap/backlog + the 3 themes; sets charter-aligned priority; seeds epics; enforces Definition-of-Ready + the Constitution | prioritized candidate work-list |
| **Research** (fan-out) | Landscape ("how SJ fits current AI tooling") + per-theme tech research | structured research briefs |
| **BA refine** (fan-out) | Turns candidates + research into beads refined to *Ready*; runs the Constitution path-check | `stage:ready` beads |
| **Implementer** (fan-out, worktree-isolated) | Claims a ready bead, builds to the bar, opens a PR | branch + PR |
| **Reviewer** (per PR, adversarial) | Independent review vs acceptance criteria + Constitution + DoD | approve / request-changes |
| **Merge** (serialized via `merge-slot`) | path-guard + CI-wait + up-to-date + merge → push → `bd close` | merged commit on `main` |

---

## 4. The bead lifecycle (must be built)

**[reality check]** No `stage:*` labels exist today; `bd ready` is **blocker-based**, not
stage-based. The lifecycle below is **introduced by this system** as a label convention plus
a Definition-of-Ready (DoR) wrapper. `bd` natively supports labels, deps, atomic `--claim`,
`--actor`, `--json`, and the `merge-slot` pattern (Appendix A4) — no workarounds needed.

### 4.1 States (label convention on top of `bd` status)

```
stage:backlog → stage:research → stage:refining → stage:ready
  → in_progress (bd --claim) → stage:review (PR open) → closed (merged+pushed)
                                         ↘ stage:deferred / needs-human (parked)
                                         ↘ stage:stuck (N failed attempts)
```

`stage:ready` is asserted by a refiner **only when** the Definition-of-Ready holds.

### 4.2 Definition of Ready (DoR) — all required

- Clear problem statement + explicit **acceptance criteria** (`bd ... --acceptance`).
- Scope boundary (in/out) and **touched-file estimate** (drives §7.6 collision check).
- **Constitution path-check passed** — does not touch any forbidden path/symbol (§5.1);
  if it does → `stage:deferred` + `needs-human`, never `ready`.
- Size (S/M/L, `--estimate`) and dependencies linked (`bd dep`).
- **Test plan that holds coverage ≥ 72%** (the verified CI floor, §6.1) — a new actuator
  must ship a `actuator.Conformance` test + behavior tests.

### 4.3 Ready-set filter (what the pipeline may pull)

The implementable set = `bd ready` **minus**:

- coordination/control beads: label `gt:slot`, `coordination`, the `merge-slot` bead;
- forbidden-surface beads: any bead whose touched-file estimate hits a §5.1 path
  (e.g. **`sloppy-joe-pir`** → `state/audit.go`; **`sloppy-joe-6e6`** → `secrets/broker.go`
  interface change) → relabel `stage:deferred` + `needs-human`;
- beads already `stage:stuck` (attempt cap reached, §7.5);
- beads whose file set overlaps an in-flight bead (§7.6) — held, not dropped.

---

## 5. The Constitution (hard autonomy boundary)

Agents operate freely **except** across these lines. A crossing → **auto-defer** (park the
bead `needs-human`), **never** auto-merge. Enforcement is a **deterministic guard**
(§6.2) independent of any LLM, plus the adversarial reviewer as a second layer.

### 5.1 Forbidden paths / symbols (concrete — the guard's allowlist-of-deny)

Any diff touching these is auto-deferred (verified surfaces, Appendix A1/A2):

| Surface | Paths / symbols |
|---|---|
| Intent signing & canonical form | `intent/sign.go`, `intent/verify.go`, `core/intent.go` → `CanonicalBytes`, `ActionKind`, `KnownActionKinds` |
| Tamper-evident audit chain | `state/audit.go` → `ChainHash`, `CheckpointPayload`, `MakeCheckpoint`, `VerifyChain`, `VerifyAgainstCheckpoint` |
| Idempotency / at-most-once | `state/sqlite.go` + `state/redis.go` → `ClaimIntent`, `ReleaseIntent`, `AppendAudit`; `engine/engine.go` `applyIntent` claim/sign/audit lines |
| Credential broker (default-deny) | `secrets/broker.go` → `Broker` interface, `NewEnvBroker`; `bootstrap/bootstrap.go` `BuildRegistry` allowlist |
| Fail-closed RBAC | `ee/auth.go` → `ScopeForPath`, `HasScope`, `Middleware` |
| Actuator contract | the `actuator.Actuator` interface signature; adding `ActionKind` constants |

### 5.2 Categorical prohibitions (beyond the path list)

- Don't break the **load-bearing invariant** (every action mediated, capability-checked,
  budget-accounted, audit-journaled).
- Don't make SJ a **gateway** / put it **on the hot path** / hold **provider keys**.
- No **heavy deps** / no breaking the static `CGO_ENABLED=0` posture (no Temporal/DBOS/WASM/
  marketplace).
- `LICENSE` / security model / governance unchanged.
- **No "do-not-do" DRY refactors** (Appendix A5 decline-list: Bifrost/Envoy divergence,
  SQLite-vs-Redis backends, per-failure engine branches, `handleUsage`/`handleOTLP`,
  app.Build, route→scope table location, constant-time API-key compare).
- No `bd setup claude`; no edits to tracked `.claude/settings.json` or hooks.
- **No editing the audit/bead trail the agents operate over** (no truncating `.beads`
  history or the `sloppy audit` chain). Every autonomous action (claim, merge, push) is
  itself journaled with run-id + bead-id.
- **`fail-closed` stays fail-closed** — no diff may flip a default-deny (`ScopeDeny`, broker
  deny, mutating-endpoint guard) to fail-open.

### 5.3 Always-do

- Conventional commit + **DCO `-s`** sign-off; **never** an AI co-author / `generated-with`
  trailer (verified: commit-msg hook + commit-lint reject these, §6.1).
- Meet the satisfied bar (§6.1). Keep docs DRY (one row in `docs/README.md` per new doc).
- Preserve "surfaces, not scope."

---

## 6. The merge gate

Three layers, in order. A PR merges only if **all three** pass.

### 6.1 Layer 1 — Full CI green (server-enforced)

Required checks (verified, Appendix A3): **lint** (`go vet`, `yamlfmt -lint`,
`shfmt -d`, `golangci-lint run` with 14 linters, `actionlint`) · **test**
(`go test -race -covermode=atomic`, **coverage ≥ 72%**) · **build** (ubuntu/macos/windows,
`CGO_ENABLED=0`) · **govulncheck** · **CodeQL** · **commit-lint** (conventional + DCO,
reject AI trailers).
**[reality check]** `go test -race` does **not** run on this Windows machine (no C
compiler). Agents self-verify with plain `go test ./...` + build + lint locally, but the
**race + coverage truth is the PR's CI run** — so the gate *waits on GitHub CI*, it does not
trust local green alone.

### 6.2 Layer 2 — Deterministic Constitution path-guard (no LLM)

Compute the PR's touched files (`gh pr diff --name-only`). If any match §5.1 → **hard fail
→ auto-defer** (close PR, relabel bead `stage:deferred` + `needs-human`). Also grep the diff
for hardcoded `token`/`Bearer` literals and for new `ActionKind` constants outside
`core/intent.go` → fail. This runs **before** the LLM reviewer and cannot be talked out of
it by a persuasive diff.

### 6.3 Layer 3 — Adversarial reviewer-agent

Independent agent receives the **touched-file list + the unified diff + the bead's
acceptance criteria + the Constitution**. It is prompted to *refute* (default to
request-changes when uncertain). Verdict `approve` / `request-changes`. On request-changes →
bounded retry (≤ 2) back to the implementer; then `stage:stuck`.

### 6.4 Merge mechanics (serialized, race-safe)

Inside the `merge-slot` (acquire → … → release):

1. Ensure the PR branch is **up to date with `origin/main`** (auto-update / rebase);
2. Wait for required checks (`gh pr checks --watch`, bounded timeout);
3. On green + reviewer-approved + guard-clean → `gh pr merge --squash --auto` (server
   enforces "branches up to date" so a stale-base merge is refused);
4. After merge: `git fetch`, `bd close <id>` with reason, **release slot**.
5. On any failure (conflict, red recheck): deterministic cleanup — leave/retry per attempt
   cap, **always release the slot**, prune the worktree.

---

## 7. Safety preconditions & operational guards

These make "unattended" safe. The infra ones ship/verify **under supervision in run one**,
before promotion to cron.

### 7.1 Branch protection on `main` (precondition)  ⚠ outward-facing

**[reality check]** `main` is unprotected (verified `404`). Before any unattended push,
install a ruleset on `main`: **require the status checks** in §6.1, **disallow direct
pushes**, require linear history. *Review is enforced by our gate (§6.3), not GitHub* — a
required-review rule can't be satisfied (the bot is the PR author and there's no second
human/App reviewer), so we require checks + no-direct-push and let the orchestration enforce
review. Admin (the user) may bypass. **This changes the user's own workflow** (they'd push
via PR or admin-bypass) → flagged for explicit approval at the review gate (§11).

### 7.2 Commit provenance / identity

Commit under a **single sanctioned git identity** with `-s` (DCO), a `Bead-Id:` trailer for
provenance, and **never** an AI trailer. Identity choice (user's own vs a dedicated bot
identity) is an open decision (§11).

### 7.3 GitHub orchestration token boundary

The push/PR/merge token lives **outside** the sloppy-joe broker (which only governs
`SLOPPY_TOKEN_<CAP>` for actuators). Use a **fine-grained, repo-scoped, least-privilege**
token (contents + PR write), injected via env at run start, **never** written to beads,
audit, or transcripts (secret-scrub gate). Today's `gh` is authed as `jakecastillo` with
broad `repo`+`workflow` scopes — narrowing this is part of §7.1's precondition work.

### 7.4 `BEADS_DIR` wiring

Every spawned agent gets `BEADS_DIR=<main>/.beads` (or `bd -C <main>`). No exceptions —
worktree agents otherwise fork an empty tracker.

### 7.5 Retry bounds, per-run budget, kill-criteria

- **Attempt cap** per bead = 3 → then `stage:stuck` + stop touching it.
- **Per-run budget:** max beads, max wall-clock, max CI re-runs (mirrors the product's own
  "global intent budget + jitter" backpressure philosophy).
- **Kill-criteria (halt + notify):** any forbidden-surface auto-merge *attempt*; a red push
  reaching `main`; `merge-slot` held past TTL; > N `stage:stuck` beads. Analogous to the
  product's `docs/phase0/kill-criteria.md`.

### 7.6 File-collision avoidance

Derive each ready bead's touched-file set; **never dispatch two in-flight beads with
overlapping files** concurrently (serialize via a soft-lock label or `bd dep`). Prevents the
guaranteed-conflict case (e.g. several `go-idiom` beads all touch `actuator/*`).

### 7.7 Cron resume / crash recovery (startup sweep)

Idempotent first step of every run: `git fetch`; `git worktree prune` + remove known-stale
worktrees; reset `in_progress` beads with a dead run-id back to `stage:ready` (or `stuck`
after N); **force-release** the `merge-slot` if its holder run-id is dead. Stamp every
claim/slot with the run-id so staleness is detectable.

---

## 8. The themes, bounded

### 8.1 Breadth — bounded to the sanctioned path

**[reality check]** The vision gates *opening the Actuator interface to external drivers* at
Phase 4 (with signing/SLSA); Bifrost/Envoy are intentionally **experimental**. So "all
popular AI integration configs" is scoped to: **new actuators for EXISTING `ActionKind`s,
reusing `httpRouteActuator` + the `TokenFunc` broker, passing `actuator.Conformance`,
default-off behind an `enabled` flag.** Candidates: Portkey, OpenRouter, a generic-webhook
route_override, vLLM/Ollama admin shapes. **Adding a new `ActionKind` is a schema/governance
change → deferred** (`needs-human`). This keeps breadth real **and** invariant-safe.

### 8.2 Easy workflows

Adoption UX: recipes/templates/`config init`/integration guides (leverages existing
`recipe`/`bootstrap`/`config-init` work) + developer ergonomics. Refined by BA/research into
concrete beads; non-invariant by construction.

### 8.3 Memory + token optimization

Bounded to (a) **orchestration efficiency** (§10) and (b) **non-invariant product paths**
(e.g. bounded in-memory structures off the hot path). Explicitly **must not** touch intent
serialization / `CanonicalBytes`, the audit chain, credential resolution, or the
Signal/Rule/Intent schema — the tempting-but-forbidden targets.

### 8.4 Demand-gating, surfaced not skipped

The PM seeds a `theme:validation` epic capturing the Phase-0 demand question and tags
breadth beads with the demand-risk note, so the steward-vs-validate tension lives **in the
tracker**, visible, never silently overridden.

---

## 9. The proven run + promotion criteria

**Run one (supervised):**

1. Startup sweep (§7.7) + install safety net (§7.1–7.3) — **infra bead, supervised**.
2. Discover → filtered ready-set (§4.3) + seed epics for the 3 themes.
3. Research (parallel) → refine a small batch to `stage:ready` (incl. **one bounded breadth
   actuator**, §8.1).
4. Implement (parallel worktrees) → review → merge for the §2 first-run beads.
5. Report.

**Promotion criteria → cron (all must hold):**

- ≥ N beads closed cleanly (PR → CI green → reviewer-approved → merged → pushed);
- **zero** red pushes to `main`; **zero** forbidden-surface auto-merge attempts;
- `merge-slot` always released; no orphaned worktrees/claims left;
- branch protection (§7.1) active and observed to block a direct push.

Only then is the same Workflow wired to cron to drain `stage:ready` continuously.

---

## 10. Token / memory discipline (orchestration)

- **Beads as memory** → stateless roles, no giant contexts (the biggest lever).
- **Structured outputs (schemas)** between stages — no prose dumps.
- **Explore agents** for search; reviewers read **diffs**, not trees.
- **Pipeline, not barrier**, so wall-clock + idle minimized; parallelism capped.
- **Per-run token budget guard**; loop bounded by the ready-set + §7.5 budget.

---

## 11. Open decisions to confirm at the spec-review gate

1. **Branch protection on `main` (§7.1)** — outward-facing; changes the user's own push
   workflow. Approve enabling it (required checks + no-direct-push, admin bypass)? *Strongly
   recommended — it's the safety net that makes unattended push safe.*
2. **Commit identity (§7.2)** — user's own git identity (`Jake Castillo
   <jakecast@hawaii.edu>`) with `-s`, or a dedicated bot identity? (Both DCO-honest; bot is
   cleaner provenance.)
3. **First-run breadth target (§8.1)** — which provider for the first bounded actuator
   (Portkey / OpenRouter / generic-webhook)? Default: generic-webhook `route_override`
   (lowest external-API risk).
4. **First-run size / budget (§7.5)** — default ≈ 5–6 safe beads + 1 breadth actuator + the
   infra bead; adjust N?

---

## 12. Out of scope / deferred

The security-core (§5.1) under autonomy; new `ActionKind`s; opening the Actuator interface to
*external* drivers (Phase 4, needs signing/SLSA); Postgres/Sigstore/xDS YAGNI parks;
`sloppy-joe-pir` and `sloppy-joe-6e6` (forbidden-surface) until a supervised day.

---

## Appendix A — Verified facts (2026-06-14 grounding pass)

**A1 — Security invariants.** `intent/sign.go:37 Sign`, `:41 Verify`, `:66 LoadOrCreateSigner`;
`core/intent.go:53 CanonicalBytes`; `intent/verify.go:27 AppliedAuditDetail`, `:37
VerifyAuditDetail`; `state/audit.go:24 ChainHash`, `:43 VerifyChain`, `:79 CheckpointPayload`,
`:116 MakeCheckpoint`, `:139 VerifyAgainstCheckpoint`; `state/sqlite.go:63 ClaimIntent`, `:99
AppendAudit`; `state/redis.go:46 ClaimIntent`, `:73 AppendAudit`; `secrets/broker.go:12
Broker`, `:19 NewEnvBroker`; `ee/auth.go:69 ScopeForPath`, `:100 HasScope`, `:125 Middleware`;
`engine/engine.go:286` sign, `:298` claim, `:338` audit.

**A2 — Actuator contract.** `actuator/actuator.go` (`Actuator` interface = `Capabilities()
[]ActionKind`, `Apply`, `Revert`; `Registry`), `actuator/conformance.go` (`Conformance`),
`actuator/httproute.go` (`httpRouteActuator`, `TokenFunc`, `requestFn`), `core/intent.go`
(`ActionKind` enum = route_override/open_issue/page/throttle_tenant/disable_deployment;
`KnownActionKinds`), `core/receipt.go` (`Receipt`/`Outcome`), `bootstrap/bootstrap.go`
(`BuildRegistry`). Existing actuators: LiteLLM, GitHub, Slack, Bifrost, Envoy, Log, Fake.

**A3 — Satisfied bar.** `make ci = fmt-check vet lint lint-actions vulncheck test-race`;
CI jobs lint / test (race + coverage **≥ 72%**) / build (3 OS, `CGO_ENABLED=0`) / govulncheck
/ CodeQL / commit-lint (conventional + DCO `-s`, reject AI trailers). 14 golangci linters.

**A4 — `bd` capabilities.** labels (`bd label …`), deps (`bd dep …`), `bd ready [--claim]`,
`bd update --claim/--status/--add-label/--acceptance`, `bd create -p -l -d --acceptance
--deps --estimate`, `bd merge-slot create|check|acquire|release`, `--actor`/`$BEADS_ACTOR`,
`--json`. No missing capability.

**A5 — Do-not-DRY decline-list.** Bifrost/Envoy intentional divergence; SQLite-vs-Redis
backends; per-failure-mode engine branches; `handleUsage`/`handleOTLP`; unified `app.Build`;
route→scope table location; bespoke constant-time API-key compare.

**A6 — Verified environment.** `origin = github.com/jakecastillo/sloppy-joe`; `gh` authed
(`jakecastillo`, `repo`+`workflow`); `go1.26.4` on PATH; `main` **unprotected** (`404`); 11
bd labels, **no `stage:*`**; `bd ready` (14) includes `merge-slot` + `pir`.
