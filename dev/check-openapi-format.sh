#!/usr/bin/env bash
#
# Checks that each passed OpenAPI document is yq's readable block-style YAML.
set -euo pipefail

formatted="$(mktemp)"
trap 'rm -f "${formatted}"' EXIT

for file in "$@"; do
	if ! mise exec -- yq -P "${file}" >"${formatted}"; then
		exit 1
	fi
	if ! diff -u "${file}" "${formatted}"; then
		echo "Run: mise exec -- yq -P -i ${file}" >&2
		exit 1
	fi
done
