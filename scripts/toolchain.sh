#!/usr/bin/env bash

docker_user_bin=""
if [[ -n "${LOCALAPPDATA:-}" ]] && command -v cygpath >/dev/null 2>&1; then
	docker_user_bin="$(cygpath -u "$LOCALAPPDATA")/Programs/DockerDesktop/resources/bin"
fi

# Git Bash does not always inherit installers' Windows PATH updates.
for candidate in \
	"/c/Program Files/Go/bin" \
	"/c/Program Files/Docker/Docker/resources/bin" \
	"$docker_user_bin"; do
	if [[ -d "$candidate" && ":$PATH:" != *":$candidate:"* ]]; then
		PATH="$candidate:$PATH"
	fi
done
export PATH
