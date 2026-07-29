.PHONY: help setup doctor db-up db-down db-logs backup build run dev test test-local smoke coverage race integration e2e fmt-check vet vuln ci clean install-deps tidy

help:
	@echo "GoShorty - URL Shortener with TTL"
	@echo ""
	@echo "Available commands:"
	@echo "  make setup          - Create an ignored .env.local with a random secret"
	@echo "  make doctor         - Validate Go, Docker, Compose, and local env"
	@echo "  make db-up          - Start local PostgreSQL 16"
	@echo "  make db-down        - Stop local PostgreSQL without deleting data"
	@echo "  make db-logs        - Follow local PostgreSQL logs"
	@echo "  make backup         - Create a local PostgreSQL custom-format backup"
	@echo "  make install-deps    - Download Go dependencies"
	@echo "  make build          - Build the executable"
	@echo "  make run            - Run the server"
	@echo "  make dev            - Run in development mode (go run)"
	@echo "  make test           - Run tests"
	@echo "  make test-local     - Run tests against local PostgreSQL"
	@echo "  make smoke          - Verify health, metrics, request ID, and JSON logs"
	@echo "  make coverage       - Run tests and enforce the coverage floor"
	@echo "  make race           - Run Linux/CGO race detector"
	@echo "  make integration    - Run PostgreSQL tests using TEST_DATABASE_URL"
	@echo "  make e2e            - Run E2E against an already running server"
	@echo "  make ci             - Run the local CI-equivalent quality checks"
	@echo "  make clean          - Remove built executable"
	@echo "  make tidy           - Tidy and download dependencies"

setup:
	bash scripts/setup-env.sh

doctor:
	bash scripts/doctor.sh

db-up:
	docker compose --env-file .env.local up -d postgres

db-down:
	docker compose --env-file .env.local down

db-logs:
	docker compose --env-file .env.local logs -f postgres

backup:
	powershell -NoProfile -ExecutionPolicy Bypass -File scripts/backup-local.ps1

install-deps:
	go mod download

build: install-deps
	go build -trimpath -o goshorty .

run: build
	./goshorty.exe

dev:
	bash scripts/dev.sh

test:
	go test ./... -count=1

test-local:
	bash scripts/test-local.sh

smoke: build
	powershell -NoProfile -ExecutionPolicy Bypass -File scripts/smoke-local.ps1 -Binary goshorty.exe

coverage:
	go test ./... -count=1 -covermode=atomic -coverprofile=coverage.out
	bash scripts/check-coverage.sh coverage.out 30

race:
	CGO_ENABLED=1 go test -race ./... -count=1

integration:
	go test ./storage -count=1 -v -covermode=atomic -coverprofile=storage-coverage.out
	bash scripts/check-coverage.sh storage-coverage.out 65

e2e:
	bash scripts/e2e.sh

fmt-check:
	test -z "$$(gofmt -l .)"

vet:
	go vet ./...

vuln:
	go run golang.org/x/vuln/cmd/govulncheck@latest ./...

ci: fmt-check vet test coverage build

clean:
	rm -f goshorty goshorty.exe coverage.out storage-coverage.out

tidy:
	go mod tidy

.DEFAULT_GOAL := help
