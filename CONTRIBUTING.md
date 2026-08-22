# Contributing

Domestique is a private single-tenant service. The accepted contracts in
[`docs/specs`](docs/specs) define its behavior; a change must not silently
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

`mise run quick` runs everything the full gate runs except six checks it defers
— `build-check`, `test-race`, `vulncheck`, `ui-audit`, `ui-browser-install` and
`ui-browser-test` — so it stays worth running on every iteration and is not
itself a full gate. Run the full gate yourself with `mise run check` when a
change implicates one of the six: the release build, concurrent code, a
dependency, or the browser suite.
[The delivery specification](docs/specs/delivery.md#the-authoritative-gate-is-github-actions)
says why each is deferred, and how the difference is asserted rather than only
documented.

`mise run test-race` is the one to reach for after touching anything concurrent
— the sync service and its reporter, the Wahoo client, the Access verifier, or
the composition root — because a data race there surfaces as a corrupted report
or a wedged run rather than as a failing test. It is the one command here that
needs cgo, so it needs a C compiler and takes several times as long as
`mise run test`.

Either loop skips a check whose files have not moved since it last passed, and
says `sources up-to-date, skipping` where it would have run one.
`mise run --force <task>` runs it anyway, and CI always does — the gate a merge
must satisfy never reads that cache.

Neither loop starts the production image. `mise run container-smoke` does: it
runs the image under the hardening
[`docs/compose.example.yml`](docs/compose.example.yml) documents and asserts the
runtime contract, so an image that builds but cannot come up fails here rather
than at deploy time. It takes an image rather than building one — a local image
build needs a `docker login dhi.io` for the hardened base images — so set
`DOMESTIQUE_SMOKE_IMAGE` to a reference in your local image store, or build
`domestique:smoke` first. CI runs it on every pull request that changes a
container input, and
[the delivery specification](docs/specs/delivery.md#proving-the-runtime-contract)
lists everything it asserts.

Install the optional local Git hook with:

~~~sh
mise exec -- prek install
~~~

The installed hook hardcodes the path of the `prek` it was installed with, so
run that again after a toolchain bump moves the pinned version; otherwise the
hook silently falls back to whatever `prek` is on your `PATH`.

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

Neither `quick` nor `check` measures coverage. `mise run coverage` is what you
run to read a change with its untested parts visible.

~~~sh
mise run coverage
~~~

That writes a Go coverage profile to `.coverage/go.out` and the browser UI's
LCOV report to `.coverage/ui/lcov.info`, and prints a summary for each. Nothing
is committed — `/.coverage/` is gitignored. `mise run coverage-go` and
`mise run coverage-ui` run one side alone. Both are unit suites, so the pair
takes seconds and needs no browser.

The Go profile is collected with `-coverpkg` over `./cmd/...` and
`./internal/...`, which makes the percentage `go test` prints on each of its own
lines meaningless: that one is the fraction of the whole service one package's
tests reached. Read the per-package summary printed after the run, which
`dev/coveragesummary` produces from the merged profile.

**On a pull request, one status decides the merge.** `codecov/patch/go`
requires the lines your change adds or alters in Go to be covered at least as
well as the base commit's already are. There is no fixed percentage to clear,
deleting well-covered code cannot fail it, and a change with no measurable Go
passes it. The other three statuses — `codecov/patch/ui` and the two project
totals — report and block nothing; the UI one cannot judge because the number
is Vitest alone and the browser suite's reach does not enter it.
[The delivery specification](docs/specs/delivery.md#coverage) states what is
measured, what is deliberately not, and why the statuses are shaped this way.

Ask that question locally before you push, rather than spending a CI run on it:

~~~sh
mise run patch-coverage
~~~

That measures both languages, then grades the lines your change adds against the
same rule Codecov applies, comparing against the merge base with `main` and
reading your working tree rather than only what you have committed. It prints
each uncovered added line as `file:line`. A Go shortfall fails the task; the UI
half only reports, as its status does. The UI number reproduces Codecov's
exactly; the Go one is an estimate, because a profile describes blocks rather
than lines, and reads a couple of tenths of a point low. Treat a Go verdict
inside that margin as worth a second look rather than as settled.

One edge worth knowing. A branch forked from before coverage was published has a
base commit with no report, so the patch status has nothing to compare against;
and if that branch's tree also predates the coverage job, it uploads nothing
itself, so the required contexts never arrive and the pull request blocks on
their absence rather than on a number. Rebasing onto current `main` answers
both, and is worth doing before opening the pull request rather than after.

### When a change cannot clear a coverage gate

Some changes legitimately cannot, and the gate is not a claim otherwise:
generated code, a package only a live provider can exercise, a component whose
behaviour is a browser's rather than this UI's, a fix that has to ship before
the test that pins it. What the gate asks is that this be a decision somebody
made and wrote down.

The escape hatch is the administrator pull-request bypass the branch ruleset
grants, which GitHub records on the pull request. Use that rather than switching
a status off for an afternoon, and write the reason in the pull request body:
what is uncovered, why a test is not the answer here, and what would have to
change for it to be. The mechanism cannot ask why, so an override nobody can
read is indistinguishable from a gate that never fired.

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

### When a browser test fails

The browser suite is the one whose failures are expensive to reproduce — a
Chromium download and a demo stack — so it is set up to explain itself from a
single run, and CI keeps what it wrote.

On a runner the suite reports through Playwright's `github` reporter, which
annotates the pull request's Files-changed view at the file and line the
assertion failed on. Locally it reports through `list`, unchanged.

Everything else it leaves goes under the gitignored `.playwright/` directory: a
trace per failed test under `.playwright/results/`, the failure screenshot
beside it, and the HTML report under `.playwright/report/`. The job that drives
the browser uploads that directory as an artifact when it fails, as
`playwright-ui`, kept for seven days — long enough to look at a failure, not
long enough to accumulate. A green run uploads nothing.

Open a downloaded trace at [trace.playwright.dev](https://trace.playwright.dev),
which runs in the browser and uploads nothing, or with
`npx playwright show-trace <file>`. It carries the DOM, the console, the network
and a screenshot per step, which is most of what re-running the test locally
would have told you. The library behind it is the synthetic one in
`internal/demo`, so a trace holds no real route.

## Contributions

Keep changes focused, add regression tests for behavior changes, and avoid
network access in normal tests. Do not commit credentials, tokens, static
configuration, private route data, generated FIT files, SQLite state, or host
deployment files.

Use Conventional Commits. Pull requests should explain any changes to the
normative specifications, safety gates, or operator action required for
deployment. A change to what an operator sees or does when a run stops belongs
in [the operator recovery runbook](docs/runbook.md) in the same change: it is
written against the failure categories the service emits, and a category added
without an entry there fails `mise run test`.

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
