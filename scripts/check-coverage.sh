#!/usr/bin/env bash
set -euo pipefail

coverage_file="${1:-coverage.out}"
minimum="${2:-30}"

if [[ ! -f "$coverage_file" ]]; then
	echo "coverage file not found: $coverage_file" >&2
	exit 1
fi

total="$(go tool cover -func="$coverage_file" | awk '/^total:/ {gsub(/%/, "", $3); print $3}')"
if [[ -z "$total" ]]; then
	echo "unable to determine total coverage" >&2
	exit 1
fi

echo "Total coverage: ${total}% (minimum: ${minimum}%)"
awk -v total="$total" -v minimum="$minimum" 'BEGIN {
  if (total + 0 < minimum + 0) {
    printf "coverage %.1f%% is below required %.1f%%\n", total, minimum > "/dev/stderr"
    exit 1
  }
}'
