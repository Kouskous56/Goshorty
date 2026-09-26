# GoShorty CI and integration testing

This document describes the automated quality gate introduced in phase 3B.

## Continuous integration

GitHub Actions runs `.github/workflows/ci.yml` for:

- every pull request;
- every push to `main`;
- manual `workflow_dispatch` runs.

Only read access to repository contents is granted to the workflow. Concurrent
runs for the same branch are cancelled so obsolete builds do not waste minutes.

### Quality and security job

The quality job performs:

1. `go mod verify`;
2. `go mod tidy` followed by a clean-diff check for `go.mod` and `go.sum`;
3. `gofmt` verification;
4. `go vet ./...`;
5. all unit tests with atomic coverage;
6. aggregate coverage threshold of 30%;
7. a clean production build;
8. the official Go `govulncheck` action.

The current local aggregate coverage is above the initial 30% floor. This floor
is a regression guard, not the final target. Raise it as coverage improves.

CI and production are pinned to Go 1.26.8 because earlier 1.26 patch releases
(≤ 1.26.5) contain reachable standard-library vulnerabilities reported by
`govulncheck` (fixed in 1.26.6).

### Race detector job

Linux CI runs:

```bash
CGO_ENABLED=1 go test -race ./... -count=1
```

This is separate from the normal test job because the race build is slower and
requires CGO.

### PostgreSQL integration and E2E job

The job starts an isolated PostgreSQL 16 service container, then:

1. runs all storage tests with `TEST_DATABASE_URL`;
2. checks that storage coverage is at least 65%;
3. builds the real GoShorty binary;
4. starts it in `GIN_MODE=release`;
5. waits for `/ready`;
6. runs `scripts/e2e.sh`.

The E2E flow verifies:

- liveness and PostgreSQL readiness;
- user registration;
- authenticated URL creation;
- URL ownership lookup;
- redirect status and destination;
- atomic visit statistics;
- password rotation;
- rejection of the old password;
- acceptance of the new password;
- authenticated deletion and final database state.

All E2E data lives only in the disposable CI PostgreSQL service.

### Browser E2E (Playwright) job

The browser job builds the real binary and runs it directly against the
in-memory development storage (no `DATABASE_URL`, no `GIN_MODE=release`), then
drives the SPA from a real Chromium through Playwright:

1. sets up Node 22 and installs `@playwright/test` plus Chromium (with system
   dependencies via `npx playwright install --with-deps chromium`);
2. starts GoShorty on `:8080` and waits for `/ready`;
3. runs the `e2e/tests/spa.spec.ts` suite with the `chromium` project
   (`workers: 1`).

The suite covers:

- the happy path: register → create a short URL → public `/r/:code` redirect →
  logout → login again;
- the T7 session-restore regression: a tampered `localStorage` token is
  rejected on reload with the "Session expired, please login again" message;
- the admin / regular-user tab split, including the exact admin row in the
  users table.

It is a real-browser, black-box SPA check on top of the API-level E2E. The job
sets `REGISTER_LIMIT_PER_HOUR: 50` so the three registrations per run (plus any
Playwright retries) stay far below the production default of 5/hour — this
keeps the suite deterministic without weakening the production limit.

## Running tests locally

### Unit tests

```bash
go test ./... -count=1
go vet ./...
go build ./...
```

### Coverage

```bash
go test ./... -covermode=atomic -coverprofile=coverage.out
bash scripts/check-coverage.sh coverage.out 30
go tool cover -html=coverage.out
```

### Race detector

Run on Linux, WSL, or another environment with CGO and a C compiler:

```bash
CGO_ENABLED=1 go test -race ./... -count=1
```

### PostgreSQL integration tests

Create a disposable database and provide its URL:

```bash
export TEST_DATABASE_URL='postgres://postgres:postgres@127.0.0.1:5432/goshorty_test?sslmode=disable'
go test ./storage -count=1 -v
```

Without Docker or a local server, run the same suite against an embedded
PostgreSQL instance (the platform binary is downloaded from Maven Central on
first use):

```bash
EMBEDDED_PG=1 go test ./storage -count=1 -v
```

Each PostgreSQL test creates and later removes its own random schema. Never point
`TEST_DATABASE_URL` at a production database.

### Full E2E flow

Start GoShorty with a disposable database, then run:

```bash
bash scripts/e2e.sh http://127.0.0.1:8080
```

The script requires `bash`, `curl`, and `jq`.

### Browser E2E (local)

Serve the app from a terminal (in-memory dev mode, no database needed):

```powershell
$env:SECRET_KEY='dev-only-secret-key-0123456789'
$env:ADMIN_PASSWORD='admin123'
$env:ADMIN_EMAIL='admin@local.test'
$env:PUBLIC_BASE_URL='http://127.0.0.1:8080'
$env:ALLOWED_ORIGINS='http://127.0.0.1:8080'
$env:TRUSTED_PROXIES='127.0.0.1'
$env:REGISTER_LIMIT_PER_HOUR='50'   # default 5/hour is for production
go run .
```

From another terminal:

```bash
cd e2e
npm install
npm run test:local        # uses the system Chrome; no browser download
```

The `local-chrome` project targets the machine's installed Chrome via
`channel: 'chrome'`. Raise `REGISTER_LIMIT_PER_HOUR` for local runs so repeat
runs and Playwright retries never hit the production 5/hour/IP registration
limit; the server holds the counter in memory, so restarting it also resets the
budget.

## Coverage policy

- Aggregate project coverage must not fall below 30%.
- Storage coverage with PostgreSQL integration must not fall below 65%.
- New bug fixes should include a regression test.
- New storage methods should be covered by both the in-memory implementation and
  PostgreSQL integration tests where applicable.
- Do not lower a threshold merely to make CI green. Fix the test gap or explain
  and review the exceptional case.

## Troubleshooting

### Module files are dirty

Run:

```bash
go mod tidy
git diff -- go.mod go.sum
```

Commit intentional dependency changes.

### PostgreSQL job cannot become ready

Inspect the service-container health output and the `GoShorty E2E server log`
section printed by the workflow cleanup trap.

### E2E receives HTTP 429

The script deliberately stays below production rate limits. A 429 generally
means the limits or the test flow changed without being updated together.

### govulncheck fails

Read the reported reachable call stack, upgrade the affected dependency, rerun
the complete test suite, and document any compatibility impact.
