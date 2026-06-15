# Quickstart — the loop in 5 minutes

Thread the whole **observe → decide → act → record** loop on your laptop, with no
gateway, no provider keys, and no network — using only the shipped `sloppy` CLI and
the example assets in [`../examples/`](../examples/). Every command below is a
read-or-local-state operation; nothing leaves your machine.

> **One-time build.** From the repo root:
>
> ```bash
> go build -o bin/sloppy ./cmd/sloppy
> ```
>
> Everything below assumes `bin/sloppy` is on your `PATH` (or call it as
> `./bin/sloppy`). The example rules and fixtures are referenced by relative path,
> so run these from the repo root.

## 1. Scaffold a workspace — `sloppy init`

`init` writes a working starter config, a redacted `.env.sample`, and an ed25519
signing key. It is non-interactive and never clobbers an existing config without
`--force`.

```text
$ sloppy init
✓ wrote sloppy.yaml
✓ wrote .env.sample
✓ created signing key sloppy.key
next: edit sloppy.yaml, then run `sloppy config validate` and `sloppyd`
```

The scaffolded `sloppy.yaml` enables the **cost-guard** recipe and leaves every
credentialed platform (LiteLLM, GitHub, Slack) disabled — so the loop runs end to
end with the built-in Log actuator and no secrets.

## 2. See what a recipe becomes — `sloppy recipe show`

Recipes are curated workflows that render into ordinary, content-hashed rules. List
them, then render one to the exact rule it produces:

```text
$ sloppy recipe list
cost-guard      enabled    Cost burn over a $/hr threshold -> fail over to a cheaper/local model (+ optional issue/page).
cost-runaway    available  Extreme spend over a $/hr threshold -> throttle the tenant (TTL, auto-revert on_clear) (+ optional issue/page).
fallback-storm  available  Critical provider-fallback storm -> page on-call (+ optional issue).
latency-guard   available  p95 latency regression over a threshold -> page on-call (+ optional issue).

$ sloppy recipe show cost-guard
# recipe: cost-guard v1
on: cost.budget_burn
when: signal.data.spend_1h_usd > 5
for: 5m
then:
  - route_override: { alias: gpt-4o, to: ollama/llama3, ttl: 30m }
with: { dry_run: false, intent_budget: "3/h" }
# rendered rule sha: fa1d1b3d4b07
```

The rendered rule carries a content-hash SHA, so the same params always reproduce
the same rule — that is the artifact you review in a PR. (The issue/page actions
only render once you enable GitHub/Slack in `sloppy.yaml`.)

## 3. Dry-run a fixture in CI — `sloppy test --replay`

`test --replay` deterministically runs a JSONL fixture of signals against your rules
and prints **what would fire** — no actuation, no state writes. This is the
CI-gateable check. Replay the shipped fixture against the example rules:

```text
$ sloppy test --replay examples/fixtures/replay.jsonl --rules examples/rules
s1-cost        would route_override target=gpt-4o (rule a84055d963f1)
s1-cost        would open_issue target=gpt-4o (rule a84055d963f1)
s1-cost        would page target=gpt-4o (rule a84055d963f1)
s2-latency     would page target=gpt-4o (rule 8c5ea7eb163c)
s2-latency     would open_issue target=gpt-4o (rule 8c5ea7eb163c)
s3-fallback    would page target=gpt-4o (rule c788804cbedd)
s4-quiet       (no rule)
s5-runaway     would route_override target=gpt-4o (rule a84055d963f1)
s5-runaway     would open_issue target=gpt-4o (rule a84055d963f1)
s5-runaway     would page target=gpt-4o (rule a84055d963f1)
s5-runaway     would throttle_tenant target=acme (rule 2e907441dfaa)
replay: 5 signal(s), 10 intent(s) would fire
```

Each line is `<signal-id> would <action> target=<...> (rule <sha>)`. The quiet
signal (`s4-quiet`, $1/hr) matches no rule — exactly the negative case you want a
fixture to lock in.

## 4. Act on a single signal — `sloppy inject --now`

`inject --now` fires matching rules immediately (bypassing `for:` windows), applies
the intents, and appends them to the tamper-evident audit chain. Drive the shipped
cost-spike signal through the example rules:

```text
$ sloppy inject --now --rules examples/rules examples/signals/cost-spike.json
  → route_override target=gpt-4o args=map[alias:gpt-4o to:ollama/llama3 ttl:30m]
  → open_issue target=gpt-4o args=map[repo:acme/ops]
  → page target=gpt-4o args=map[slack:#oncall]
applied            route_override target=gpt-4o
applied            open_issue target=gpt-4o
applied            page target=gpt-4o
```

With no platforms enabled, the Log actuator handles each intent — but the intent is
still built, ed25519-signed, idempotency-checked, and recorded just as it would be
against a real gateway.

## 5. Read the tamper-evident record — `sloppy audit`

Every applied intent lands in a hash-chained audit log. Tail it, then verify both
the chain and each persisted ed25519 signature (the CI-gateable integrity check —
it exits non-zero on any tamper or signature failure):

```text
$ sloppy audit tail
   1  intent.applied    route_override target=gpt-4o rule=a84055d963f1 canon=… sig=…
   2  intent.applied    open_issue target=gpt-4o rule=a84055d963f1 canon=… sig=…
   3  intent.applied    page target=gpt-4o rule=a84055d963f1 canon=… sig=…
chain: verified ✓ (3 entries)

$ sloppy audit --verify-sigs
…
chain: verified ✓ (3 entries)
signatures: 3 verified, failed=0
```

Each entry persists the signed canonical bytes (`canon=`) and full signature
(`sig=`); `--verify-sigs` recomputes the canonical bytes and checks every signature
against the persisted public key (`sloppy.key.pub`). A broken chain or a failed
signature exits non-zero, so you can drop this straight into a PR check.

## Where to go next

You just ran the loop one signal at a time. The next steps:

- **Run it continuously** — `sloppyd` adds HTTP ingest, `for:`-window evaluation
  across a live signal stream, and TTL auto-revert. See the
  [Quickstart in the repo `README`](../README.md#quickstart).
- **Wire a real gateway or sink** — copy-paste env vars + `platforms:` blocks for
  LiteLLM, GitHub, Slack, and OTLP in [`integrations.md`](integrations.md).
- **Understand why it exists** — [`vision.md`](vision.md): the problem, the pivot,
  and the wedge.
