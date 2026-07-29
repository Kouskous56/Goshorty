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

cd "$project_root"
go test ./... -count=1 -covermode=atomic -coverprofile=coverage.out
bash scripts/check-coverage.sh coverage.out 30
go vet ./...
go build ./...
