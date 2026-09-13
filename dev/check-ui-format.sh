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

# pnpm's shim falls back to `command -v node`; git invokes hooks with
# whatever PATH the client gave it, which may have no shell profile behind
# it at all, so run it through mise's own environment instead of trusting that.
exec mise exec -- ./node_modules/.bin/biome format "$@"
