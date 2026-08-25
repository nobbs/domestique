#!/usr/bin/env sh
set -eu

for file in internal/httpapi/contract/openapi.gen.go internal/readiness/contract/openapi.gen.go \
	internal/webui/app/src/api/generated.ts; do
	git ls-files --error-unmatch "$file" >/dev/null
done

# Against HEAD, not the index: a plain `git diff` compares the working tree to
# what is staged, so bindings regenerated and then `git add`ed — which is the
# order a pre-commit hook produces — would read as clean while differing from
# the committed contract.
git diff HEAD --exit-code -- internal/httpapi/contract/openapi.gen.go \
	internal/readiness/contract/openapi.gen.go internal/webui/app/src/api/generated.ts
