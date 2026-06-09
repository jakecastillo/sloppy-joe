# Sloppy Joe — Audience & Expansion (demand-gated)

> Companion to [`vision.md`](vision.md). The vision doc names **one** user — the
> lone platform engineer — on purpose. This doc records *who else gets value from
> the same loop* and *how we would widen onto them later*. *None of the expansion
> below is built, committed, or scheduled.* It is gated behind Phase-0 validation
> and a real pull from each role. It exists so the roadmap can grow without
> re-litigating the wedge, and so we never confuse "narrow beachhead" with "small
> market."

## The distinction that makes the whole thing legible

There are two different questions, and conflating them is what produces the
misleading "is this only for platform engineers?" / "can anyone use it?" debate:

- **Who *operates* it** — installs `sloppyd`, writes CEL rules, wires the gateway
  and OTel, holds the admin tokens. This is, and for the foreseeable future
  remains, the **platform / AI-infra engineer**. Operating a Go control loop
  bolted to a gateway is infra work; that ceiling is a *category* truth (it is
  equally true of Prometheus, Vector, and OPA), not a Sloppy Joe limitation.
- **Who *benefits* from it** — reads the audit trail, gets the cost breach
  contained, can prove to an auditor what fired and why. This is a **much wider
  set of roles**, and today **none of them have a surface of their own.**

The strategic point: **beachhead ≠ total market.** We land with the one role that
can adopt the tool cold, then widen by giving the roles who already care about the
*output* a purpose-built lens onto it — *not* by making CEL easier for everyone.

## Personas

| Role | What they care about in the loop | Surface today |
|---|---|---|
| **Platform / AI-infra engineer** *(operator, beachhead)* | Writes rules, runs the daemon, owns the gateway wiring and tokens | ✅ full — CLI, daemon, rules, replay, audit |
| **FinOps / finance** | The priced cost ledger, budget breaches, spend auto-remediation | ❌ none — needs the engineer to translate |
| **Security / compliance / GRC / auditor** | The tamper-evident audit chain; "which policy version fired, when, and why"; policy-as-reviewed-artifact | ❌ none — `sloppy audit tail` on a CLI is not an auditor's surface |
| **SRE / on-call** | Gets paged; reviews intent diffs in PRs; runs `test --replay` in CI | ⚠️ partial — CLI + replay exist; could co-own rules |
| **Eng lead / CTO** | Incidents auto-remediate; a defensible paper trail exists | ❌ none — beneficiary, no view of their own |

The demand drivers we are riding are owned by the **bottom four rows**, not the
top one: FinOps practitioners now near-universally manage AI spend, and the
audit/provenance need is regulation-backed (EU AI Act Art. 12 lifetime event
logging, in force Aug 2026; SOC 2 / ISO 42001 change-log requirements). The
platform engineer *installs* Sloppy Joe; the FinOps lead and the compliance owner
are who make it *budgeted and renewed*. Right now they cannot see it.

## Expansion plan (post-PMF, each step demand-gated)

Sequenced by leverage. Each step adds a **read or approve surface** onto the
single existing loop — it does **not** add scope to the engine, and every action
still flows through the one load-bearing invariant (nothing acts except by a
mediated, capability-checked, budget-accounted, audit-journaled call).

- **A. FinOps / compliance read-only surface** *(highest leverage — maps straight
  onto the verified cost-governance + audit demand).* A `sloppy report` /
  exportable view of the cost ledger and the audit chain — "what breached, what
  fired, under which rule SHA, reverted when." Hands FinOps and auditors their own
  artifact with zero CEL. No new write path, so no new risk surface.
- **B. "What would have fired" PR-comment bot** *(serves eng leads + SRE).* On a
  rule-change PR, run `test --replay` against recent traffic and post the diff in
  plain language ("this change would have rerouted 3× and paged once last week").
  Turns policy review into something non-engineers can reason about — the wedge's
  core promise made legible to reviewers.
- **C. Approval-gate UI / Slack flow** *(serves non-engineer approvers).* Built
  over the already-parked `ee/` RBAC + approval gates: let a FinOps or eng lead
  *approve* a pending remediation without authoring one. The first surface where a
  non-operator *acts* — gated, audited, reversible like everything else.

> Later, further out: a hosted/managed offering is the only honest path to
> anything resembling "anyone can use it," because it removes the operator burden
> entirely. That is a different product with a different surface, and is out of
> scope until the self-hosted wedge has clear PMF.

## What expansion must NOT change

The widening above is **surfaces, not scope.** It preserves every invariant that
made the June-2026 pivot worth doing:

- Still **not a gateway**; still **off the inference hot path**.
- Still **never holds provider keys** — only scoped admin/notify tokens.
- Still **one core loop** — no second product grafted on.
- "Anyone can use it" is reinterpreted honestly: not "anyone installs and runs the
  engine," but "the people who already care about the engine's output finally have
  a way to see and act on it."

## Gate

Nothing in the expansion plan ships before (1) a Phase-0 **GO** on the platform-
engineer wedge and (2) a specific second persona actively pulling for their
surface. Document the pull in `docs/phase0/` before building the surface — same
bar the engine itself was held to.
