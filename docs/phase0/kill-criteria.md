# Kill Criteria & Competitive Landscape

## Where Sloppy Joe sits

Most pieces already exist. The honest question is whether the **seam** Sloppy Joe
owns is real and unowned. ✅ = does it well, 🟡 = partial/possible, ❌ = no.

| Capability | Envoy AI GW | LiteLLM | Portkey | n8n | StackStorm | **Sloppy Joe** |
|---|---|---|---|---|---|---|
| Model-agnostic gateway (route/fallback) | ✅ | ✅ | ✅ | ❌ | ❌ | ❌ (sits on top) |
| Generic event → action automation | ❌ | 🟡 (alerts) | 🟡 (webhooks) | ✅ | ✅ | ✅ |
| **AI-native signals** (cost-burn, eval/quality regression, guardrail trip) | ❌ | 🟡 | 🟡 | ❌ | ❌ | ✅ |
| **Response policy as reviewed Git artifact** (PR + CI replay) | ❌ | ❌ | ❌ | ❌ | 🟡 | ✅ |
| **Signed, reversible remediation + tamper-evident audit** | ❌ | ❌ | ❌ | ❌ | ❌ | ✅ |
| Cross-vendor (works across competing gateways) | ❌ | ❌ | ❌ | 🟡 | 🟡 | ✅ |
| Cost as first-class, queryable state | 🟡 | 🟡 | ✅ | ❌ | ❌ | ✅ |

**The wedge:** the bottom three rows together — *governed, reviewable, signed,
cross-vendor remediation of AI-native signals*. No single row is a moat; the
combination, delivered as reviewable IaC, is the bet.

## Kill criteria (stop or pivot if…)

1. **No pain.** ≥3/5 partners handle AI incidents with existing tooling and don't
   find it painful enough to change. → **STOP.**
2. **Artifact rejected.** Partners don't want response-policy in a PR/CI — they
   prefer a dashboard toggle or an opaque webhook. → **STOP** (the differentiator is moot).
3. **Glue is good enough.** Their n8n + budget-cron + Grafana glue is "fine forever"
   and a one-binary loop isn't materially less painful. → **PIVOT or STOP.**
4. **Incumbent absorption underway.** A gateway they already run is shipping
   "rules-in-Git + audit" this quarter, eliminating the cross-vendor need. →
   re-evaluate the moat (cross-vendor neutrality + signed receipts) honestly.
5. **Wrong wedge.** A *different* adjacent pain (e.g. eval/CI gating, spend
   attribution) is felt far more strongly. → **PIVOT** toward it.

## Survive criteria (proceed if…)

- ≥3/5 already hand-roll the observe→decide→act loop and call it painful, **and**
- ≥3/5 say "yes" to reviewing the automated response in a PR with an audit trail, **and**
- at least one is willing to run the demo against their own gateway.
