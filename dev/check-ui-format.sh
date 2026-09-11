#!/usr/bin/env sh
set -eu

# biome.json lives in internal/webui/app, so biome must run there; strip the
# repo-root-relative prefix prek supplies from each staged path.
cd "$(dirname "$0")/../internal/webui/app"

stripped=
for f in "$@"; do
	stripped="$stripped ${f#internal/webui/app/}"
done

# shellcheck disable=SC2086 # word-splitting is the point: rebuilding args.
exec ./node_modules/.bin/biome format $stripped
