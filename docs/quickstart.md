# Quickstart — the loop in 5 minutes

This threads the whole **observe → decide → act → record → revert** loop on a clean
machine using only the binaries you build and the shipped [`examples/`](../examples/)
assets. No gateway, no provider keys, no network — every command below runs as
written against the SQLite default.

> **Prerequisites:** Go (to build the two binaries) and `curl` (for the daemon step).
> The full copy-paste wiring for a *real* LiteLLM / GitHub / Slack / OTLP setup lives
> in [`integrations.md`](integrations.md) — this page deliberately stays zero-config.

## Build the two binaries

```bash
go build -o bin/sloppy  ./cmd/sloppy
go build -o bin/sloppyd ./cmd/sloppyd
```

`sloppy` is the one-shot CLI; `sloppyd` is the long-running daemon. Run everything
below from the repo root so the `examples/` paths resolve.

## 1. Scaffold a workspace — `sloppy init`

```bash
./bin/sloppy init
```

This writes a working `sloppy.yaml` (with the `cost-guard` recipe enabled), a
redacted `.env.sample`, an ed25519 signing key (`sloppy.key`, plus its `.pub`), and
an empty `./rules/` directory with a commented `example.yaml.sample`. It is
non-interactive and safe to re-run — a second `init` is a no-op unless you pass
`--force`.

```text
✓ wrote sloppy.yaml
✓ wrote .env.sample
✓ created signing key sloppy.key
✓ created rules dir rules
✓ wrote rules/example.yaml.sample
next: run `sloppy config validate`, then `sloppy doctor`, then start the daemon with `sloppyd`
```

## 2. Lint the config — `sloppy config validate`

An offline CI gate: it parses `sloppy.yaml`, checks every field, and renders the
enabled recipes so a bad parameter or template fails the gate.

```bash
./bin/sloppy config validate
```

```text
✓ config valid
```

## 3. Check the environment — `sloppy doctor`

`doctor` is green out of the box after `init`: the rules path exists, the state DB
opens and migrates, the cost ledger is queryable, and the actuator registry loads.
With no platforms enabled, it acts in **Log only** mode — nothing reaches the network.

```bash
./bin/sloppy doctor
```

```text
[✓] rules      rules exists but has no rule files yet (add *.yaml rules, or rely on recipes)
[✓] state-db   opens + migrates ok
[✓] ledger     usage store queryable
[✓] platforms  none enabled (Log only)
[✓] litellm    disabled (probe skipped)
[✓] actuators  5 kind(s): disable_deployment, open_issue, page, route_override, throttle_tenant
```

## 4. See what a recipe expands to — `sloppy recipe show cost-guard`

Recipes are curated rules. `recipe show` renders one to the exact CEL rule it
produces (plus the content-hash SHA), read-only — this is the rule `cost-guard`
contributes to your live loop.

```bash
./bin/sloppy recipe show cost-guard
```

```text
# recipe: cost-guard v1
on: cost.budget_burn
when: signal.data.spend_1h_usd > 5
for: 5m
then:
  - route_override: { alias: gpt-4o, to: ollama/llama3, ttl: 30m }
with: { dry_run: false, intent_budget: "3/h" }
# rendered rule sha: 2f8857278202
```

## 5. Replay a fixture — `sloppy test --replay`

Replay the shipped JSONL fixture against the shipped example rules and see what
*would* fire — deterministic, no actuation, no state writes. This is the CI gate you
drop into a PR check. Point `--rules` at [`examples/rules`](../examples/rules/), since
the freshly scaffolded `./rules/` is empty (the live loop is driven by the recipe).

```bash
./bin/sloppy test --replay examples/fixtures/replay.jsonl --rules examples/rules
```

```text
s1-cost        would route_override target=gpt-4o (rule a84055d963f1)
s1-cost        would open_issue target=gpt-4o (rule a84055d963f1)
s1-cost        would page target=gpt-4o (rule a84055d963f1)
s2-latency     would page target=gpt-4o (rule 8c5ea7eb163c)
...
replay: 5 signal(s), 10 intent(s) would fire
```

## 6. Fire a real signal — `sloppy inject`

`inject --now` runs one recorded signal through the rules immediately (bypassing the
`for:` window), applies the matching intents, and writes the signed audit entries.
With no platforms enabled the intents resolve through the Log actuator — safe to run
anywhere.

```bash
./bin/sloppy inject --now --rules examples/rules examples/signals/cost-spike.json
```

```text
applied            route_override target=gpt-4o
applied            open_issue target=gpt-4o
applied            page target=gpt-4o
```

## 7. Read the tamper-evident audit — `sloppy audit tail`

Every applied intent appended a hash-chained, ed25519-signed audit entry. `audit
tail` prints the chain and verifies it; it exits non-zero if the chain is tampered.

```bash
./bin/sloppy audit tail
```

```text
   1  intent.applied    route_override target=gpt-4o rule=a84055d963f1 canon=… sig=…
   2  intent.applied    open_issue target=gpt-4o rule=a84055d963f1 canon=… sig=…
   3  intent.applied    page target=gpt-4o rule=a84055d963f1 canon=… sig=…
chain: verified ✓ (3 entries)
```

> To verify each intent's signature against the persisted public key (a CI-gateable
> check), add `--verify-sigs`: `./bin/sloppy audit --verify-sigs`.

## 8. Run it continuously — `sloppyd`

The daemon adds HTTP ingest, the TTL auto-revert ticker, and a `/status` metrics
endpoint. It binds **loopback** (`127.0.0.1:8723`) by default, so it starts with no
extra flags and is not reachable from the network.

```bash
./bin/sloppyd --rules examples/rules &
```

```text
level=INFO msg="sloppyd listening" addr=127.0.0.1:8723
```

Poll its status, then POST the same signal over HTTP and watch the counter move:

```bash
curl 127.0.0.1:8723/status
# {}

curl -XPOST 127.0.0.1:8723/v1/signals -d @examples/signals/cost-spike.json
curl 127.0.0.1:8723/status
# {"signals_handled":1}
```

> To expose the daemon beyond loopback you must authenticate it: `sloppyd --addr
> :8723 --auth` (with `SLOPPY_API_KEYS`) — the bind guard refuses an unauthenticated
> network-reachable bind.

## Next steps

- **Wire a real gateway / sink** — env vars + `platforms:` block + exact `curl` per
  integration in [`integrations.md`](integrations.md).
- **Browse the runnable assets** — the CEL rules, the sample signal, and the replay
  fixture under [`examples/`](../examples/).
- **Understand why it exists** — [`vision.md`](vision.md), then [`audience.md`](audience.md).
