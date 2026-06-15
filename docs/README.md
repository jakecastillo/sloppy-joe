# Sloppy Joe — Documentation

> **The map.** Every document in the project, grouped by what you're trying to do.
> The repo [`README`](../README.md) is the front door — what Sloppy Joe is, plus a
> quickstart. This index is the room behind it: go as deep as you need, no deeper.

**New here? Read in this order:** [`README`](../README.md) (what + run it) →
[`vision.md`](vision.md) (why it exists) → [`audience.md`](audience.md) (who it's
for) → the [design spec](superpowers/specs/2026-06-08-sloppy-joe-v0-design.md) (how
it's built).

## Start here — get it running

| Doc | What's in it |
|---|---|
| [`../README.md`](../README.md) | The one-liner, the architecture diagrams, and the quickstart. |
| [`quickstart.md`](quickstart.md) | The 5-minute end-to-end walkthrough — `init → config validate → doctor → recipe show → test --replay → inject → audit tail → sloppyd`, threaded with the shipped `examples/` assets, zero-config. |
| [`integrations.md`](integrations.md) | Copy-paste quickstart to wire each gateway/sink/source — LiteLLM, GitHub, Slack, OTLP metrics — env vars + `platforms:` enable block + exact `curl`. |
| [`../examples/`](../examples/) | Runnable CEL rules, a sample signal, and a replay fixture — the fastest way to watch a rule fire. |
| [`../CONTRIBUTING.md`](../CONTRIBUTING.md) | How to build and test, and the bar every change must clear. |

## Understand it — why it exists, what it is and isn't

| Doc | What's in it |
|---|---|
| [`vision.md`](vision.md) | **Canonical "why."** The refined problem, the pivot away from building a gateway, the single load-bearing invariant, and the beachhead user. Read this first. |
| [`audience.md`](audience.md) | Who *operates* the loop vs. who *benefits* from it, and the demand-gated plan to widen beyond the lone platform engineer. |
| [`superpowers/specs/2026-06-08-sloppy-joe-v0-design.md`](superpowers/specs/2026-06-08-sloppy-joe-v0-design.md) | **Canonical engineering design** — scope, schemas (Signal / Rule / Intent), success criteria, non-goals. |

## Roadmap & plans — where it's going

| Doc | What's in it |
|---|---|
| [`superpowers/plans/2026-06-08-roadmap.md`](superpowers/plans/2026-06-08-roadmap.md) | The current roadmap — NOW / NEXT / LATER, with sequencing rationale. |
| [`superpowers/plans/BACKLOG.md`](superpowers/plans/BACKLOG.md) | The active plan backlog (what's done, what's queued). |
| [`superpowers/plans/`](superpowers/plans/) | The numbered build plans (1–4) v0 shipped against. |
| [`../CHANGELOG.md`](../CHANGELOG.md) | What actually shipped, by version. |

## Validate the bet — Phase 0

> The project-defining risk is demand, not code. This kit is how we test it.

| Doc | What's in it |
|---|---|
| [`phase0/README.md`](phase0/README.md) | The demand-validation kit and the GO / PIVOT / STOP decision rule. |
| [`phase0/target-profile.md`](phase0/target-profile.md) | Who to recruit — the screening filter (must-haves, disqualifiers, where to find them). |
| [`phase0/discovery-script.md`](phase0/discovery-script.md) | The 5-question, ~20-minute discovery interview. |
| [`phase0/demo-script.md`](phase0/demo-script.md) | The 5-minute live demo to show design partners. |
| [`phase0/kill-criteria.md`](phase0/kill-criteria.md) | The explicit conditions under which we stop or pivot. |

## Contribute & policy

| Doc | What's in it |
|---|---|
| [`../CONTRIBUTING.md`](../CONTRIBUTING.md) | Contribution workflow, including DCO sign-off (`git commit -s`). |
| [`conventional-commits.md`](conventional-commits.md) | The commit-message format (enforced by hook + CI). |
| [`../.github/DCO`](../.github/DCO) | The Developer Certificate of Origin. |
| [`../SECURITY.md`](../SECURITY.md) | How to report a vulnerability. |
| [`../LICENSE`](../LICENSE) · [`../NOTICE`](../NOTICE) · [`../AUTHORS`](../AUTHORS) | Apache-2.0 license, attribution notice, and authorship. |

---

> **How this system stays DRY:** every fact lives in exactly one doc; everything
> else links to it. This index carries the *descriptions* so the individual docs
> don't repeat each other, and the top-level `README` carries only a few curated
> entry points that link back here. When you add a doc, add one row here — that's
> the only place its summary belongs.
