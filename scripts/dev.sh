#!/usr/bin/env bash
set -euo pipefail

script_dir="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
project_root="$(cd "$script_dir/.." && pwd)"
. "$script_dir/toolchain.sh"
env_file="${1:-.env.local}"
env_path="$project_root/$env_file"

if [[ ! -f "$env_path" ]]; then
	echo "$env_file is missing. Run: bash scripts/setup-env.sh" >&2
	exit 1
fi

set -a
# shellcheck disable=SC1090
source "$env_path"
set +a

localdb_port="${LOCALDB_PORT:-5433}"

if command -v docker >/dev/null 2>&1 && docker info >/dev/null 2>&1; then
	docker compose --env-file "$env_path" -f "$project_root/compose.yaml" up -d postgres
	for attempt in $(seq 1 30); do
		if docker compose --env-file "$env_path" -f "$project_root/compose.yaml" exec -T postgres \
			pg_isready -U "$POSTGRES_USER" -d "$POSTGRES_DB" >/dev/null 2>&1; then
			break
		fi
		if [[ "$attempt" -eq 30 ]]; then
			echo "PostgreSQL did not become ready in time" >&2
			exit 1
		fi
		sleep 1
	done
else
	echo "Docker not available; using embedded PostgreSQL on port $localdb_port" >&2
	cd "$project_root"
	mkdir -p ".localdb/data"
	go build -buildvcs=false -o ".localdb/localdb" ./cmd/localdb

	# .env.local points DATABASE_URL at the Docker PostgreSQL on 5432; the
	# embedded instance replaces it so the app gets real persistence.
	DATABASE_URL="postgres://goshorty:goshorty@127.0.0.1:$localdb_port/goshorty?sslmode=disable"
	export DATABASE_URL

	"./.localdb/localdb" -port "$localdb_port" -data ".localdb/data" >&2 &
	localdb_pid=$!
	trap 'kill "$localdb_pid" 2>/dev/null || true' EXIT

	for attempt in $(seq 1 60); do
		if kill -0 "$localdb_pid" 2>/dev/null && (exec 3<>"/dev/tcp/127.0.0.1/$localdb_port") 2>/dev/null; then
			exec 3>&-
			break
		fi
		if [[ "$attempt" -eq 60 ]]; then
			echo "Embedded PostgreSQL did not become ready on port $localdb_port" >&2
			exit 1
		fi
		sleep 1
	done
	echo "Embedded PostgreSQL ready at $DATABASE_URL"
fi

cd "$project_root"
go run .
