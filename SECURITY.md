# Security Policy

Sloppy Joe sits in the control path for AI operations — it holds scoped gateway
admin / notification tokens (never your provider keys, which stay in the gateway)
and maintains a tamper-evident, signed audit chain. We take its security seriously.

## Supported versions

Pre-1.0: only the latest `main` / latest tagged release receives fixes.

## Reporting a vulnerability

Please report privately via GitHub Security Advisories ("Report a vulnerability"
on the repo's Security tab). Do **not** open a public issue for security reports.

Include: affected version/commit, reproduction, and impact. We aim to acknowledge
within a few days.

## Security-relevant design

- **Provider keys never enter Sloppy Joe.** Only short-lived, scoped admin/notify
  tokens, held by the secret broker (`secrets`), default-deny by capability.
- **Audit log is hash-chained** (`state.ChainHash` / `VerifyChain`); edits to a
  persisted entry are detectable. Append is atomic per backend (SQLite transaction;
  Redis WATCH/MULTI/EXEC).
- **Signed audit checkpoint (length + head anchor).** The hash chain alone has **no
  length anchor**: any valid prefix of a chain is itself a valid chain, so on its own
  `VerifyChain` cannot distinguish truncation, full deletion, or a freshly re-chained
  wholesale replacement from legitimate "less data" — all of those PASS `VerifyChain`.
  To close that gap, a checkpoint-enabled store maintains a **signed checkpoint**
  (`state.Checkpoint`: entry count + head hash + ed25519 signature over
  `CheckpointPayload(count, head_hash)` + the persisted public key), updated **inside
  the same atomic append** so the anchor can never lag the chain it certifies.
  `VerifyAudit` (SQLite + Redis) now recomputes the count and head hash, compares them
  to the persisted checkpoint (fewer rows ⇒ truncation; different head ⇒ replacement;
  a stripped checkpoint over a non-empty chain ⇒ tamper), and verifies the checkpoint
  signature against its persisted public key. This raises the bar from **"any DB
  writer can silently rewrite history"** to **"a DB writer who *also* holds the signing
  key."**
  Residual limitation (honest scope): the checkpoint lives in the **same writable
  store** as the chain. An attacker who holds the signing key — or who rewrites **both**
  the chain **and** the checkpoint (re-signing it under a key the verifier trusts) — can
  still forge an internally consistent, "verified" history. Full resistance requires an
  anchor **outside** the writable store (an external transparency log / periodic offsite
  attestation of the head + count); that is a deliberate follow-up, not yet implemented.
- **Remediation intents are ed25519-signed and independently verifiable.** Each
  applied intent's audit entry persists the exact signed canonical bytes plus the
  full signature; `sloppy audit --verify-sigs` recomputes `Intent.CanonicalBytes`
  and verifies every signature against the persisted public key, reporting
  verified/failed counts and exiting non-zero on any failure (a CI-gateable check).
  Threat model (honest scope): the private signing key lives at `sloppy.key`
  (mode `0600`) and the public key is exported to `sloppy.key.pub`. Anyone holding
  only the public key can verify that an audited intent was signed by the key
  holder and detect any post-hoc edit to a persisted intent field, but **cannot
  forge a verifiable intent** — that requires the private key. Compromise or theft
  of `sloppy.key` defeats this property; protect it like any signing secret. This
  is in-process integrity only — there is no external transparency log / anchoring.
- **Remediation intents are reversible** (durable TTL auto-revert).
- **HTTP API RBAC** is available via `sloppyd --auth` (API-key → scope).
- Dependencies are scanned with `govulncheck` (CI) and CodeQL.
