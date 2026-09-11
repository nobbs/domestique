#!/usr/bin/env sh
set -eu

# biome.json lives in internal/webui/app, so biome must run there; strip the
# repo-root-relative prefix prek supplies from each staged path.
cd "$(dirname "$0")/../internal/webui/app"

# Rebuilds "$@" prefix-stripped, one quoted argument at a time: a POSIX shell
# has no arrays, and an unquoted rebuild would word-split or glob-expand a path.
count=$#
while [ "$count" -gt 0 ]; do
	stripped="${1#internal/webui/app/}"
	shift
	set -- "$@" "$stripped"
	count=$((count - 1))
done

exec ./node_modules/.bin/biome format "$@"
