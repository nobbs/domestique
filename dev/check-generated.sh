#!/usr/bin/env sh
set -eu

for file in internal/httpapi/contract/openapi.gen.go internal/webui/app/src/api/generated.ts; do
	git ls-files --error-unmatch "$file" >/dev/null
done

git diff --exit-code -- internal/httpapi/contract/openapi.gen.go internal/webui/app/src/api/generated.ts
