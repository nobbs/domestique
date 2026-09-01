#!/usr/bin/env sh
set -eu

generated=internal/sqlite/internal/sqlcgen

find "$generated" -type f -name '*.go' -exec git ls-files --error-unmatch '{}' \; >/dev/null
git diff HEAD --exit-code -- "$generated"
