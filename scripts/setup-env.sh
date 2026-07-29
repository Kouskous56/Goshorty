#!/usr/bin/env bash
set -euo pipefail

script_dir="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
project_root="$(cd "$script_dir/.." && pwd)"
destination="${1:-.env.local}"
destination_path="$project_root/$destination"

if [[ -e "$destination_path" ]]; then
	echo "$destination already exists; no values were overwritten."
	exit 0
fi

if command -v openssl >/dev/null 2>&1; then
	secret="$(openssl rand -hex 32)"
else
	echo "openssl is required to generate a local SECRET_KEY" >&2
	exit 1
fi

sed "s/^SECRET_KEY=GENERATE_ME$/SECRET_KEY=$secret/" \
	"$project_root/.env.example" >"$destination_path"
chmod 600 "$destination_path"

echo "Created $destination with a random local SECRET_KEY."
echo "Local admin password: local-admin-password-change-me"
echo "This file is ignored by Git. Do not reuse its credentials elsewhere."
