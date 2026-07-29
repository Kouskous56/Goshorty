# GoShorty production operations

## Service signals

| Signal | Endpoint or field | Expected |
|---|---|---|
| Liveness | `GET /health` | HTTP 200 |
| Readiness | `GET /ready` | HTTP 200 and PostgreSQL reachable |
| Metrics | `GET /metrics` | Prometheus text; Bearer token in release |
| Release identity | `GET /version` | Version, commit, build timestamp |
| Correlation | `X-Request-ID` | Returned on every response |
| Logs | stdout | One JSON object per application/request event |

Request metrics use Gin route templates such as `/api/shorten/:code`, not raw
paths. This prevents short codes from creating unbounded labels or appearing in
metrics. Request logs include method, route, status, duration, client IP,
response size, and request ID. They do not include authorization headers,
request bodies, passwords, tokens, database URLs, or redirect destinations.
Security-sensitive actions emit a separate `security_audit` event with actor,
target, outcome, and request ID.

In release mode, scrape metrics with:

```bash
curl -H "Authorization: Bearer $METRICS_TOKEN" https://your-domain.example/metrics
```

Suggested alerts:

- `/ready` fails for two consecutive checks;
- HTTP 5xx exceeds 2% for five minutes;
- p95 request latency exceeds one second for five minutes;
- repeated process restarts;
- database volume approaches its storage limit.

## Local backup and restore drill

Create a custom-format PostgreSQL backup:

```powershell
powershell -NoProfile -ExecutionPolicy Bypass -File scripts/backup-local.ps1
```

The ignored `backups/` directory receives the dump and the command prints its
SHA-256 checksum. Copy important backups to encrypted storage outside the
machine.

Restore into the local Compose database:

```powershell
powershell -NoProfile -ExecutionPolicy Bypass -File scripts/restore-local.ps1 `
  -BackupFile backups/goshorty-YYYYMMDD-HHMMSS.dump `
  -ConfirmDatabaseReset
```

The confirmation flag is mandatory because restore replaces local tables.
Afterward run the environment doctor, integration tests, login, create a short
URL, and verify its redirect.

## Production backup policy

- Back up before every schema migration and at least daily.
- Retain daily backups for 14 days and monthly backups for 6 months.
- Encrypt backups at rest and restrict access separately from application
  credentials.
- Record timestamp, database identifier, application revision, migration
  version, size, and SHA-256 checksum.
- Perform a restore drill into an isolated non-production database monthly.
- A backup is not considered valid until a restore and application smoke test
  succeed.
- Never run the local restore script against Railway; use a separate target
  database and explicit `pg_restore` credentials.

## Incident and secret rotation

1. Rotate `ADMIN_PASSWORD` through the authenticated Security screen. This
   increments `token_version` and immediately revokes that user's old tokens.
   A user can also call `POST /api/auth/revoke` to revoke every current session
   without changing the password.
2. Delete a compromised user to invalidate access immediately.
3. Rotate `SECRET_KEY` only for global session invalidation; it invalidates all
   tokens at once and requires every user to sign in again.
4. Rotate database credentials in Railway, update the reference, redeploy, and
   verify `/ready`.
5. Search structured logs by `request_id`; never paste tokens or database URLs
   into incident notes.

## Deployment verification

1. Confirm Railway variables match `.env.production.example`.
2. Confirm the pre-deploy backup and checksum.
3. Deploy one revision.
4. Verify `/health`, `/ready`, `/version`, and authenticated `/metrics`.
5. Run login → create → redirect → stats → delete.
6. Change a test user's password and confirm its old token returns HTTP 401.
7. Check JSON logs for the same `X-Request-ID` returned to the client.
8. Observe error rate and latency for at least ten minutes.
