# Integrations — copy-paste quickstart

Wire one gateway or sink at a time. Each section is self-contained: the **env vars**
to export, the `platforms:` **enable block** to paste into `sloppy.yaml`, and the
**exact `curl`** to prove it end-to-end against a running `sloppyd`.

The rules never change here. Secrets are **always** a `token_env` reference resolved
by the `SLOPPY_TOKEN_*` broker — never inline in the Git-reviewed config (the broker
also honors a `<NAME>_FILE` form pointing at a mounted file, e.g. `/run/secrets`).
After editing, run `sloppy config validate` to lint and `sloppy doctor` to probe
connectivity and confirm each enabled platform's token is present.

> **Conventions used below.** `sloppyd` listens on `:8723`. The `curl` examples
> assume auth is **off** (the default on a localhost bind). On a network-reachable
> bind, `sloppyd`'s bind guard requires `--auth` with `SLOPPY_API_KEYS` — then add
> `-H 'X-API-Key: <key>'` to every request (`/healthz` stays public). The
> ingest/usage routes need the `ingest:write` scope; `/status` needs `status:read`.

---

## LiteLLM (gateway — `route_override`)

Sloppy Joe sits beside the LiteLLM gateway you already run and acts back on it
through signed, reversible `route_override` intents over its admin API. It never
holds your provider keys.

**Env vars:**

```bash
export SLOPPY_LITELLM_URL=http://localhost:4000   # your LiteLLM admin base URL
export SLOPPY_TOKEN_LITELLM=sk-...                # LiteLLM admin token (the secret)
```

**Enable block** (`sloppy.yaml` under `platforms:`):

```yaml
platforms:
  litellm: { enabled: true, url: http://localhost:4000, token_env: SLOPPY_TOKEN_LITELLM }
```

**Prove it** — drive a cost spike and watch the loop reroute, then read `/status`:

```bash
curl -XPOST localhost:8723/v1/signals -d @examples/signals/cost-spike.json
curl localhost:8723/status
```

> Legacy zero-config still works: with no `platforms:` block, setting
> `SLOPPY_LITELLM_URL` (+ `SLOPPY_TOKEN_LITELLM`) wires the LiteLLM actuator
> identically.

---

## Generic webhook (gateway — `route_override`)

The generic-webhook actuator drives any gateway or receiver that accepts a simple
JSON control call — a homegrown gateway, or a thin shim in front of one without a
dedicated adapter. Apply `POST`s `<url>/route` with `{"model": <target>, "to": <dest>}`
(bearer-authed); Revert restores the self-route.

**Env vars:**

```bash
export SLOPPY_TOKEN_WEBHOOK=...   # bearer token your receiver accepts (the secret)
```

**Enable block** (`sloppy.yaml` under `platforms:`):

```yaml
platforms:
  webhook: { enabled: true, url: http://localhost:9000, token_env: SLOPPY_TOKEN_WEBHOOK }
```

**Prove it** — fire a signal whose rule reroutes, then read `/status`:

```bash
curl -XPOST localhost:8723/v1/signals -d @examples/signals/cost-spike.json
curl localhost:8723/status
```

---

## Cloudflare AI Gateway (gateway — `throttle_tenant` / `disable_deployment`)

The Cloudflare actuator throttles or disables a Cloudflare AI Gateway via its admin
REST API (a single authenticated `PUT`). Apply pins `rate_limiting_limit=0` (no
requests admitted); Revert restores the prior limit (`intent.Args["prior_limit"]`, else
a conservative default). The `url` embeds the account-scoped collection path and
`intent.Target` is the gateway id appended as the final path segment. Auth is a
Cloudflare API token sent as `Authorization: Bearer`.

**Env vars:**

```bash
export SLOPPY_TOKEN_CLOUDFLARE=...   # Cloudflare API token (the secret)
```

**Enable block** (`sloppy.yaml` under `platforms:`):

```yaml
platforms:
  cloudflare:
    enabled: true
    url: https://api.cloudflare.com/client/v4/accounts/<account_id>/ai-gateway/gateways
    token_env: SLOPPY_TOKEN_CLOUDFLARE
```

**Prove it** — Cloudflare acts on `throttle_tenant` / `disable_deployment` intents, so
enable a rule that emits one (the shipped `cost-runaway` recipe throttles a tenant on
extreme spend), then drive the matching signal and read `/status`:

```bash
curl -XPOST localhost:8723/v1/signals -d @examples/signals/cost-spike.json
curl localhost:8723/status
```

---

## GitHub (sink — open an issue)

The GitHub actuator opens an issue as the incident record (carrying the intent id
and rule SHA as provenance). Use a fine-grained, repo-scoped, least-privilege token.

**Env vars:**

```bash
export SLOPPY_TOKEN_GITHUB=ghp_...   # repo-scoped token with issues:write (the secret)
```

**Enable block** (`sloppy.yaml` under `platforms:`):

```yaml
platforms:
  github: { enabled: true, repo: owner/name, token_env: SLOPPY_TOKEN_GITHUB }
```

**Prove it** — fire a signal whose rule opens an issue, then read `/status`:

```bash
curl -XPOST localhost:8723/v1/signals -d @examples/signals/cost-spike.json
curl localhost:8723/status
```

---

## Slack (sink — page a channel)

The Slack actuator posts to an incoming-webhook URL. The **webhook URL itself is the
secret** (anyone holding it can post), so it resolves just-in-time through the broker
and is never stored inline.

**Env vars:**

```bash
export SLOPPY_TOKEN_SLACK=https://hooks.slack.com/services/...   # webhook URL (the secret)
```

**Enable block** (`sloppy.yaml` under `platforms:`):

```yaml
platforms:
  slack: { enabled: true, channel: "#ai-ops", token_env: SLOPPY_TOKEN_SLACK }
```

**Prove it** — fire a signal whose rule pages, then read `/status`:

```bash
curl -XPOST localhost:8723/v1/signals -d @examples/signals/cost-spike.json
curl localhost:8723/status
```

---

## OTLP metrics (source — feed the cost ledger)

Point your OpenTelemetry GenAI metrics at `POST /v1/otlp/metrics`. Sloppy Joe reads
any metric whose name contains `token`, using the data-point attributes
`gen_ai.token.type` (`input`|`output`), `tenant`, and `gen_ai.request.model` (or
`model`) to record token usage into the cost ledger. This is a source, not a
platform — there is no `platforms:` block; the endpoint is live whenever the cost
ledger is enabled (the daemon default).

**Prove it** — POST an OTLP/JSON metrics payload (expects `202`; partial/total
persistence failures return `207`/`500` so an outage is never reported as success):

```bash
curl -XPOST localhost:8723/v1/otlp/metrics -H 'Content-Type: application/json' -d '{
  "resourceMetrics": [{ "scopeMetrics": [{ "metrics": [
    { "name": "gen_ai.client.token.usage", "sum": { "dataPoints": [
      { "asInt": "1000", "attributes": [
        { "key": "gen_ai.token.type", "value": { "stringValue": "input" } },
        { "key": "tenant", "value": { "stringValue": "acme" } },
        { "key": "gen_ai.request.model", "value": { "stringValue": "gpt-4o" } } ] } ] } } ] }] }]
}'
```

Then confirm the spend landed in the ledger via the status surface:

```bash
curl localhost:8723/status
```

---

## Price book (turn OTLP token metrics into spend — `--pricebook`)

The OTLP source above records **token counts**, but cost is absent from the OTel GenAI
semconv, so Sloppy Joe prices it itself from a static price book. **Without a price book
every model is priced at $0** — the cost ledger stays empty, `state.spend_1h_usd` reads
`0`, and the shipped **cost-guard** recipe never fires (the demo looks broken). Point the
daemon at a price book to close the loop from token metrics to dollars.

`sloppy init` drops a starter `pricebook.yaml.sample` beside your config (and a fuller
[`examples/pricebook.yaml`](../examples/pricebook.yaml) ships in the repo). The prices in
both are **illustrative only and will drift** — copy one to a `*.yaml` file and replace
the numbers with your provider's current rates:

```yaml
# pricebook.yaml — model -> per-1k-token price in USD (a model absent here is priced $0)
gpt-4o:
  input_per_1k: 0.0025
  output_per_1k: 0.01
claude-3-5-sonnet:
  input_per_1k: 0.003
  output_per_1k: 0.015
```

**Wire it** — pass it on the daemon (flag wins over `env`/file), or set `engine.pricebook`
in `sloppy.yaml`:

```bash
sloppyd --pricebook pricebook.yaml
```

**Prove it** — POST the same OTLP token payload as above, then read `/status`: with the
price book loaded the recorded tokens now carry a non-zero dollar cost, so `spend_1h_usd`
climbs and cost-guard has something to guard:

```bash
curl localhost:8723/status
```
