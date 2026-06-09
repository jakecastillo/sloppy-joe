# Discovery Interview (≈20 min, 5 questions)

Goal: find out whether the duct tape is real and painful, and whether
response-as-reviewable-artifact resonates. **Don't pitch first — listen.**

## The five questions

1. **"Walk me through the last time your AI traffic misbehaved in prod"**
   (cost spike, provider outage, a model regression, a guardrail/PII trip).
   *Listen for:* how they found out, how long it took, what they did manually.

2. **"What automatically responds to that today, if anything?"**
   *Listen for:* n8n / a budget cron / Grafana alerts / a gateway webhook / "nothing,
   we get paged." **This is the duct tape.** Ask them to show it.

3. **"Who decides what the automated response should be, and where does that logic live?"**
   *Listen for:* a dashboard toggle, a script in a repo, tribal knowledge.
   *Probe:* "Is that logic reviewed before it ships? Could a teammate diff it?"

4. **"After an automated action fires, can you prove what happened and why — to an
   auditor or a teammate — including which policy version caused it?"**
   *Listen for:* "no / we grep logs / sort of." Gauge how much that gap bothers them.

5. **"If your AI-incident response were a file in Git you reviewed in a PR and
   replayed in CI before it shipped — would you use that? What would have to be
   true?"**
   *This tests the wedge directly.* Capture objections verbatim.

## Scoring each interview

For each partner, record yes/no:
- [ ] Hand-rolls the observe→decide→act loop today (Q2)
- [ ] Finds it painful / has been burned (Q1, Q2)
- [ ] Response logic is **not** currently reviewable (Q3)
- [ ] Feels the audit/provenance gap (Q4)
- [ ] Would adopt response-as-PR-artifact (Q5)

Roll up against `kill-criteria.md`. **3+ partners with 4+ boxes checked → GO.**

## Anti-leading reminders

- Don't describe Sloppy Joe until after Q5.
- "Would you use X?" is weak signal; "show me what you do today" is strong signal.
- A polite "sounds cool" is a NO. Look for "can I try it on my gateway?"
