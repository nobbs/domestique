# Contributing

Domestique is a private single-tenant service. The accepted contracts in
[`docs/specs`](docs/specs) define v1 behavior; a change must not silently
weaken their access, deletion, or secret-handling rules.

## Local checks

GitHub Actions is the authoritative gate: it runs the complete validation for
every changed path on every pull request, and its aggregate check is what a
merge must satisfy. Local checks exist to give you that answer sooner.

Install the project-local tools and run the routine loop:

~~~sh
mise install
mise exec -- make quick
~~~

`make quick` runs everything the full gate runs except five checks it defers —
`build-check`, which cross-compiles both published architectures; `vulncheck` and
`ui-audit`, which need the network and a current advisory database; and
`ui-browser-install` and `ui-browser-test`, which download a browser and then
drive it over the demo stack for minutes. That difference is asserted, not just
documented, so a check added to the gate cannot quietly drop out of the routine
loop.

Run the full gate yourself with:

~~~sh
mise exec -- make check
~~~

Install the optional local Git hook with:

~~~sh
mise exec -- prek install
~~~

`make fmt` applies Go formatting. The Git hook may also make safe whitespace
repairs and exits non-zero so they can be reviewed and staged deliberately.

The hook judges a commit on the files it stages: Go formatting and Markdown
lint see the staged files alone, so a commit never fails on a defect in a file
it did not touch. It deliberately runs no tests, no full linting, no audit, no
cross-compilation, no image build, and no browser suite — that work belongs to
`make check` and to GitHub Actions, and `make hook-check` fails if it appears
in `prek.toml`. Keeping the hook to roughly a second is what keeps it worth
leaving installed.

## Coverage

Nothing local fails on a percentage: neither `quick` nor `check` measures
coverage, and `make coverage` is something you run to read a change with its
untested parts visible. On a pull request, one number is a merge condition.

CI measures both languages on every pull request and every push — including a
documentation-only one — and publishes them to Codecov under two flags, `go` and
`ui`, so that a shortfall names the language it came from.

**The Go lines a pull request adds or changes must be covered at least as well
as the base commit's Go already is.** That is `codecov/patch/go`: required by
the branch ruleset, and no longer informational. There is no fixed percentage to
clear, because the bar is whatever the tree already manages — which also means
it rises as coverage improves. It reads only the diff, so deleting well-covered
code or moving statements between packages cannot fail a pull request. A 1%
threshold absorbs rounding, and a change with no measurable Go in it reports
"Coverage not affected" and passes.

One edge worth knowing: if the base commit carries no coverage report at all —
a branch forked from before coverage was published — there is nothing to compare
against and the status passes rather than blocking. Rebasing onto current `main`
is what makes it measure.

`codecov/patch/ui` is required too, but stays informational: it has to arrive,
and its number never fails. The UI measurement does not yet describe what the
browser suite reaches — the Playwright run drives the map and the page-level
components in real Chromium, but its coverage never enters the LCOV report
uploaded here, so those components still read as untested. An enforced target on
`ui` would fail every pull request that touches them, which would make the
override below the normal route rather than the exception. Fixing that
measurement is tracked as its own work, and flipping `ui` belongs to it. Both
project statuses report trend and block nothing.

Because both contexts are required to arrive, the coverage job carries no path
filter: a commit that uploaded nothing would leave them never posted and the
pull request blocked with nothing to un-block it. The trade that comes with it:
while Codecov is down, or for a pull request from a fork that has no upload
token, the contexts do not arrive and the merge waits. Removing them from the
ruleset is the escape hatch, and it is a repository settings change rather than
a code one.

Locally:

~~~sh
mise exec -- make coverage
~~~

That writes a Go coverage profile to `.coverage/go.out` and the browser UI's
LCOV report to `.coverage/ui/lcov.info`, and prints a summary for each. Nothing
is committed — `/.coverage/` is gitignored. `make coverage-go` and
`make coverage-ui` run one side alone.

The Go profile is collected with `-coverpkg` over `./cmd/...` and
`./internal/...`, so a function exercised only through another package's tests
counts as covered rather than dead. The same flag makes the percentage `go test`
prints on each of its own lines meaningless — that one is the fraction of the
whole service one package's tests reached — so read the per-package summary
printed after the run, which `dev/coveragesummary` produces from the merged
profile.

Four things are deliberately not measured:

| Not measured | Why |
| --- | --- |
| `dev/` | Repository tooling rather than the service. It has its own tests, and they run in the normal suite. |
| `src/**/*.test.{ts,tsx}` and `src/test/` | The tests and their harness are the measurement, not its subject. |
| `src/**/*.d.ts` | Type-only declarations emit no runtime statement to reach. |
| `src/main.tsx` | It mounts React onto a real document and does nothing else, so a test of it would assert the framework rather than this UI. |

Everything else is measured, including code that no unit test can reach today.
The map components need WebGL, and the page-level components are assembled
rather than unit-tested, so both report low — which is the honest answer.
Browser-level coverage for them is tracked as its own work; excluding them would
hide a real gap behind a comfortable number.

### When a change cannot clear the Go gate

Some changes legitimately cannot, and the gate is not a claim otherwise:
generated code, a package only a live provider can exercise, a fix that has to
ship before the test that pins it. What the gate asks is that this be a decision
somebody made and wrote down.

The branch ruleset grants the Admin repository role a `pull_request` bypass. It
permits merging a pull request whose required checks have not passed, and
nothing else — direct push, force-push, and branch deletion stay closed. GitHub
records the override on the pull request, which is the reason to use that
mechanism rather than switching the status off for an afternoon.

The mechanism cannot ask why, and a ruleset cannot make a required check
conditional on a label, so any label-driven exception would be decoration. Write
the reason in the pull request body instead: what is uncovered, why a test is
not the answer here, and what would have to change for it to be. An override
nobody can read is indistinguishable from a gate that never fired.

The verdict comes from Codecov rather than from the workflow. The `Required` job
deliberately does not judge coverage itself, so a failure names the app that
measured it and CI stays green on its own terms.

## Contributions

Keep changes focused, add regression tests for behavior changes, and avoid
network access in normal tests. Do not commit credentials, tokens, static
configuration, private route data, generated FIT files, SQLite state, or
deployment files from the Raspberry Pi.

Use Conventional Commits. Pull requests should explain any changes to the
normative specifications, safety gates, or operator action required for
deployment.
