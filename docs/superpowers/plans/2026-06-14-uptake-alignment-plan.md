# Sloppy Joe — Uptake & Alignment Plan (2026-06-14)

> From a 4-lens parallel research pass (distribution · usability-for-many · AI-native
> integration · OSS-adoption patterns) synthesized into a ranked plan. Goal: make the
> tool **easy to get, easy to use by many people, and AI-agent-reachable** — without
> violating the load-bearing invariants (not a gateway, never holds provider keys, off
> the inference hot path, surfaces-not-scope).

## The single most important alignment

**Sloppy Joe already ships the hard part and hides it.** The release backend
(`.goreleaser.yaml` + `release.yml`) already cross-compiles both binaries for 6 OS/arch
combos with **checksums, SBOM (syft), cosign keyless signing, and SLSA provenance**, and
**v0.1.0 is tagged** — so signed artifacts exist on the Releases page right now. Yet every
doc says only `go build`. **The #1 uptake lever is pure distribution surfacing + last-mile
packaging — near-zero new infra, high leverage.** Second lever: turn the existing *read*
surfaces (audit/replay/recipe/validate/status) into agent- and FinOps-reachable surfaces
(`sloppy mcp`, `--json`, `sloppy report`) — AI-native uptake with **no new write path**.

## Ranked moves

| # | Move | Effort | Autonomous | Theme |
|---|---|---|---|---|
| 1 | **Install section** in README + quickstart (surface the signed binaries + `go install` + verify) | S | ✅ | distribution |
| 2 | `.goreleaser.yaml`: **nfpm (deb/rpm/apk)** + homebrew_casks + scoops blocks | M | ✅ (nfpm); tap push = human | distribution |
| 3 | **`install.sh`** — `uname` detect → fetch release archive → cosign/checksum verify → `/usr/local/bin` | M | ✅ | distribution |
| 4 | README **badges + one-line nav** header (CI, release, license, Go Report) | S | ✅ | distribution |
| 5 | **`sloppy mcp`** — read-only MCP stdio server over audit/replay/recipe/validate/status | M | ✅ | ai |
| 6 | **`--json`** output for `audit tail` + `test --replay` (machine-readable; feeds MCP narration) | S | ✅ | ai |
| 7 | **`sloppy report`** — read-only audit summary + spend-since (table/json/csv) for FinOps/auditors | M | ✅ | ai |
| 8 | **`make demo`** + fix the empty-rules `init` dead-end (sub-60s first-value) | S | ✅ | workflows |

## Human-gated follow-ups (flagged, NOT done autonomously — touch guarded surfaces)

- **`.github/workflows/release.yml`** edits for: GHCR/container publish (`packages: write`,
  ghcr login, qemu/buildx), and cross-repo **Homebrew tap / Scoop bucket** push tokens
  (needs a PAT secret + two new external repos `homebrew-tap`, `scoop-bucket`). The
  `.goreleaser.yaml` config + `docker-compose image:` swap land autonomously; the workflow
  + token wiring is the owner's step.
- **CI actions Node-20 → Node-24** bump (dated Jun 16 2026; `.github/workflows/` guarded).
- **Per-model spend rollup** (`SpendByModel` GROUP BY) for `sloppy report` — needs
  `state/sqlite.go`/`state/redis.go`/`state/store.go` (forbidden). The autonomous `report`
  is scoped to the existing `SpendSince` + `Audit` queries only.
- **Flipping the "contributions welcome once v0 design is locked" gate** + seeding
  good-first-issues — an owner product/timing decision (issue templates can be scaffolded,
  the gate wording stays).

## Do-not (hard constraints from the research, enforced)

- **No in-binary LLM/model SDK** or server-side model calls (`rules generate --prompt`,
  `audit explain` that calls a model). NL→CEL and audit-narration stay **client-side**:
  sloppy ships the schema/resource + the deterministic `rules validate` gate; the agent
  generates. Keeps "never holds your provider keys" intact.
- **No new ActionKind / no inference-proxying or key-holding adapter.** Gateway breadth
  stays behind the existing Actuator interface (admin token via the `SLOPPY_TOKEN_*` broker).
- **No write/actuation through MCP** — read-only resources/tools; omitting inject/Apply is
  what keeps it invariant-safe.

## Sequencing

Wave A (parallel, file-disjoint): #1+#4 (README/docs), #2 (goreleaser), #3 (install.sh),
#8 (Makefile + init). Then the `cmd/sloppy/main.go`-touching trio #6 (`--json`), #7
(`report`), #5 (`mcp`) run **one per cycle** (they share the command dispatch file, so
serialize to avoid rebase conflicts). The MCP server adds one pure-Go dep
(`modelcontextprotocol/go-sdk`) — CI build + govulncheck gate it.
