#!/usr/bin/env bash
set -euo pipefail

base_url="${1:-http://127.0.0.1:8080}"
work_dir="$(mktemp -d)"
trap 'rm -rf "$work_dir"' EXIT

request() {
	local method="$1"
	local path="$2"
	local body="${3:-}"
	local token="${4:-}"
	local args=(-sS -o "$work_dir/body.json" -w "%{http_code}" -X "$method")
	if [[ -n "$body" ]]; then
		args+=(-H "Content-Type: application/json" --data "$body")
	fi
	if [[ -n "$token" ]]; then
		args+=(-H "Authorization: Bearer $token")
	fi
	curl "${args[@]}" "$base_url$path"
}

expect_status() {
	local actual="$1"
	local expected="$2"
	local step="$3"
	if [[ "$actual" != "$expected" ]]; then
		echo "$step: expected HTTP $expected, got $actual" >&2
		cat "$work_dir/body.json" >&2 || true
		exit 1
	fi
}

echo "Checking liveness and PostgreSQL readiness"
status="$(request GET /health)"
expect_status "$status" 200 "health"
jq -e '.status == "ok"' "$work_dir/body.json" >/dev/null
status="$(request GET /ready)"
expect_status "$status" 200 "ready"
jq -e '.status == "ready"' "$work_dir/body.json" >/dev/null

suffix="${RANDOM}${RANDOM}"
username="e2e${suffix}"
email="${username}@example.test"
old_password="e2e-old-password-${suffix}"
new_password="e2e-new-password-${suffix}"
code="e2e${suffix}"
target="https://example.com/e2e-target"

echo "Registering a user"
status="$(request POST /api/auth/register \
	"$(jq -nc --arg u "$username" --arg p "$old_password" --arg e "$email" \
		'{username:$u,password:$p,email:$e}')")"
expect_status "$status" 201 "register"
token="$(jq -er '.token' "$work_dir/body.json")"
jq -e --arg u "$username" '.user.username == $u and .user.role == "user"' "$work_dir/body.json" >/dev/null

echo "Creating and reading a short URL"
status="$(request POST /api/shorten \
	"$(jq -nc --arg url "$target" --arg code "$code" \
		'{url:$url,expires_in:"24h",custom_code:$code}')" "$token")"
expect_status "$status" 201 "shorten"
jq -e --arg code "$code" '.short_code == $code' "$work_dir/body.json" >/dev/null

status="$(request GET "/api/shorten/$code" "" "$token")"
expect_status "$status" 200 "URL info"
jq -e --arg target "$target" '.original_url == $target and .visits == 0' "$work_dir/body.json" >/dev/null

echo "Checking redirect and visit statistics"
redirect_status="$(curl -sS -o /dev/null -D "$work_dir/headers" --max-redirs 0 \
	-w "%{http_code}" "$base_url/goshorty/24h/$code")"
expect_status "$redirect_status" 302 "redirect"
grep -qi "^Location: ${target}" "$work_dir/headers"

status="$(request GET /api/stats "" "$token")"
expect_status "$status" 200 "stats"
jq -e '.total_urls == 1 and .total_visits == 1' "$work_dir/body.json" >/dev/null

echo "Rotating the password"
status="$(request PUT /api/auth/password \
	"$(jq -nc --arg current "$old_password" --arg next "$new_password" \
		'{current_password:$current,new_password:$next}')" "$token")"
expect_status "$status" 200 "change password"
status="$(request GET /api/auth/me "" "$token")"
expect_status "$status" 401 "old token after password change"

status="$(request POST /api/auth/login \
	"$(jq -nc --arg u "$username" --arg p "$old_password" '{username:$u,password:$p}')")"
expect_status "$status" 401 "old password login"

status="$(request POST /api/auth/login \
	"$(jq -nc --arg u "$username" --arg p "$new_password" '{username:$u,password:$p}')")"
expect_status "$status" 200 "new password login"
new_token="$(jq -er '.token' "$work_dir/body.json")"

echo "Deleting the URL and verifying the final state"
status="$(request DELETE "/api/shorten/$code" "" "$new_token")"
expect_status "$status" 200 "delete URL"
status="$(request GET /api/shorten/all "" "$new_token")"
expect_status "$status" 200 "list URLs"
jq -e '.urls | length == 0' "$work_dir/body.json" >/dev/null

echo "Revoking all sessions"
status="$(request POST /api/auth/revoke "" "$new_token")"
expect_status "$status" 200 "revoke sessions"
status="$(request GET /api/auth/me "" "$new_token")"
expect_status "$status" 401 "revoked token"

echo "E2E API flow passed"
