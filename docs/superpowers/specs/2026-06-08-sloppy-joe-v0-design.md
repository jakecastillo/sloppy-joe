# Sloppy Joe — v0 (Phase-1 MVP) Design Spec

**Status:** Approved design · **Date:** 2026-06-08 · **Supersedes:** the rejected "AI ops gateway control plane" v0 (see `docs/vision.md`).

This is the canonical design for the first implementable slice. The implementation plan derives from this document.

---

## 1. Overview

Sloppy Joe is an open-source, model-agnostic **AI-ops control loop**. It sits *on top of* an existing LLM gateway and turns model-layer signals (cost burn, latency/quality regression, fallback storms, guardrail trips, provider degradation) into **governed, audited, Git-reviewed automated responses**. It is **not** a gateway and is **never** on the inference request hot path.

**Novel primitive — the Governed AI-Ops Rule:** a cost/eval/guardrail breach spawns a governed, capability-scoped, audited remediation whose policy is a Git-versioned, PR-reviewable, replayable artifact, executed as a signed reversible intent any gateway can apply.

**Single load-bearing invariant:** *nothing acts on the world except by a mediated call that is capability-checked, budget-accounted, backpressure-gated, and audit-journaled.*

### Non-goals (v0)
No gateway/router/normalization; no WASM ABI or plugin marketplace; no `ee/` open-core split; no Python agent runtime; no Redis/multi-replica; no "exactly-once" durability claim; no embedded Temporal/DBOS.

## 2. Scope & success criteria

**In scope:** the full 5-layer observe→decide→act loop; a LiteLLM reference Actuator; YAML+CEL rules; a derived cost ledger; replay-in-CI; a hash-chained audit log; at-least-once idempotent durability; the `sloppy` CLI + `sloppyd` daemon.

**Success criteria:**
1. A single rule closes the loop on real **LiteLLM + Ollama** in under 10 minutes from install.
2. The loop survives `kill -9` + restart with **no double-firing** of a remediation.
3. Every automated action is signed, appended to a tamper-evident audit log, and reviewable as a Git diff (rule SHA recorded on each action).
4. `sloppy test --replay` deterministically reproduces what a rule *would* do against recorded telemetry, with no production effect.
5. The running system doubles as the live demo for design-partner conversations (Phase 0, run in parallel).

## 3. Architecture

```
        ┌─────────────────────────────────────────────┐
        │  LiteLLM (operator already runs it)          │
USERS ─▶ │  serves inference · emits OTel · admin API   │ ◀─ provider keys live HERE
        └───────┬───────────────────────────▲─────────┘
          OTel / metrics                admin calls (reroute, etc.)
                │                             │
        ┌───────▼─────────────────────────────┴─────────┐
        │  SLOPPY JOE (sloppyd) — off the request path   │
        │  ingest → state → rules(CEL) → intent → actuate │
        │   SQLite: cost ledger · dedup · hash-chain audit │
        │   secret broker (admin/GitHub/Slack tokens only)│
        └───────┬─────────────────────┬──────────────────┘
            open issue (GitHub)    page/notify (Slack)
```

The daemon reacts on its own clock (seconds), never on the inference critical path. This structurally eliminates the v0 "stateless gateway" and retry-storm flaws: Sloppy Joe owns no hot-path state and issues no inference requests.

## 4. Module boundaries (Go, library-first)

`libsloppyjoe` is the importable core — everything except `cmd/`.

| Package | Responsibility | Depends on |
|---|---|---|
| `core` | Shared types: `Signal`, `Incident`, `Rule`, `Intent`, `Receipt` | (none) |
| `ingest` | OTLP push receiver + LiteLLM admin/spend poller → normalized `Signal`s | `core` |
| `state` | `Store` interface; `sqlite` backend; `CostLedger`; `AuditLog` (hash-chain) | `core` |
| `rules` | YAML loader, CEL compile/eval, level-triggered reconciler, replay engine | `core`, `state` |
| `intent` | Build/sign `Intent`s; the `Apply / Revert / Receipt` contract | `core`, `secrets` |
| `actuator` | `Actuator` interface + adapters `litellm`, `github`, `slack` | `core`, `intent`, `secrets` |
| `secrets` | Broker: token store, JIT decrypt, zeroize (in-proc, sidecar-ready) | `core` |
| `cmd/sloppy` | CLI | all of `libsloppyjoe` |
| `cmd/sloppyd` | Daemon (continuous reconcile) | all of `libsloppyjoe` |

Each unit is independently testable and communicates through a narrow interface. The `Store`, `Actuator`, and `secrets.Broker` interfaces are the seams that later allow Redis, additional gateways, and a sidecar broker without touching callers.

## 5. Data model (schemas)

**Signal** — an OTel GenAI event wrapped in a CloudEvents envelope:
```
Signal {
  id            string            // CloudEvents id
  source        string            // e.g. "litellm/otel"
  type          string            // e.g. "cost.budget_burn", "latency.regression", "fallback.fired"
  time          timestamp
  subject       { tenant, deployment, alias string }
  severity      enum(info|warning|critical)
  correlation_key string          // for dedup/incident grouping
  dedup_window  duration
  evidence      []Evidence        // otel span links, metric snapshots
  data          map[string]any    // type-specific payload
}
```

**Incident** — persisted, correlated lifecycle object:
```
Incident { id, signal_ref, status enum(open|ack|resolved), opened_at,
           timeline []Event, rule_sha string }
```

**Rule** — declarative, Git-versioned (YAML + CEL); see §6.

**RemediationIntent** — signed, reversible:
```
Intent { id, kind, target, args map, ttl duration,
         evidence []Evidence, rule_sha, signature }
```

**Receipt** — proof of an applied/failed action:
```
Receipt { intent_id, actuator, applied_at, before any, after any,
          outcome enum(applied|reverted|failed), signature }
```

## 6. Rule language (YAML + CEL)

Outer structure is YAML; the `when` condition is a [CEL](https://github.com/google/cel-go) expression evaluated against a typed context. CEL is sandboxed and side-effect-free, which keeps rules deterministic and therefore replayable.

```yaml
# rules/cost-guard.yaml
on: cost.budget_burn          # Signal type this rule subscribes to
when: signal.tenant == "acme" && state.spend_1h_usd > 5.0
for: 5m                        # condition must hold this long (level-triggered)
then:
  - route_override: { alias: gpt-4o, to: ollama/llama3, ttl: 30m }
  - open_issue:     { repo: acme/ops }
  - page:           { slack: "#oncall" }
with:
  dry_run: false
  intent_budget: 3/h           # max remediations/window (backpressure)
  rollback: on_clear           # auto-revert when condition clears or ttl expires
```

**CEL context (read-only):**
- `signal` — the triggering `Signal` (subject fields flattened: `signal.tenant`, `signal.alias`, `signal.severity`, plus `signal.data.*`).
- `state` — queryable derived state: `state.spend_1h_usd`, `state.spend_24h_usd`, `state.error_rate_5m`, `state.p95_latency_ms`, `state.circuit[provider]`.

**Reconciliation:** level-triggered (Kubernetes desired-state style), not edge-triggered. The reconciler computes desired vs. observed each tick; a rule already-fired and still-true does not re-fire (idempotent), and recovery is gated (see §10). Every fired action records the rule's git SHA.

## 7. Data flow

1. **Observe** — `ingest` receives OTLP telemetry (push) and polls LiteLLM admin/spend; normalizes to a `Signal`. Token-usage metrics feed the `CostLedger`.
2. **Decide** — reconciler dedups by `correlation_key` + `dedup_window`, opens/updates an `Incident`, evaluates matching rules' CEL against `signal` + `state`. A rule whose `when` holds for `for` produces an Action Plan.
3. **Act** — the plan is filtered by governance (`dry_run`, `intent_budget`, approval, backpressure); surviving actions become signed `Intent`s; the `Actuator` applies each via the broker's scoped token (LiteLLM reroute; GitHub issue with evidence + rule SHA; Slack page).
4. **Record** — each Apply/Revert/failure yields a signed `Receipt` appended to the hash-chained audit log and linked to the `Incident` timeline; the audit entry is also re-exported as OTel so it appears in the operator's existing dashboards.
5. **Revert** — on `ttl` expiry or condition-cleared (`rollback: on_clear`), the reversible `Intent` is reverted and the `Incident` transitions to `resolved`.

## 8. Cost ledger

Cost is **absent from the OTel GenAI semconv**, so it is derived: `token-usage metrics × a static, versioned price-book YAML`, aggregated in windows (1h/24h) per tenant/key/deployment/model. Values are surfaced as **"estimated"** with a confidence note. Strict budgets may `fail-closed` only when explicitly configured. The price book is a first-class, user-editable file; staleness and provider pricing drift are documented limitations (see §17). The ledger is the state that `cost.*` signals and `state.spend_*` CEL fields read from.

## 9. Durability & state

**Model:** at-least-once delivery + SQLite checkpoint + idempotent actuators keyed by `intent_id` + signed receipts. This is deliberately **not** "exactly-once" and uses **no** external workflow engine.

- On restart, in-flight `Incident`s resume; re-issued `Intent`s are deduplicated by the actuator via prior receipts (no double reroute).
- Single-replica (embedded SQLite) for v0. The `Store` interface keeps a Redis/Dragonfly atomic backend a Phase-2 swap, not a rewrite.
- **Store-unreachable behavior is a deliberate `fail_open` / `fail_closed` knob** (default `fail_open`), never an accident.
- The audit log is append-only and **hash-chained** (each entry includes the prior entry's hash) so tampering is detectable.

## 10. Security model

- **Provider keys never enter Sloppy Joe — they remain in LiteLLM.** Sloppy Joe holds only scoped **admin / GitHub / Slack** tokens.
- Tokens live in the **secret broker**: JIT decrypt per use, zeroize after, sourced from env / file / KMS reference. In v0 the broker is an isolated in-process module with **no network surface**, architected to be promoted to a unix-socket sidecar process at scale.
- Every action is **capability-checked** against the actuator's manifest and **audited**; bulk token reads are alertable.
- **Backpressure** is first-class on the action side: per-rule `intent_budget`, full/decorrelated **jitter** on retries, and dedup windows. Because remediations are governed and rate-limited, the action layer cannot become an amplifier.

## 11. Actuators (v0 set)

| Actuator | Action | Reversible? |
|---|---|---|
| `litellm` | `route_override` (admin API) | Yes — via `ttl` / explicit Revert |
| `github` | `open_issue` (with evidence + rule SHA) | n/a (issue stays as record) |
| `slack` | `notify` / `page` | n/a |

Each adapter ships a **capability manifest** declaring which actions and network hosts it uses and whether it touches secrets (default-deny). Unsupported actions **degrade gracefully** to `notify` / `open_issue` rather than failing silently. The `Actuator` contract is `Capabilities() / Apply() / Revert() / Receipt()`.

## 12. Error handling

| Failure | Behavior |
|---|---|
| Store unreachable | `fail_open`/`fail_closed` knob; log + OTel alert |
| Actuator Apply fails | retry within `intent_budget` with jitter; record a `failed` receipt; never bypass circuit state |
| Gateway admin-API drift | adapter capability-probe at startup; degrade to `notify`/`open_issue`; honest support matrix |
| Crash mid-incident | resume on restart; idempotent dedupe by `intent_id` via receipts |
| Malformed rule | rejected at `rules apply`/`validate` with line-level error; never partially loaded |

## 13. Self-observability

`sloppyd` exports its own OTel metrics/traces (signals ingested, rules evaluated, intents applied/reverted/failed, ledger lag) so operators monitor Sloppy Joe with the same stack they already run. Audit entries are re-exported as OTel logs.

## 14. Testing strategy (TDD)

- **Unit:** CEL evaluation, ledger pricing math, dedup/correlation, **hash-chain integrity**, idempotency keys.
- **Replay / golden:** recorded telemetry fixtures → expected `Intent`s; deterministic, runs in CI (`sloppy test --replay`).
- **Integration:** dockerized **LiteLLM + Ollama**; drive a real cost spike; assert reroute + GitHub issue + signed receipt + auto-revert + **crash-resume without double-fire**.
- **Conformance:** the published **Actuator `Apply/Revert/Receipt`** suite, so a second actuator implementation has a contract to satisfy.

## 15. CLI surface

```
sloppy up                      # start the daemon (or `sloppyd run`)
sloppy rules apply <dir>       # load/reconcile rules from a directory (Git-tracked)
sloppy rules validate <dir>    # lint + type-check rules (CI gate)
sloppy test --replay <fixture> # dry-run rules against recorded telemetry
sloppy audit tail|query        # read the hash-chained audit log
sloppy doctor                  # check LiteLLM/OTel connectivity + adapter capabilities
sloppy inject <signal.json>    # inject a synthetic signal (demo/testing)
```

## 16. Configuration

A single `sloppy.yaml`: gateway endpoint(s) + admin-token reference, OTLP receiver bind address, LiteLLM poll interval, state path (SQLite file), price-book path, `fail_open`/`fail_closed`, default `intent_budget`, secret sources. Rules live separately in a Git-tracked directory.

## 17. Decisions & defaults

| Decision | Choice | Rationale |
|---|---|---|
| Brand / command | "Sloppy Joe" / `sloppy` | `joe` collides with the classic Unix editor; `sloppy` is clean and on-brand |
| Public repo handle | finalize at first publish (`sloppyjoe` candidate) | `sloppy-joe` is taken by an unrelated CLI; not needed for implementation |
| Language | Go, library-first | constraints + research; single static binary |
| First gateway | LiteLLM | most-adopted OSS; mature admin API; biggest beachhead overlap |
| Rule language | YAML + CEL | declarative, sandboxed, deterministic, familiar (k8s/Kyverno) |
| State backend | SQLite only (v0) | single-binary, zero-dep; Redis is a Phase-2 swap |
| Ingest | OTLP push receiver + LiteLLM admin poll | works against unmodified gateways |
| Secret broker | in-proc, isolated, sidecar-ready | smallest surface for v0; promotable |
| License | Apache-2.0 | permissive + patent grant; foundation-friendly |

## 18. Open risks (tracked, not hidden)

Demand unvalidated (mitigated by the parallel Phase-0 track) · absorption timing (incumbents could ship "rules-in-Git + audit"; cross-vendor neutrality + signed receipts is the defense) · gateway admin-API drift · OTel `gen_ai` semconv is still "Development" status (pin a version, isolate the mapping) · derived-cost accuracy · durability honesty under crash/load.

## 19. Phase-0 parallel track (not a build gate, per decision)

Run alongside implementation: recruit 3–5 design-partner platform engineers at 20–200-person AI companies; confirm they hand-roll the n8n + budget-cron + Grafana duct tape and want response-as-artifact; write a kill-criteria comparison vs Envoy AI Gateway / LiteLLM / Portkey / n8n / StackStorm; pressure-test the Signal/Rule/Intent schema with them. The running MVP is the demo.

## 20. Out of scope / roadmap

- **Phase 2 (wk 10–20):** more signals (eval/quality regression, guardrail/PII trips), Bifrost + Envoy AI Gateway actuators (cross-vendor neutrality = the moat), `throttle_tenant`/`disable_deployment` intents, Redis multi-replica + global backpressure.
- **Phase 3 (wk 20–32, only if pulled):** enterprise — SSO/RBAC, KMS-backed sidecar broker, Postgres, audit/SIEM export, behind the same interfaces.
- **Phase 4 (post-PMF):** open the Actuator/Driver interface to third parties **with** mandatory signing (cosign/Sigstore) + SLSA provenance + digest-pinning + capability manifests from the first external driver; consider standardizing the Intent spec only if 2–3 independent implementations appear.
