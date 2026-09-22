# GoShorty threat model

## Assets and trust boundaries

Primary assets are account credentials, signed tokens, short-link ownership,
redirect destinations, operational secrets, and PostgreSQL data. Requests cross
the public Railway proxy before reaching Gin; only configured proxy ranges are
trusted. PostgreSQL and Railway variables are privileged infrastructure.

## Material threats and controls

| Threat | Current control | Residual action |
|---|---|---|
| Credential stuffing | bcrypt, generic login errors, login rate limit | Monitor 401/rate-limit trends |
| Stolen bearer token | expiry, live user lookup, `token_version` revocation | Keep `TOKEN_TTL` short enough for risk |
| Stale admin privilege | role reloaded on every request | Audit role changes in platform logs |
| Last-admin removal | storage and transaction invariant | Maintain a documented recovery owner |
| Short-code race | atomic memory reservation/PostgreSQL unique constraint | Monitor conflict rate |
| Cross-user data access | owner-scoped queries and admin checks | Keep authorization regression tests |
| Host/proxy spoofing | validated `PUBLIC_BASE_URL`, trusted proxy allowlist | Review Railway proxy range changes |
| Browser injection | strict CSP (`script-src 'self'`, no inline code), nosniff, frame denial, CORS allowlist | Keep the SPA free of inline scripts/handlers (regression-tested) |
| Oversized/slow requests | body cap and HTTP server timeouts | Apply upstream edge limits too |
| Open redirect abuse | only stored, validated HTTP(S) destinations | Add reputation controls if service becomes public |
| Log/metric leakage | route templates; no bodies, auth headers, or raw URLs; metrics Bearer token in release | Restrict platform log access |
| Database loss | persistent volume, dump/restore runbook and drills | Enable provider backup appropriate to plan |
| Migration drift | SHA-256 checksum ledger and startup refusal on mismatch | Never edit an applied migration |

## Token revocation semantics

Each token contains the user's persisted `token_version`. Authentication reloads
the user and requires an exact version match. Password changes update the bcrypt
hash and increment the version atomically. Existing version-0 tokens remain
compatible until that user's first password rotation. User deletion revokes all
tokens because live lookup fails. Rotating `SECRET_KEY` is the emergency global
revocation mechanism.

`POST /api/auth/revoke` increments the same version without changing the
password. It invalidates all browser/API sessions for that user, including the
token used to request revocation.

## Token format and hardening

Tokens keep the self-designed format `base64url(payload).base64url(signature)`
signed with HMAC-SHA256 (no JWT library). Verification checks the signature —
length-constrained to the exact SHA-256 size — **before** parsing the payload,
and rejects tokens larger than 4 KB.

Claims:

| Claim | Meaning |
|---|---|
| `jti` | random per-token ID (audit/correlation; enables per-session revocation later) |
| `user_id`, `username`, `role`, `token_version` | identity + live revocation version |
| `iss`, `aud` | optional issuer/audience; empty on both sides = not enforced |
| `kid` | signing key id (`v1`); legacy tokens carry none |
| `issued_at`, `expires_at` | issuance and expiry timestamps |

Additional verification rules (all additive, existing tokens stay valid):

- `issued_at` in the future (beyond 30 s skew) is rejected.
- Token lifetime (`expires_at - issued_at`) may not exceed the configured
  `TOKEN_TTL`; a leaked signing key cannot mint indefinitely valid tokens.
- `iss`/`aud` are enforced only when `TOKEN_ISSUER`/`TOKEN_AUDIENCE` are set;
  enabling them invalidates previously issued tokens (clients re-login).
- `kid` must be `v1`; unknown key ids are rejected.

Key rotation is smooth via `SECRET_KEY_PREVIOUS`: set the new value in
`SECRET_KEY`, move the old value into `SECRET_KEY_PREVIOUS` for the transition
window, then remove it. During the window new tokens are signed with the active
key while legacy kid-less tokens still verify against the previous key.
Rotating `SECRET_KEY` without a previous value remains the emergency
global-revocation mechanism.

Login performs a real bcrypt comparison against a fixed dummy hash when the
username does not exist, equalizing response timing to resist timing-based
username enumeration.

## Browser session handling

The SPA stores the bearer token in `localStorage` (kept deliberately; an
HttpOnly-cookie migration would require a CSRF defense and a refresh strategy
and is deferred as future work). To bound the blast radius of a stolen or stale
token, the frontend:

- validates the session on page load with `GET /api/v1/auth/me` and replaces the
  cached user object with the server response instead of trusting
  `localStorage`;
- treats any `401` on a protected endpoint as an expired session: clears local
  storage, returns to the login view, and shows a "session expired" notice;
- never generates inline `<script>` blocks or `onclick` attributes, so the strict
  `script-src 'self'` Content Security Policy stays effective;
- targets only the canonical `/api/v1/*` surface (legacy aliases remain
  server-side for compatibility).

## Audit events

Successful authentication, registration, password rotation, session
revocation, role changes, and user deletion emit structured `security_audit`
events. Denied login and duplicate-registration attempts record bounded reason
codes. Events exclude passwords, request bodies, bearer tokens, authorization
headers, and database URLs.

## Security reporting

Do not include live tokens, passwords, `SECRET_KEY`, or credential-bearing
database URLs in reports. Provide the affected route, timestamp, deployment
revision, and `X-Request-ID`.

## Dependency vulnerability posture

Baseline scan on 2026-09-22 (pre-T2) with `govulncheck` v1.8.0 (Go 1.27.0, `vuln.go.dev`
database last modified 2026-09-16), run in both source (`./...`) and binary
modes. Result: **exit 0 — 0 reachable (called) vulnerabilities**. The scan
reported 41 finding instances covering **32 unique advisories**, all in
packages outside the exercised call graph (bcrypt, Gin, pgx).

| Module in build | Unique advisories | Notable advisories | Fix | Latest |
|---|---|---|---|---|
| `golang.org/x/crypto` v0.36.0 | 20 | CVE-2026-39830<sup>1</sup>, CVE-2026-39827, CVE-2026-39828, CVE-2026-39829, CVE-2026-39831…39835, CVE-2025-47913, CVE-2025-47914, CVE-2025-58181, CVE-2026-56854, CVE-2026-56855, GO-2026-5932 (openpgp deprecated/unsafe) | v0.52.0–v0.56.0 | v0.57.0 |
| `golang.org/x/net` v0.38.0 | 10 | CVE-2026-25680, CVE-2026-25681, CVE-2026-27136 (html), CVE-2026-33814 (http2), CVE-2026-46600 (dns/dnsmessage) | v0.45.0–v0.56.0 | v0.59.0 |
| `golang.org/x/sys` v0.31.0 | 1 | CVE-2026-39824 (`windows.NewNTUnicodeString`) | v0.44.0 | v0.48.0 |
| `google.golang.org/protobuf` v1.30.0 | 1 | CVE-2024-24786 (JSON unmarshal loop) | v1.33.0 | v1.36.12 |

<sup>1</sup> The severe SSH cluster (CVE-2026-39827…39835 incl. the critical
CVE-2026-39830 deadlock) lives in `golang.org/x/crypto/ssh`, `ssh/agent`, and
`ssh/knownhosts`. GoShorty imports only `golang.org/x/crypto/bcrypt`, so these
symbols are unreachable; the cluster was fixed in T2 (`x/crypto` v0.57.0).

### Post-upgrade re-scan (2026-09-22, Task 2)

Dependencies upgraded to the newest releases (gin v1.12.0, pgx/v5 v5.11.0,
`x/crypto` v0.57.0, `x/net` v0.59.0, `x/sys` v0.48.0, `x/text` v0.42.0,
protobuf v1.36.12, validator v10.30.5, sonic v1.15.4, quic-go v0.59.1, plus
follow-on indirects; `go` directive, CI, Docker, and `.tool-versions` now at
1.26.8). Re-scan with `govulncheck` v1.8.0: **exit 0 in both source and
binary modes** — a single residual unreachable advisory remains:

| Advisory | Module | Why it remains |
|---|---|---|
| GO-2026-5932 | `golang.org/x/crypto/openpgp` | Deprecated "unsafe by design" upstream; GoShorty has no use for it (imports only bcrypt) and no fixed release exists to move to |

All 32 baseline module advisories and the four toolchain advisories are
resolved by this upgrade; the same database notes `quic-go` < v0.59.1 and
`go.mongodb.org/mongo-driver` GSSAPI issues for modules kept only in the
module graph (they are not linked into the binary — verified with
`go list -deps`).

### Standard library (toolchain)

The first CI run on this baseline also scanned the pinned toolchain's
standard library. Go 1.26.5 was affected by four reachable advisories, all
fixed in `go1.26.6`:

| Advisory | Package | Issue |
|---|---|---|
| GO-2026-6090 | `crypto/tls` | unlimited post-handshake messages |
| GO-2026-6089 | `net/http` | missing `ReadHeaderTimeout` on unencrypted HTTP/2 check |
| GO-2026-6088 | `encoding/xml` | unbounded recursion during decode |
| GO-2026-5972 | `encoding/asn1` | unbounded recursion depth |

CI now pins the newest 1.26 patch (`1.26.8`). Policy: keep the toolchain on
the newest patch of the declared minor; advance the minor (`go` directive in
`go.mod`, CI inputs, Dockerfile) as part of the task that requires it.

### Mitigation and patch policy

- Keep every module at the newest patch via Dependabot (`.github/dependabot.yml`,
  weekly, grouped). Merge security PRs within their severity window: **critical
  ≤ 7 days, high ≤ 30 days**.
- CI must stay **exit 0** on `golang/govulncheck-action`; local gates are
  `make vuln` and `make ci`.
- Every CI run attaches an SPDX SBOM artifact
  (`goshorty-sbom.spdx.json`) generated from the release binary for
  provenance and supply-chain review.
- Re-run this baseline whenever the dependency graph changes and update the
  table (module, date, advisory count) so drift is visible in review.
- If a reachable fix requires a newer Go toolchain, bump `go.mod` and the CI
  `go-version` inputs in the same PR as the dependency change.
