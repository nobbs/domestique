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
mise run quick
~~~

Mise pins the toolchain and defines every command as a task, so it is required
rather than convenient: there is no Makefile and no other entry point. Install
it from [mise.jdx.dev](https://mise.jdx.dev) first; `mise tasks` then lists
everything this repository offers.

`mise run quick` runs everything the full gate runs except five checks it defers
— `build-check`, which compiles the published release target; `vulncheck` and
`ui-audit`, which need the network and a current advisory database; and
`ui-browser-install` and `ui-browser-test`, which download a browser and then
drive it over the demo stack for minutes. That difference is asserted, not just
documented, so a check added to the gate cannot quietly drop out of the routine
loop.

Run the full gate yourself with:

~~~sh
mise run check
~~~

Either loop skips a check whose files have not moved since it last passed, and
says `sources up-to-date, skipping` where it would have run one.
`mise run --force <task>` runs it anyway, and CI always does — the gate a merge
must satisfy never reads that cache.

Neither loop starts the production image. `mise run container-smoke` does: it
runs the image under the hardening the deployment example documents and asserts
that the service comes up, both probes answer, an anonymous caller is refused,
the process is unprivileged, and nothing was written outside the state mount. It
takes an image rather than building one — a local image build needs a
`docker login dhi.io` for the hardened base images — so set
`DOMESTIQUE_SMOKE_IMAGE` to a reference in your local image store, or build
`domestique:smoke` first. CI runs it on every pull request that changes a
container input.

Install the optional local Git hook with:

~~~sh
mise exec -- prek install
~~~

`mise run fmt` applies Go formatting. The Git hook may also make safe whitespace
repairs and exits non-zero so they can be reviewed and staged deliberately.

The hook judges a commit on the files it stages: Go formatting and Markdown lint
see the staged files alone, so a commit never fails on a defect in a file it did
not touch. It deliberately runs no tests, no full linting, no audit, no
cross-compilation, no image build, and no browser suite — that work belongs to
`mise run check` and to GitHub Actions, and `mise run hook-check` fails if it
appears in `prek.toml`. Keeping the hook to roughly a second is what keeps it
worth leaving installed.

## Coverage

Nothing local fails on a percentage: neither `quick` nor `check` measures
coverage, and `mise run coverage` is something you run to read a change with its
untested parts visible. On a pull request, one number is a merge condition.

CI measures both languages on every pull request and every push — including a
documentation-only one — and publishes them to Codecov under two flags, `go` and
`ui`, so that a shortfall names the language it came from.

**The lines a pull request adds or changes must be covered at least as well as
the base commit's already are, in each language separately.** That is
`codecov/patch/go` and `codecov/patch/ui`: both required by the branch ruleset,
and neither informational. There is no fixed percentage to clear, because the
bar is whatever the tree already manages — which also means it rises as coverage
improves. A status reads only the diff, so deleting well-covered code or moving
statements between packages cannot fail a pull request. A 1% threshold absorbs
rounding, and a change with no measurable Go, or no measurable UI, reports
"Coverage not affected" for that flag and passes it.

One edge worth knowing. A branch forked from before coverage was published has a
base commit with no report, so the patch status has nothing to compare against;
and if that branch's tree also predates the coverage job, it uploads nothing
itself, so the required contexts never arrive and the pull request blocks on
their absence rather than on a number. Codecov is expected to pass a patch it
cannot compare, but that has not been observed here and nothing depends on it.
Rebasing onto current `main` answers both, and is worth doing before opening the
pull request rather than after.

`codecov/patch/ui` was informational for a while, and is not any more. It could
not judge while the UI measurement described only what jsdom reaches: the
Playwright run drives the map and the page-level components in real Chromium,
but none of that entered the uploaded report, so a pull request touching them
would have failed a status for code it had in fact exercised. The report now
includes the browser suite, so the number the status reads is the one the tree
deserves. Both project statuses report trend and block nothing.

Because both contexts are required to arrive, the coverage job carries no path
filter: a commit that uploaded nothing would leave them never posted and the
pull request blocked with nothing to un-block it. The trade that comes with it:
while Codecov is down, or for a pull request from a fork that has no upload
token, the contexts do not arrive and the merge waits. Removing them from the
ruleset is the escape hatch, and it is a repository settings change rather than
a code one.

That same job is where the browser half of the UI number is collected, rather
than the path-filtered `UI` job that runs the same suite for its own sake. A
flag assembled by a filtered job would mean one thing on the commits that job
ran on and another on the commits it skipped, and `patch/ui` compares two
commits. So a change touching no UI still drives a browser, which is most of why
that job takes as long as it does.

Locally:

~~~sh
mise run coverage
~~~

That writes a Go coverage profile to `.coverage/go.out` and the browser UI's
LCOV report to `.coverage/ui/lcov.info`, and prints a summary for each. Nothing
is committed — `/.coverage/` is gitignored. `mise run coverage-go` and
`mise run coverage-ui` run one side alone.

`mise run coverage-ui` is the slow one, because after the unit suites it runs
the whole-page suite in a real browser and merges what that reached into the
same report. If no browser is installed it says so, keeps the unit half, and
still succeeds — run `mise run ui-browser-install` once if you want the whole
number locally.

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
| `src/main.tsx` | It mounts React onto a real document and does nothing else, so a test of it would assert the framework rather than this UI. The browser does load it, and the merge drops it there too, so that the total does not depend on which collector ran. |

Everything else is measured, and code no unit test can reach is measured by the
test that can reach it. The map components need WebGL and the page-level
components are assembled rather than unit-tested, so both are read from the
whole-page suite: `scripts/coverage.ts` runs the `dev-server` Playwright
project with V8 coverage recording, maps each module the dev server served back
through its source map to the file under `src/` it came from, and merges that
with the Vitest report before the LCOV file is written. Both halves are V8
coverage over the same files, so a statement both suites reach counts once.

The `bundle` project is not measured. It drives the minified production bundle,
which ships without a source map, so nothing it records could be attributed back
to `src/` — and it exercises the same UI, so measuring it would pay twice for
the same answer.

The Vitest report is the subject on both axes: which files are measured, and
which statements each of them has. The files come from its own list rather than
from globs restated somewhere else, and the browser's counts are carried onto
its statement maps by source location. So the merge can only move the numerator.
The total is the same tree either way, the two halves cannot drift apart about
what they are measuring, and neither can drift from the `include` and `exclude`
in `vite.config.ts`.

### When a change cannot clear a coverage gate

Some changes legitimately cannot, and the gate is not a claim otherwise:
generated code, a package only a live provider can exercise, a component whose
behaviour is a browser's rather than this UI's, a fix that has to ship before
the test that pins it. What the gate asks is that this be a decision somebody
made and wrote down.

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

## Test results

Every suite writes a JUnit report as it runs, under `/.test-results/`, which is
gitignored:

| Suite | Report |
| --- | --- |
| `mise run test` | `.test-results/go.xml` |
| `mise run ui-test` | `.test-results/ui/vitest.xml` |
| `mise run ui-browser-test` | `.test-results/ui/browser.xml` |

Nothing local reads them — the terminal output is still what a local run is for.
They exist so that CI can upload the same files to Codecov's test analytics, from
the `Test` and `UI` jobs, under the same `go` and `ui` flags the coverage reports
use.

That upload is what puts a failure on the pull request. The Codecov comment names
the tests that failed, with their output, instead of leaving them in a folded
Actions log; and because Codecov keeps the results of past runs, it also marks a
test as **flaky** when it has both passed and failed on `main` without the code
under it changing. A flake marked there is a test to fix or delete rather than
one to re-run: the browser suite retries nothing, deliberately, so a test that is
not reproducible reports nothing.

No test result gates a merge. Nothing compares them against a base commit, no
required status reads them, and a pull request that skipped a path-filtered job
simply has no report for that suite on that commit. A failed upload — no token on
a fork, or Codecov unavailable — costs the report and not the run.

## Contributions

Keep changes focused, add regression tests for behavior changes, and avoid
network access in normal tests. Do not commit credentials, tokens, static
configuration, private route data, generated FIT files, SQLite state, or
deployment files from the Raspberry Pi.

Use Conventional Commits. Pull requests should explain any changes to the
normative specifications, safety gates, or operator action required for
deployment.

## Dependency updates

Renovate proposes dependency updates as they are released, at most two an hour
and five open at once; `renovate.json5` holds the configuration and says next to
each rule why it is what it is. Most
non-major updates automerge through GitHub's auto-merge once the required checks
pass, so the gate is what makes them safe — see
[the delivery specification](docs/specs/delivery.md#dependency-updates) for what
that covers and what it deliberately does not.

Three things want a person: any major, anything touching `maplibre-gl` or
`react-map-gl`, and anything opened for a published advisory. The map packages
are there because the browser suite renders the map without judging it, which is
the same reason a map change of your own needs a look before it merges.

The `dhi.io` base images are covered, and their digests automerge — that is what
a hardened base image is for, and a Dockerfile change is built and smoke-tested
before it merges. A tag change comes grouped with the `.mise.toml` and `go.mod`
entries it has to agree with, so a new Node or Go arrives as one pull request
rather than three.
