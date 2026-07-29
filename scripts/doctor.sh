#!/usr/bin/env bash
set -euo pipefail

script_dir="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
project_root="$(cd "$script_dir/.." && pwd)"
. "$script_dir/toolchain.sh"
env_file="${1:-.env.local}"
env_path="$project_root/$env_file"
failures=0

check() {
	local description="$1"
	shift
	if "$@" >/dev/null 2>&1; then
		printf '[OK]   %s\n' "$description"
	else
		printf '[FAIL] %s\n' "$description"
		failures=$((failures + 1))
	fi
}

check "Go is installed" command -v go
if command -v go >/dev/null 2>&1; then
	required="$(tr -d '[:space:]' <"$project_root/.go-version")"
	version="$(GOTOOLCHAIN=auto go version 2>/dev/null || true)"
	if [[ "$version" == *"go${required} "* ]]; then
		echo "[OK]   Go $required is active"
	else
		echo "[FAIL] Go $required is active"
		failures=$((failures + 1))
	fi
fi

if [[ -f "$env_path" ]]; then
	echo "[OK]   $env_file exists"
	set -a
	# shellcheck disable=SC1090
	source "$env_path"
	set +a
	for name in SECRET_KEY ADMIN_PASSWORD PUBLIC_BASE_URL DATABASE_URL ALLOWED_ORIGINS TRUSTED_PROXIES; do
		if [[ -n "${!name:-}" ]]; then
			echo "[OK]   $name is configured"
		else
			echo "[FAIL] $name is configured"
			failures=$((failures + 1))
		fi
	done
	if [[ "${SECRET_KEY:-}" == "GENERATE_ME" ]]; then
		echo "[FAIL] SECRET_KEY is not a template placeholder"
		failures=$((failures + 1))
	else
		echo "[OK]   SECRET_KEY is not a template placeholder"
	fi
else
	echo "[FAIL] $env_file exists"
	failures=$((failures + 1))
fi

check "Docker CLI is installed" command -v docker
if command -v docker >/dev/null 2>&1; then
	check "Docker engine is running" docker info
	if [[ -f "$env_path" ]] && docker info >/dev/null 2>&1; then
		check "compose.yaml and environment values are valid" \
			docker compose --env-file "$env_path" -f "$project_root/compose.yaml" config --quiet
	fi
fi

if ((failures > 0)); then
	echo
	echo "Environment doctor found $failures problem(s)."
	exit 1
fi
echo
echo "Development environment is ready."
