# 🥪 Sloppy Joe

**Serving governed AI ops.** Sloppy Joe is an open-source, model-agnostic **control loop** that cleans up the slop of running AI in production. It sits *on top of* the LLM gateway you already run and turns model-layer signals — cost spikes, fallback storms, latency/quality regressions, guardrail trips, provider outages — into **governed, audited, Git-reviewed automated responses**.

> It is **not** another LLM gateway. It's the observe → decide → act loop that gateways and generic automation tools leave as a gap you currently fill with duct tape (n8n + a budget cron + Grafana alerts).

## The one-liner

A cost / eval / guardrail breach spawns a governed, capability-scoped, audited remediation whose **policy is a file in Git you review in a PR and replay in CI** — executed as a signed, reversible intent any gateway can apply.

## What it feels like

```text
$ sloppy up
  🥪 sloppy joe — serving governed AI ops
  watching OTel GenAI stream… cost ledger live · 1 rule armed

$ sloppy rules apply ./rules
  ✓ cost-guard.yaml   (loop armed)

$ sloppy audit tail
  reroute acme → ollama   ✓ signed receipt
```

> The command is **`sloppy`** (`sloppy up`, `sloppy audit tail`). We avoided `joe` because it collides with the classic `joe` editor (Joe's Own Editor) on many Unix systems — the brand stays **Sloppy Joe**.

## Status

🚧 **Design approved — pre-implementation.** The v0 design is locked (see [`docs/superpowers/specs/2026-06-08-sloppy-joe-v0-design.md`](docs/superpowers/specs/2026-06-08-sloppy-joe-v0-design.md)); the implementation plan is next. A **Phase-0 demand-validation** with design partners runs in parallel (see [`docs/vision.md`](docs/vision.md)). No implementation code yet — by design.

## Principles

- **Model-agnostic** — consumes OpenTelemetry GenAI + CloudEvents, acts via vendor-neutral signed intents
- **Linux/Unix-first** — single static Go binary
- **Library-first** — a small core library; CLI + daemon are thin wrappers
- **Packageable solo → enterprise** from one core (swappable adapters)
- **Issue-driven · automation-first** — and multi-purpose, not single-purpose
- **Never holds your provider keys** — those stay in the gateway

## License

Intended: **Apache-2.0** (permissive, with a patent grant). `LICENSE` to be added at design lock.

---

*Built in the open. Contributions welcome once the v0 design is locked.*
