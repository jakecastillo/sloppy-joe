# 🥪 Sloppy Joe

**Serving governed AI ops.** Sloppy Joe is an open-source, model-agnostic **control loop** that cleans up the slop of running AI in production. It sits *on top of* the LLM gateway you already run and turns model-layer signals — cost spikes, fallback storms, latency/quality regressions, guardrail trips, provider outages — into **governed, audited, Git-reviewed automated responses**.

> It is **not** another LLM gateway. It's the observe → decide → act loop that gateways and generic automation tools leave as a gap you currently fill with duct tape (n8n + a budget cron + Grafana alerts).

## The one-liner

A cost / eval / guardrail breach spawns a governed, capability-scoped, audited remediation whose **policy is a file in Git you review in a PR and replay in CI** — executed as a signed, reversible intent any gateway can apply.

## What it feels like

```text
# one-shot: run a signal through your rules now (acts, then writes the audit)
$ sloppy inject --now --rules examples/rules examples/signals/cost-spike.json
  applied            route_override target=gpt-4o
  applied            page target=gpt-4o

# CI gate: replay a fixture and see what WOULD fire (no side effects)
$ sloppy test --replay examples/fixtures/replay.jsonl --rules examples/rules
  replay: 4 signal(s), 6 intent(s) would fire

# the tamper-evident audit log
$ sloppy audit tail
  chain: verified ✓ (3 entries)

# run continuously: HTTP ingest + TTL auto-revert + /status metrics
$ sloppyd --rules examples/rules
  🥪 sloppyd listening on :8723
```

> The command is **`sloppy`** (`sloppy up`, `sloppy audit tail`). We avoided `joe` because it collides with the classic `joe` editor (Joe's Own Editor) on many Unix systems — the brand stays **Sloppy Joe**.

## Status

✅ **v0 implemented (Plans 1–4).** Library (`libsloppyjoe`) + `sloppy` CLI + `sloppyd` daemon. `go test ./...` green across all packages; static `CGO_ENABLED=0` binaries. Design + plans live under [`docs/superpowers/`](docs/superpowers/). A **Phase-0 demand-validation** with design partners runs in parallel (see [`docs/vision.md`](docs/vision.md)).

## Architecture

Sloppy Joe is a **control loop, not a gateway** — it sits beside the LLM gateway you already run, consumes its telemetry, and acts back on it through narrow, signed, reversible intents. It is never on the inference request hot path, and it never holds your provider keys.

```mermaid
flowchart LR
  apps["Apps / agents"] -->|inference requests| gw

  subgraph GW["LLM gateway — you already run it (not built by Sloppy Joe)"]
    gw["LiteLLM · Bifrost · Envoy AI Gateway"]
    keys[("provider keys live HERE")]
  end
  gw --> prov["OpenAI · Anthropic · Ollama / vLLM"]

  subgraph SJ["Sloppy Joe — AI-ops control loop (off the hot path)"]
    auth["ee/ API-key RBAC"]
    subgraph ING["ingest (HTTP)"]
      i1["POST /v1/signals"]
      i2["POST /v1/usage"]
      i3["POST /v1/otlp/metrics"]
    end
    eng["engine: reconcile → CEL → sign → govern → idempotent"]
    rls[("rules/*.yaml + CEL")]
    subgraph ST["state.Store"]
      led[("cost ledger")]
      aud[("hash-chained audit")]
      rev[("pending TTL reverts")]
    end
    brk["secret broker (admin / notify tokens only)"]
    subgraph ACT["actuators — signed, reversible intents"]
      ac1["LiteLLM route_override"]
      ac2["GitHub issue"]
      ac3["Slack page"]
      ac4["Bifrost · Envoy"]
    end
  end

  gw -. OTel telemetry .-> ING
  auth --> ING
  ING --> eng
  rls --> eng
  eng <--> ST
  brk --> ACT
  eng --> ACT
  ac1 -. admin API .-> gw
  ac4 -. admin API .-> gw
  ac2 --> ext1["GitHub"]
  ac3 --> ext2["Slack"]

  subgraph BK["state backends"]
    db1[("SQLite — solo")]
    db2[("Redis — multi-replica")]
  end
  ST --- db1
  ST --- db2

  subgraph BIN["binaries"]
    c1["sloppy CLI — inject · test --replay · audit · doctor"]
    c2["sloppyd — ingest + TTL revert + /status"]
  end
  c1 --> eng
  c2 --> ING
```

> Everything in this diagram is implemented and tested.

The runtime loop — **observe → decide → act → record → revert** — all off the request hot path:

```mermaid
sequenceDiagram
  participant GW as Gateway (OTel)
  participant IN as Sloppy Joe ingest
  participant EN as engine
  participant ST as state (ledger/audit/reverts)
  participant AC as actuator
  GW-->>IN: telemetry / signal (cost burn, latency, fallback…)
  IN->>EN: Signal
  EN->>ST: read cost / policy state
  EN->>EN: CEL match? `for:` window held?
  EN->>EN: build + ed25519-sign RemediationIntent
  EN->>ST: idempotency check (skip if already applied)
  EN->>AC: Apply (reroute / open issue / page)
  AC-->>GW: admin API (reroute)
  EN->>ST: mark applied + append hash-chained audit
  Note over EN,ST: TTL elapses (sloppyd ticker)
  EN->>AC: Revert
  EN->>ST: mark reverted + audit
```

## Quickstart

```bash
go build -o bin/sloppy  ./cmd/sloppy
go build -o bin/sloppyd ./cmd/sloppyd

# fire a recorded signal through the example rules, then read the audit
./bin/sloppy inject --now --rules examples/rules --db /tmp/sloppy.db examples/signals/cost-spike.json
./bin/sloppy audit tail --db /tmp/sloppy.db

# verify every applied intent's ed25519 signature against the persisted public key
# (sloppy.key.pub). Exits non-zero if any signature fails — a CI-gateable check.
./bin/sloppy audit --verify-sigs --db /tmp/sloppy.db --key sloppy.key

# or run the daemon and POST signals / usage over HTTP
./bin/sloppyd --rules examples/rules --db /tmp/sloppy.db &
curl -XPOST localhost:8723/v1/signals -d @examples/signals/cost-spike.json
curl localhost:8723/status
```

**Commands:** `sloppy inject` · `sloppy rules validate` · `sloppy test --replay` · `sloppy audit tail` · `sloppy audit --verify-sigs` · `sloppy doctor` · `sloppyd` (daemon).
- **Verifiable signatures:** intents are ed25519-signed; the applied-audit entry persists the signed canonical bytes + full signature. `sloppy audit --verify-sigs` recomputes each intent's canonical bytes and verifies the signature against the persisted public key (`sloppy.key.pub`), exiting non-zero on any failure. The private key (`sloppy.key`, mode `0600`) is required to forge a verifiable intent; a holder of only the public key can verify authenticity and detect tampering but cannot sign. See [`SECURITY.md`](SECURITY.md) for the threat model.
- **State backend:** `sloppyd --store sqlite` (default) or `--store redis --redis-addr host:6379`.
- **Auth:** `sloppyd --auth` with `SLOPPY_API_KEYS="key1=ingest:write,status:read"`.
- **Gateway:** to wire a real LiteLLM admin API, set `SLOPPY_LITELLM_URL` and `SLOPPY_TOKEN_LITELLM`.
- **`for:` windows:** one-shot `sloppy inject --now` fires immediately; the `sloppyd` daemon evaluates `for:` windows across the live signal stream.
- **Gate rules in CI (no infra):** `sloppy rules validate ./rules` compiles every CEL `when`, checks action kinds + `intent_budget`, and exits non-zero on error — drop it in a PR check.

## Documentation

New here? The fastest path to productive:

1. **Run it** — the [Quickstart](#quickstart) above, then poke at [`examples/`](examples/) (CEL rules + a sample signal + a replay fixture).
2. **Understand why it exists** — [`docs/vision.md`](docs/vision.md): the problem, the pivot, and the wedge.
3. **See who it's for** — [`docs/audience.md`](docs/audience.md): who *operates* it vs. who *benefits*.

📖 **Full documentation map → [`docs/`](docs/README.md)** — every doc, grouped by intent (*understand · plan · validate · contribute*). One index, no duplicated copy: descriptions live there, this README just points in.

## Principles

- **Model-agnostic** — consumes OpenTelemetry GenAI + CloudEvents, acts via vendor-neutral signed intents
- **Linux/Unix-first** — single static Go binary
- **Library-first** — a small core library; CLI + daemon are thin wrappers
- **Packageable solo → enterprise** from one core (swappable adapters)
- **Issue-driven · automation-first** — and multi-purpose, not single-purpose
- **Never holds your provider keys** — those stay in the gateway

## Deploy (local end-to-end)

A `docker-compose.yml` brings up the whole loop — Ollama (local model), LiteLLM
(gateway), Redis (state), and `sloppyd` (the control loop), no provider keys
required:

```bash
docker compose up -d --build
# drive a cost spike and watch it remediate:
curl -XPOST localhost:8723/v1/signals -d @examples/signals/cost-spike.json
curl localhost:8723/status
```

The end-to-end test runs against that stack. It's guarded by the `integration`
build tag, so it never runs in the normal suite:

```bash
SLOPPY_E2E_BASE=http://localhost:8723 go test -tags integration ./test/e2e/...
```

> The LiteLLM admin schemas the actuators use are still provisional — this stack
> is where you verify them against a real gateway.

## License

Sloppy Joe is licensed under the **[Apache License 2.0](LICENSE)** — permissive,
with an explicit patent grant (the trust standard for control-plane / infra
projects). Contributions are accepted under the same license via a
[Developer Certificate of Origin](.github/DCO) sign-off (`git commit -s`); see
[CONTRIBUTING](CONTRIBUTING.md). Copyright © The Sloppy Joe Authors.

---

*Built in the open. Contributions welcome once the v0 design is locked.*
