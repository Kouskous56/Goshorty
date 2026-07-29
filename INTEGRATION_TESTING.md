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

CI and production are pinned to Go 1.26.5 because earlier 1.26 patch releases
contain reachable standard-library vulnerabilities reported by `govulncheck`.

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

Each PostgreSQL test creates and later removes its own random schema. Never point
`TEST_DATABASE_URL` at a production database.

### Full E2E flow

Start GoShorty with a disposable database, then run:

```bash
bash scripts/e2e.sh http://127.0.0.1:8080
```

The script requires `bash`, `curl`, and `jq`.

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
