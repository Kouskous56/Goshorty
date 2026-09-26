# GoShorty environments

GoShorty uses the same application binary in development, test, CI, and
production. Only environment variables and external infrastructure differ.

## Required toolchain

- Go 1.26.8
- Git
- Docker Desktop or Docker Engine with Compose v2 (optional — without it, the
  scripts fall back to an embedded PostgreSQL server)
- Bash for the portable scripts, or Windows PowerShell 5.1+/PowerShell 7
- `curl` and `jq` for the E2E suite

The required Go version is declared in all relevant places:

- `go.mod`
- `.go-version`
- `.tool-versions`
- `.github/workflows/ci.yml`

Run the environment doctor before starting work:

```powershell
powershell -NoProfile -ExecutionPolicy Bypass -File scripts/doctor.ps1
```

or:

```bash
bash scripts/doctor.sh
```

## Development environment

Create the ignored local environment file:

```powershell
powershell -NoProfile -ExecutionPolicy Bypass -File scripts/setup-env.ps1
```

or:

```bash
bash scripts/setup-env.sh
```

The setup command:

- copies `.env.example` to `.env.local`;
- generates a cryptographically random local `SECRET_KEY`;
- never overwrites an existing `.env.local`;
- leaves the file outside Git tracking.

Start PostgreSQL and the app:

```powershell
powershell -NoProfile -ExecutionPolicy Bypass -File scripts/dev.ps1
```

or:

```bash
bash scripts/dev.sh
```

Open `http://127.0.0.1:8080`.

`dev.ps1`/`dev.sh` detect Docker automatically: with Docker they start the
Compose PostgreSQL, without it they start the embedded PostgreSQL server
(`cmd/localdb`, persistent data under `.localdb/`) and point `DATABASE_URL` at it.

Default local admin credentials:

```text
username: admin
password: local-admin-password-change-me
```

These values are only for the developer's private machine.

## Local PostgreSQL

`compose.yaml` runs PostgreSQL 16 with:

- a named persistent volume;
- a healthcheck;
- configurable host port;
- no application container, so Go still runs directly for fast iteration.

Useful commands:

```bash
docker compose --env-file .env.local up -d postgres
docker compose --env-file .env.local logs -f postgres
docker compose --env-file .env.local down
```

`down` keeps the named database volume. To delete local database data, use
`docker compose down -v` deliberately; this is destructive and is not included
in any project script.

### Embedded PostgreSQL (no Docker)

When Docker is unavailable, `cmd/localdb` runs a real PostgreSQL server as a
child process. The platform binary is downloaded once from Maven Central into
the user cache (`~/.embedded-postgres-go`); database files persist under
`.localdb/data` (git-ignored) so restarts keep the data.

```bash
go run ./cmd/localdb
```

Flags: `-port` (default `5433`, override with `LOCALDB_PORT`), `-data`
(default `.localdb/data`), `-user`/`-password`/`-database` (default
`goshorty`/`goshorty`/`goshorty`). It prints the ready `DATABASE_URL`; press
Ctrl+C to stop. `scripts/dev.ps1` / `scripts/dev.sh` run it automatically when
no Docker engine is detected.

## Test environment

`.env.test.example` contains deterministic, non-production test values. CI has
the same class of isolated settings directly in its workflow.

Run unit, PostgreSQL integration, vet, coverage, and build checks locally:

```powershell
powershell -NoProfile -ExecutionPolicy Bypass -File scripts/test-local.ps1
```

or:

```bash
bash scripts/test-local.sh
```

PostgreSQL tests create random schemas and remove them afterward. Even so,
`TEST_DATABASE_URL` must never point to production.

When Docker is unavailable, `test-local.ps1`/`test-local.sh` set `EMBEDDED_PG=1`
instead and the storage suite runs against an embedded PostgreSQL instance
(see `INTEGRATION_TESTING.md`).

## CI environment

GitHub Actions creates an ephemeral PostgreSQL 16 service and supplies all
variables inside the job. CI does not require repository secrets because every
credential is test-only and the database is destroyed with the runner.

CI uses Go 1.26.8 and runs:

- module verification;
- formatting and vet;
- unit tests and coverage thresholds;
- race detector;
- reachable vulnerability scanning;
- PostgreSQL integration;
- release-mode E2E.

See `INTEGRATION_TESTING.md`.

Production operations, backup/restore drills, observability signals, and secret
rotation procedures are defined in `OPERATIONS.md`. The corresponding threat
model is in `SECURITY.md`.

## Production environment

Production variables live only in Railway Variables. Use
`.env.production.example` as a checklist, never as a source of real values.

Required:

| Variable | Purpose |
|---|---|
| `GIN_MODE=release` | Disables Gin debug mode and enables release safeguards |
| `SECRET_KEY` | HMAC signing key; use at least 32 random bytes |
| `ADMIN_PASSWORD` | Bootstrap admin password, 12–72 bytes |
| `DATABASE_URL` | Same-project Railway PostgreSQL reference |
| `PUBLIC_BASE_URL` | Canonical HTTPS public URL |
| `METRICS_TOKEN` | Bearer token of at least 32 bytes for `/metrics` |

Recommended:

| Variable | Default |
|---|---|
| `ADMIN_EMAIL` | `admin@goshorty.local` |
| `TOKEN_TTL` | `24h` |
| `TOKEN_ISSUER` | empty — issuer claim disabled (backward compatible) |
| `TOKEN_AUDIENCE` | empty — audience claim disabled (backward compatible) |
| `SECRET_KEY_PREVIOUS` | empty — previous signing key for smooth rotation |
| `ALLOWED_ORIGINS` | `PUBLIC_BASE_URL` |
| `TRUSTED_PROXIES` | `100.64.0.0/10` |
| `REGISTER_LIMIT_PER_HOUR` | `5` — per-IP registration budget per hour (raise it for test/CI suites, e.g. 50) |

Railway supplies `PORT`.

Changing `ADMIN_PASSWORD` after the admin exists does not update its stored
bcrypt hash. Rotate the actual password through the authenticated Security tab.

## Secret rules

- Never commit `.env.local`, `.env`, Railway exports, tokens, or database URLs
  containing real credentials.
- Never reuse local or CI credentials in production.
- Rotate a secret immediately if it appears in a log, issue, commit, or chat.
- Keep `SECRET_KEY` stable across normal deploys; changing it invalidates all
  existing tokens unless `SECRET_KEY_PREVIOUS` carries the previous value
  (smooth rotation, see `OPERATIONS.md`).
- Prefer Railway same-project references over public database URLs.

The `.gitignore` rules ignore all `.env*` files except committed `*.example`
templates.
