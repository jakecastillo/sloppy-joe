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
- **Audit log is hash-chained** (`state.ChainHash` / `VerifyChain`); tampering is
  detectable. Append is atomic per backend (SQLite transaction; Redis WATCH/MULTI/EXEC).
- **Remediation intents are ed25519-signed** and reversible.
- **HTTP API RBAC** is available via `sloppyd --auth` (API-key → scope).
- Dependencies are scanned with `govulncheck` (CI) and CodeQL.
