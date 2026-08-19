#!/usr/bin/env bash
#
# Guards the property that makes a fast local loop safe to offer: `make quick`
# is a strict subset of `make check`, and the work it leaves out is exactly the
# set that was deliberately deferred.
#
# GitHub Actions is the authoritative gate, so `quick` is allowed to be smaller
# than `check`. What it must never become is *different* — a loop that checks
# something the full gate does not is a loop that can pass work the merge gate
# then rejects, and a check added to `check` alone silently stops being part of
# the routine loop. Both directions are asserted here:
#
#   * every target `quick` runs is also run by `check`; and
#   * every target `check` runs is also run by `quick`, except the deferred set
#     below, which is named here so that adding to it is a deliberate edit.
#
# The comparison uses `make -n`, which expands recursive sub-makes without
# running any of the underlying checks, so this costs a few milliseconds and
# needs no network.
#
# That comparison can only see a step invoked as `$(MAKE) <target>`: a check
# written as a bare shell command in a recipe, or hung off a goal as a
# prerequisite, would be invisible to it and would slip past the two rules
# above. So the structure the comparison depends on is asserted first — every
# step of a gate goal must be a `$(MAKE) <target>` line, and a goal may take a
# prerequisite only from the small allowlist below. Without that check this
# script would assert rather less than it appears to.
#
set -euo pipefail

ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"

# Deferred to `check` and to GitHub Actions. build-check rebuilds the UI bundle
# and compiles the published release target, which is the slowest check in the
# gate whenever the build cache is cold; vulncheck and ui-audit each need the
# network and a current advisory database; ui-browser-install downloads a browser,
# and ui-browser-test then drives it over the demo stack for minutes. Adding a
# target here removes it from the routine local loop, so it needs a reason.
DEFERRED=(
	build-check
	ui-audit
	ui-browser-install
	ui-browser-test
	vulncheck
)

# The ci-* targets are the grouping CI uses to name a failing area, not checks
# in their own right, so they are not part of the comparison.
GROUP_TARGETS=(
	ci-lint
	ci-security
	ci-test
	ci-ui
)

# The goals whose structure is constrained: the two entry points and the groups
# they delegate to. Every step of each must be a `$(MAKE) <target>` line.
GATE_GOALS=(
	check
	ci-lint
	ci-security
	ci-test
	ci-ui
	quick
)

# The only prerequisites a gate goal may carry. Both install the browser UI
# dependency tree, which is a precondition of running the UI checks rather than
# a check that could go unnoticed.
ALLOWED_PREREQS=(
	ui-ensure
	ui-install
)

# Asserts the structure the target comparison relies on. Reads the Makefile
# rather than `make -n`, because it is the written form that has to hold.
check_structure() {
	awk -v goals="${GATE_GOALS[*]}" -v allowed="${ALLOWED_PREREQS[*]}" '
		BEGIN {
			split(goals, g, " ")
			for (i in g) want[g[i]] = 1
			split(allowed, a, " ")
			for (i in a) ok_prereq[a[i]] = 1
		}
		/^[a-zA-Z0-9_.-]+:/ {
			name = $0
			sub(/:.*/, "", name)
			rest = $0
			sub(/^[^:]*:/, "", rest)
			current = (name in want) ? name : ""
			if (current != "") {
				n = split(rest, prereqs, " ")
				for (i = 1; i <= n; i++)
					if (!(prereqs[i] in ok_prereq))
						print "prerequisite	" current "	" prereqs[i]
			}
			next
		}
		/^	/ {
			if (current == "")
				next
			line = $0
			sub(/^	/, "", line)
			if (line !~ /^\$\(MAKE\) [a-z0-9-]+$/)
				print "step	" current "	" line
			next
		}
		{ current = "" }
	' "${ROOT}/Makefile"
}

# Prints the sorted set of targets a goal runs, one per line. `make -n` prints a
# recursive invocation as "<make> <target>"; the sub-make it spawns inherits -n,
# so nothing is executed.
targets() {
	(cd "${ROOT}" && make -n "$1") |
		sed -n 's|^[^ ]*/\{0,1\}make\{1,\} \([a-z0-9-]\{1,\}\)$|\1|p' |
		grep -vxF -f <(printf '%s\n' "${GROUP_TARGETS[@]}") |
		sort -u
}

# Indents a block for a diagnostic, one level, preserving its line structure.
indent() {
	while IFS= read -r line; do
		printf '  %s\n' "${line}"
	done
}

quick="$(targets quick)"
check="$(targets check)"

if [[ -z "${quick}" || -z "${check}" ]]; then
	echo "check-local-gate: could not read the targets of quick or check" >&2
	exit 1
fi

status=0

# Rule zero: the structure the two comparisons below can actually see.
violations="$(check_structure)"
if [[ -n "${violations}" ]]; then
	echo "check-local-gate: a gate goal has a step the subset check cannot see:" >&2
	while IFS=$'\t' read -r kind goal detail; do
		case "${kind}" in
		prerequisite)
			echo "  ${goal}: prerequisite '${detail}' is not an allowed one" >&2
			;;
		step)
			echo "  ${goal}: step '${detail}' is not a \$(MAKE) <target> line" >&2
			;;
		esac
	done <<<"${violations}"
	echo "  Every gate step must be its own target, invoked as \$(MAKE) <target>," >&2
	echo "  or the subset comparison below silently ignores it." >&2
	status=1
fi

# Direction one: nothing is in the routine loop that the full gate skips.
extra="$(comm -23 <(echo "${quick}") <(echo "${check}") || true)"
if [[ -n "${extra}" ]]; then
	echo "check-local-gate: 'quick' runs targets that 'check' does not:" >&2
	echo "${extra}" | indent >&2
	echo "  Add them to 'check' as well, or drop them from 'quick'." >&2
	status=1
fi

# Direction two: what the full gate runs and the routine loop skips is exactly
# the deferred set — no more, and no less.
missing="$(comm -13 <(echo "${quick}") <(echo "${check}") || true)"
expected="$(printf '%s\n' "${DEFERRED[@]}" | sort -u)"

undeclared="$(comm -23 <(echo "${missing}") <(echo "${expected}") || true)"
if [[ -n "${undeclared}" ]]; then
	echo "check-local-gate: 'check' runs targets that 'quick' skips silently:" >&2
	echo "${undeclared}" | indent >&2
	echo "  Add them to 'quick', or defer them deliberately in DEFERRED below." >&2
	status=1
fi

stale="$(comm -23 <(echo "${expected}") <(echo "${missing}") || true)"
if [[ -n "${stale}" ]]; then
	echo "check-local-gate: DEFERRED names targets 'check' no longer runs:" >&2
	echo "${stale}" | indent >&2
	echo "  Remove them from DEFERRED in $(basename "${BASH_SOURCE[0]}")." >&2
	status=1
fi

if [[ ${status} -eq 0 ]]; then
	echo "check-local-gate: 'quick' is a strict subset of 'check', deferring: ${DEFERRED[*]}"
fi

exit "${status}"
