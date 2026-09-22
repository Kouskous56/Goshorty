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
| Browser injection | CSP, nosniff, frame denial, CORS allowlist | Remove inline CSP allowances when frontend is split |
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

Baseline scan on 2026-09-22 with `govulncheck` v1.8.0 (Go 1.27.0, `vuln.go.dev`
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
symbols are unreachable; the cluster is still fixed in T2 to keep the tree
clean for future code.

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
