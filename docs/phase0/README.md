# Phase 0 — Demand Validation Kit

**The project-defining risk is not technical — it's whether anyone wants this.**
Before investing further, we validate the one bet that everything rests on:

> **Hypothesis:** Platform engineers running AI in production want their *response
> to AI incidents* (cost spikes, fallback storms, eval/guardrail trips) to be a
> **reviewed, replayable, audited artifact in Git** — not a buried dashboard rule,
> a budget cron, or an n8n flow glued to a gateway.

If that's true, Sloppy Joe's wedge (governed, reviewable, auditable, reversible
AI-ops remediation) is real. If it's not, no amount of engineering matters.

## Who we're validating with (beachhead)

The **lone platform / AI-infra engineer at a 20–200-person AI-product company** who:
- already runs an LLM gateway (LiteLLM / Bifrost / Portkey) and ≥1 local model,
- has AI spend and incidents big enough to hurt (surprise token bills, provider
  outages, a model regression), and
- is too small to have a dedicated AI-platform team building bespoke control loops.

**Not** the solo hobbyist (no governance pain) and **not** the large enterprise
(building it in-house; needs SOC2/SSO we don't have yet).

## How to run it

1. **Recruit 3–5** people matching the profile (see `target-profile.md` — *human step*).
2. **Run the discovery interview** (`discovery-script.md`) — 5 questions, ~20 min.
   Listen for the duct tape: "show me how you handle a cost spike today."
3. **Show the 5-minute demo** (`demo-script.md`) — the now-working
   inject → audit → tamper → replay → validate loop. Record it once as an
   asciinema/GIF so it's reusable (*human step*).
4. **Score against `kill-criteria.md`** and decide.

## Decision rule

- **GO** (harden + grow): ≥3/5 confirm they currently hand-roll this, find it
  painful, and would put the response policy in a reviewed PR.
- **PIVOT**: they feel a *different* adjacent pain more (reshape around it).
- **STOP**: they're satisfied by existing gateway/automation tooling, or won't
  treat response-policy as a reviewable artifact.

Lock the `Signal` / `Rule` / `Intent` v0.1 schemas **only after** this feedback.

> Status: kit drafted. The recruiting + recording + interviews are human steps —
> this repo can't do them. Everything the partners are shown (the demo) is real
> and runnable today (`make build`).
