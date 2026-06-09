# Target Profile — who to recruit

> The screening filter for Phase 0. The goal is **3–5 partners who match tightly**.
> A loose fit produces polite "sounds cool" noise — which [`discovery-script.md`](discovery-script.md)
> reminds you is a **NO**. Screen against this list *before* booking; spend the
> ~20-minute interview only on a confirmed fit. This is the beachhead from
> [`vision.md`](../vision.md) made recruitable — it does not restate the interview,
> the demo, or the [kill criteria](kill-criteria.md).

## Must-have — recruit only if **all** are true

- [ ] **Role:** the lone or lead **platform / AI-infra / DevOps engineer** who *owns
      the AI stack* — not an app developer, not an ML researcher.
- [ ] **Company:** ~**20–200 people**, where AI is **in the product** (shipped, not an
      experiment).
- [ ] **Stack:** already runs an **LLM gateway** (LiteLLM / Bifrost / Portkey / Envoy
      AI Gateway) **and ≥1 self-hosted/local model** (Ollama / vLLM) — i.e. the
      cross-vendor reality is real for them.
- [ ] **Pain is real:** burned in the last ~6 months by a **surprise token bill, a
      provider outage, or a model regression**.
- [ ] **Has the keys:** can actually change gateway config / deploy a daemon — the
      access *and* the authority to adopt.

## Strong positive signals (prioritize these)

- Already hand-rolls glue: **n8n / a budget cron / Grafana alert → webhook**. (This
  is the duct tape we're betting is painful — see `discovery-script.md` Q2.)
- Cares about **audit / provenance**: regulated customers, SOC 2 in progress, or EU
  AI Act exposure.
- **Multi-gateway or mid-migration** between gateways — cross-vendor pain is live,
  which is exactly the moat.

## Disqualifiers — do NOT recruit (they will mislead the result)

| Anti-profile | Why they pollute the signal |
|---|---|
| **Solo hobbyist / side project** | No governance pain; "sounds cool" with no intent to adopt. |
| **Large enterprise with a dedicated AI-platform team** | Builds in-house; needs SSO/RBAC/KMS we don't have yet. |
| **No gateway, or single-provider via the vendor SDK** | Not in the seam Sloppy Joe owns — nothing to sit on top of. |
| **App developer / ML researcher with no ops ownership** | Not the operator and not the buyer. |

> Each disqualifier maps to a false signal against the [kill criteria](kill-criteria.md):
> recruiting them risks a false STOP (no pain) or a false GO (enthusiasm without adoption).

## Where to find them

- AI-infra / platform-eng communities: **MLOps Community Slack**, **r/LLMOps**, the
  **LiteLLM / vLLM / Ollama** Discords/forums.
- The **OpenTelemetry GenAI** SIG and CNCF platform-engineering channels.
- **Contributors and issue-filers** on the gateway repos themselves (they self-select
  as in-the-seam).
- Your own network of platform engineers; LinkedIn search by **title + gateway in the
  profile/posts**.

## Recruiting target & diversity

Aim for **3–5 confirmed-fit** partners — and don't recruit five clones:

- **≥2 different gateways** represented (tests the cross-vendor claim).
- A mix of **cost-pain-led** vs **reliability/regression-pain-led**.
- At least one with a **real audit/compliance driver**.

## The ask (outreach — no pitch)

Keep it short and research-framed, **not** a demo invite:

> "I'm researching how teams handle AI incidents in prod — cost spikes, provider
> outages, model regressions. Could I get **20 minutes** to hear how you handle them
> today? Not selling anything."

Run [`discovery-script.md`](discovery-script.md) on the call; offer the
[5-minute demo](demo-script.md) only *after* Q5 and only if they lean in
("can I try it on my gateway?"). Score each interview with the rubric in
`discovery-script.md` and roll up against `kill-criteria.md`.

## Candidate tracker

Copy this table; one row per candidate, fill it during screening.

| Candidate / role | Co. size | Gateway | Local model | Recent pain | Hand-rolls glue? | Fit? | Booked? |
|---|---|---|---|---|---|---|---|
| | | | | | | | |
