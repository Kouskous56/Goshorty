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
