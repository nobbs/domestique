#!/usr/bin/env bash
#
# Guards the one property that makes the commit hook worth keeping installed:
# it stays fast.
#
# A hook that scans the whole tree on every commit, or that runs work `make
# check` already owns, becomes slow enough that developers bypass it — and a
# bypassed hook protects nothing. So two structural rules hold in prek.toml:
#
#   * a hook that runs a command takes its files from prek, rather than
#     scanning the repository itself — it must both accept the file list and
#     name no paths of its own, because `pass_filenames = true` alongside a
#     repo-wide glob or package root still scans the tree; and
#   * no hook runs the tests, the full linters, the audits, the
#     cross-compilation, the image build, or the browser suites, which belong
#     to `make check` and to GitHub Actions.
#
# Wall-clock is deliberately not asserted: a timing assertion is flaky on a
# shared runner, and these two properties are what the timing rests on anyway.
#
set -euo pipefail

ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
CONFIG="${1:-${ROOT}/prek.toml}"

if [[ ! -f "${CONFIG}" ]]; then
	echo "check-hook-cost: no such file: ${CONFIG}" >&2
	exit 1
fi

# Commands that belong to the full gate. Matched against a hook's entry, so a
# formatter invoked by name stays allowed while its whole-repository sibling
# does not: `golangci-lint fmt` passes, `golangci-lint run` does not.
EXPENSIVE='(go|npm|npx)[[:space:]]+(test|build|audit|run|ci)|make[[:space:]]|golangci-lint[[:space:]]+run|govulncheck|gitleaks|vitest|docker|actionlint|shellcheck'

# Paths a hook must not name for itself: a recursive glob, a Go package root,
# and a bare directory argument. Written without backslashes because awk
# processes escapes in a -v assignment.
WIDE='[*][*]|[.][.][.]|(^|[[:space:]])[.]($|[[:space:]])'

awk -v expensive="${EXPENSIVE}" -v wide="${WIDE}" -v config="${CONFIG}" '
	function value(line,   parts) {
		if (match(line, /"([^"]*)"/) == 0) {
			return ""
		}
		return substr(line, RSTART + 1, RLENGTH - 2)
	}

	function report(message) {
		printf "check-hook-cost: %s:%s: %s\n", config, hookline, message > "/dev/stderr"
		failed = 1
	}

	function flush(  invocation) {
		if (!inhook || entry == "") {
			inhook = 0
			return
		}
		# A hook invokes what its entry names with what its args add, so both
		# decide the cost and both are read here.
		invocation = entry (args == "" ? "" : " " args)
		if (invocation ~ expensive) {
			report(sprintf("hook %s runs `%s`, which belongs to `make check`, not to a commit hook", id, invocation))
		}
		if (invocation ~ wide) {
			report(sprintf("hook %s names its own paths in `%s`; prek must supply the file list", id, invocation))
		}
		if (pass != "true") {
			report(sprintf("hook %s must set pass_filenames = true so it checks staged files only", id))
		}
		inhook = 0
	}

	/^[[:space:]]*\[\[repos\]\]/ { flush(); next }

	/^[[:space:]]*\[\[repos\.hooks\]\]/ {
		flush()
		inhook = 1
		hookline = NR
		id = "(unnamed)"
		entry = ""
		args = ""
		pass = ""
		next
	}

	!inhook { next }

	/^[[:space:]]*id[[:space:]]*=/ { id = value($0); next }
	/^[[:space:]]*entry[[:space:]]*=/ { entry = value($0); next }
	/^[[:space:]]*args[[:space:]]*=/ {
		sub(/^[^=]*=[[:space:]]*/, "")
		args = $0
		next
	}
	/^[[:space:]]*pass_filenames[[:space:]]*=/ {
		sub(/^[^=]*=[[:space:]]*/, "")
		sub(/[[:space:]]*(#.*)?$/, "")
		pass = $0
		next
	}

	END {
		flush()
		exit failed ? 1 : 0
	}
' "${CONFIG}"

echo "check-hook-cost: ${CONFIG} keeps the commit hook cheap"
