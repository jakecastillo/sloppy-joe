# Sloppy Joe — Vision & Direction

> Canonical statement of *why this project exists* and *what it is/isn't*. Derived from a deep-research pass and an adversarial red-team of an earlier "AI ops control plane" design (June 2026). The full v0 **design spec** will live in `docs/superpowers/specs/`.

## Refined problem

Teams running AI in production have **already** solved API fragmentation, routing, fallback, and observability (LiteLLM, Bifrost, Envoy AI Gateway, Portkey) and **already** solved generic event→action automation (n8n, StackStorm, Temporal). The single unowned, repeatedly hand-rolled job is the **seam between them**: there is no model-agnostic, self-hostable control point where

1. **model-layer signals** — cost burn, fallback storms, latency/TTFT regression, eval/quality regression, guardrail/PII trips, provider degradation — are **first-class automation triggers**, and
2. the automated **response** is a **governed, capability-scoped, audited, reviewable artifact** rather than an opaque webhook or a budget cron.

So the honest problem is **not** "build an AI ops control plane." It is four loosely-coupled concerns — **route / observe / automate / govern** — three of which are commodities. Sloppy Joe owns exactly one: **automate + govern**, closed as a loop, with cost and policy as native queryable state and the response policy as version-controlled, peer-reviewed, replayable, tamper-evidently-audited infrastructure-as-code.

## The pivot (what changed and why)

An earlier v0 proposed a model-agnostic **gateway** control plane. An adversarial red-team surfaced **9 critical flaws (all independently verified)**. The decisive one was strategic: it was a *sixth Go LLM gateway* competing in a lane already won by Envoy AI Gateway (CNCF) and Bifrost — ~80% of effort would rebuild commodity parity and ship the only differentiator last and thin.

**Decision: do not build the gateway.** The gateway is a free, swappable substrate reached through narrow adapters. Sloppy Joe is the **control loop on top**.

Flaws this dissolves or neutralizes:

| Flaw (v0) | How Sloppy Joe avoids it |
|---|---|
| "Stateless gateway" fiction | We aren't a gateway; our own state is an explicit Store interface with a documented consistency model |
| Retry-storm / metastable cascade | We're off the request hot path; backpressure (global intent budget, jitter, half-open probing) is first-class on the *action* side |
| Credential honeypot (cf. LiteLLM CVE-2026-42208) | Provider keys stay in the gateway; we hold only scoped admin/notify tokens, behind a minimal-surface broker over a local socket |
| Plugin-marketplace supply-chain backdoor | No marketplace / WASM ABI in v0; the only extension surface is the capability-gated Actuator interface |
| OpenAI-as-lingua-franca leakiness | We never normalize inference; we consume vendor-neutral OTel GenAI + CloudEvents |
| Undefined "issue" primitive | Trigger = a typed Signal (OTel GenAI event in a CloudEvents envelope); state = a persisted Incident record |
| Two products in one core | Only the durable, off-hot-path control loop — no gateway/workflow conflation |
| Niche already owned | We sit on top of incumbents, not against them; cross-vendor neutrality is part of the moat |
| Differentiator buried as 1-of-6 | The loop *is* the whole product |

## The single load-bearing invariant

**Nothing acts on the world except by making a mediated call, and every call is capability-checked, budget/quota-accounted, backpressure-gated, and audit-journaled.** This one rule buys — for free — credential containment, supply-chain containment, global backpressure, precise incident semantics, and a minimal stable ABI.

## Architecture (5 layers, library-first Go)

1. **Signal ingest (OBSERVE)** — consume OTel GenAI telemetry + provider health from the gateway you already run; normalize into a typed AI-native Signal taxonomy in a CloudEvents envelope. Cost is *derived* (priced from token-usage metrics) because cost is absent from the OTel GenAI semconv. Zero gateway changes.
2. **State** — one Store interface, swappable backends (embedded SQLite for solo → Redis/Dragonfly atomic for multi-replica). Holds the priced **Cost Ledger**, circuit/dedup state, and a hash-chained **audit log**. Explicit consistency per datum + a deliberate fail-open/fail-closed knob.
3. **Rule engine (DECIDE)** — declarative, Git-versioned Rules (`when <condition over Signals + State> for <window> then <Intent> with <dry_run|approval|intent_budget|rollback>`). Level-triggered reconcile, **replayable in CI** (`test --replay`), every action tagged with its rule git SHA.
4. **Intent + Actuator (ACT)** — a rule emits a signed, **reversible** RemediationIntent (Apply/Revert/Receipt). Capability-scoped Actuators drive the gateway's admin API (LiteLLM first; Bifrost/Envoy behind the same interface) + out-of-band actions (open GitHub issue, page Slack, deploy-gate). The only component touching admin tokens.
5. **Durability (honest)** — **at-least-once + checkpoint-on-SQLite + idempotent actuators keyed by intent-id with receipts.** Not "exactly-once," not an embedded Temporal/DBOS dependency. Still crushes non-durable webhooks; survives crash/restart without double-firing.

## The novel primitive — the Governed AI-Ops Rule

A cost/eval/guardrail breach spawns a governed, capability-scoped, audited remediation whose policy is a Git-versioned, PR-reviewable, replayable artifact, executed as a signed reversible intent any gateway can apply. No incumbent treats the *response to an AI event* as a first-class, diffable, auditable, reversible object.

## Explicitly out of scope for v0 (dropped as premature)

A gateway/router/OpenAI-normalization layer · a WASM plugin ABI · a Dify-style plugin marketplace · an `ee/` open-core split · a Python agent runtime · CNCF/RFC governance "from day one" · embedded Temporal/DBOS · "exactly-once" durability claims.

## Beachhead user

The lone platform / AI-infra engineer at a **20–200-person AI-product company** already running a gateway and at least one local model (Ollama/vLLM), getting paged about provider outages, surprise token bills, and model regressions with no automated, auditable response. **Not** the solo hobbyist; **not** the large enterprise platform team (yet).

## Phase 0 — validate before you build (the defining gate)

The project-defining risk is **unvalidated demand**: do operators actually want response-policy as a reviewed, replayable artifact, or is the n8n + budget-cron + Grafana duct tape "good enough" forever? Before hardening the rule engine: recruit 3–5 design-partner platform engineers, confirm they hand-roll the duct tape and want response-as-artifact, write kill-criteria vs Envoy AI Gateway / LiteLLM / Portkey / n8n / StackStorm, and lock the Signal/Rule/Intent v0.1 schema with them. If the duct tape isn't painful enough to replace — stop or pivot.

## Roadmap (each phase its own spec → plan → build cycle)

- **Phase 0 (wk 0–3):** validate the wedge with design partners; lock schemas.
- **Phase 1 (wk 3–10):** the 10-minute MVP loop on LiteLLM + Ollama (one signal → one rule → one reversible actuator + open-issue + page; cost ledger; replay; audit). Publish Signal/Rule/Intent v0.1 + an Actuator conformance suite.
- **Phase 2 (wk 10–20):** breadth + trust — more signals (eval regression, guardrail trips), Bifrost/Envoy actuators (cross-vendor neutrality = the moat), multi-replica state + backpressure.
- **Phase 3 (wk 20–32, only if pulled):** enterprise — SSO/RBAC/KMS/Postgres behind the same interfaces.
- **Phase 4 (post-PMF):** ecosystem — open the Actuator interface *with* signing/SLSA/digest-pinning from the first external driver.

## Open risks to keep honest

Demand unvalidated · absorption timing (incumbents could ship "rules-in-Git + audit" in a quarter; cross-vendor neutrality + signed receipts is the only structural defense) · gateway admin-API drift · OTel `gen_ai` semconv is still "Development" status · derived-cost accuracy · durability honesty under load.
