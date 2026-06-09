# 5-Minute Demo Script

The "aha" is seeing a governed remediation fire, get audited, and the audit chain
**catch tampering** — then seeing the same policy gated in CI. No provider keys,
no gateway required (the default actuator logs; point at LiteLLM for the real thing).

Record this once as an asciinema/GIF for the README + partner outreach.

## Setup (10 seconds)

```bash
make build         # bin/sloppy + bin/sloppyd
DB=/tmp/demo.db; rm -f "$DB"
```

## 1. Close the loop (the wedge, in one command)

```bash
./bin/sloppy inject --now --rules examples/rules --db "$DB" examples/signals/cost-spike.json
```
> A cost-burn signal fires a governed remediation: reroute → open issue → page.
> Each action is signed and recorded.

## 2. Show the tamper-evident audit

```bash
./bin/sloppy audit tail --db "$DB"
#   1  intent.applied  route_override target=gpt-4o rule=… sig=…
#   …
#   chain: verified ✓ (3 entries)
```

## 3. Tamper with it — the chain catches it

```bash
sqlite3 "$DB" "UPDATE audit SET detail='(quietly changed)' WHERE seq=1;"
./bin/sloppy audit tail --db "$DB"
#   chain: TAMPERED ✗ (3 entries)
```
> This is the trust story: an automated remediation log you can *prove* wasn't altered.

## 4. Policy is reviewable IaC — gate it in CI

```bash
# Deterministic replay: what WOULD these signals fire? (no side effects)
./bin/sloppy test --replay examples/fixtures/replay.jsonl --rules examples/rules

# Lint the rules — drop this in a PR check, zero infra:
./bin/sloppy rules validate examples/rules
#   ✓ 3 rule(s) valid
```

## 5. The one-liner to leave them with

> "Your AI-incident response is now a file you review in a PR, replay in CI, and
> can prove was never tampered with — running on the gateway you already have."

## Going live (optional, if they want it on their stack)

```bash
export SLOPPY_LITELLM_URL=http://localhost:4000 SLOPPY_TOKEN_LITELLM=sk-…
./bin/sloppyd --rules examples/rules --db "$DB"            # ingest + TTL revert + /status
curl -XPOST localhost:8723/v1/signals -d @examples/signals/cost-spike.json
curl localhost:8723/status
```
> Note: the LiteLLM admin-API body shape is still provisional (NEXT-tier item);
> verify against the partner's instance before relying on the live reroute.
