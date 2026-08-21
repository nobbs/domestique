# Domestique delivery specification

**Status:** accepted

This is a subordinate specification to [the service contract](service.md). It
defines the local quality gate, GitHub Actions, container hardening, the images
it publishes, and how one reaches a host. It places no Tailnet-host
configuration or secret in this repository: the one host-side file it does carry
is a non-secret deployment script that reads every host-specific value from the
environment it runs in.

## Development toolchain and commands

The repository tracks a `.mise.toml` that pins the Go toolchain, Node.js, pnpm,
`golangci-lint`, `prek`, Actionlint, Gitleaks, Markdownlint, ShellCheck, and
`govulncheck`. Mise is the source of truth for the versions used by a
developer and GitHub Actions. Bootstrap selects supported, mutually compatible
versions and records them there; no developer-global tool configuration is
required.

Mise is also the task runner, and the only entry point the repository offers.
The tasks live in a `mise-tasks.toml` that `.mise.toml` includes, so the pins
and the commands stay separately readable. There is no second, runner-free way
in: a contributor without mise cannot build, test, or check this repository at
all. That is the accepted cost of keeping one definition of each command rather
than two that drift apart, and installing mise is a single command that the
contributor guide names.

The repository provides these stable tasks:

| Task | Contract |
| --- | --- |
| `mise run fmt` | Applies Go formatting to owned Go source. |
| `mise run lint` | Runs the configured `golangci-lint` checks. |
| `mise run test` | Runs the normal deterministic test suite with `CGO_ENABLED=0` and writes its JUnit report to a gitignored directory. |
| `mise run test-race` | Runs the same suite under the race detector with `CGO_ENABLED=1` and an explicit `-timeout`. Instruments a test binary only; nothing published is built with cgo. |
| `mise run build` | Compiles the Linux service binary for `BUILD_ARCH`, the machine's own architecture unless overridden, with `CGO_ENABLED=0`, after building the browser UI. |
| `mise run ui-build` | Builds the browser UI bundle that the binary embeds. |
| `mise run ui-dev` | Runs the UI dev server, proxying the API to the local service. |
| `mise run dev-setup` | Snapshots the deployed state into an isolated development environment. |
| `mise run dev-api` | Serves the API against that snapshot on `:8081`. |
| `mise run container-smoke` | Starts the production image under the documented deployment runtime and asserts the runtime contract. Takes an image; builds none. |
| `mise run quick` | Runs the routine local loop: every check in `mise run check` except the six it defers. |
| `mise run check` | Runs the full gate locally, on demand. |
| `mise run coverage` | Writes a Go coverage profile and the browser UI's LCOV report to a gitignored directory and summarises both. |

`mise run check` is the full gate. It includes `prek run --all-files`, linting,
tests, the same tests under the race detector, TypeScript type checking, the
browser UI lint and test suites, the
browser suite, Go module verification, vulnerability analysis for both Go and
npm dependencies, a GitHub Actions workflow check, a shell-script check, a
worktree secret scan, a commit-hook cost check, a task-definition check, a
local-gate structure check, and the release-target binary compilation for the
published architecture. `mise run fmt` applies Go formatting. A fixing `prek`
hook exits non-zero after a safe mechanical repair so the resulting change can
be reviewed and staged deliberately.

### The authoritative gate is GitHub Actions

GitHub Actions runs the complete validation on every pull request targeting the
default branch, and its aggregate check is what a merge must satisfy. The gate
is paid there rather than locally; the routine local loop is fast because of
where the gate is paid, not because any check is removed, relaxed, or made
optional.

Local validation is therefore a way to learn a result earlier, and it comes at
two depths:

- `mise run quick` is the routine loop, and the expected gate before a
  hand-over. It runs everything the full gate runs except the six checks named
  below, so it stays worth running on every iteration.
- `mise run check` is the full gate on demand, unchanged. Reach for it when one
  of those six checks is specifically implicated by the change in hand, not as
  a routine step before pushing: each of them runs on every pull request anyway.

`mise run quick` defers exactly six checks, each because it is slow or needs
the network rather than because it matters less:

| Deferred check | Why |
| --- | --- |
| `build-check` | Rebuilds the UI bundle and compiles the published release target; the slowest check in the gate whenever the build cache is cold. |
| `test-race` | Reruns the whole Go suite under the race detector; needs cgo, which the rest of the repository does without, and costs several times the wall clock of the plain run. |
| `vulncheck` | Needs the network and a current Go advisory database. |
| `ui-audit` | Needs the network and a current npm advisory database. |
| `ui-browser-install` | Downloads a browser: a network fetch and a few hundred megabytes on disk. |
| `ui-browser-test` | Drives that browser over the demo stack; minutes rather than seconds, and useless without the download above. |

One task installs the browser UI dependency tree, and every check that reads
it waits for that task rather than installing anything itself. It reinstalls
from the lockfile when the lockfile has moved or the tree is gone, and does
nothing otherwise, so a dependency change is proved against a fresh tree
locally as well as in CI.

That `mise run quick` is a strict subset of `mise run check`, and that the
difference is exactly the deferred set above, is asserted rather than only
documented: a check added to the full gate fails the assertion until it is
either added to the routine loop or deferred deliberately. The routine loop may
therefore be narrower than the gate, but never different from it. The assertion
reads the declared task graph from `mise tasks ls --json`, which resolves every
dependency without running anything, so it costs milliseconds and needs no
network.

The assertion also constrains the form the gate is written in, because it can
only compare steps it can see: every step of a gate task is a task of its own,
named in `depends` or `depends_post`, and the gate task runs no command itself.
A check written inline on a gate task would pass the subset comparison while
escaping it, so the shape is checked first. Those two are the whole of
membership; `wait_for` orders tasks that already run and never schedules one, so
it cannot add a step.

The three depths compose: the commit hook judges the staged files in about a
second, `mise run quick` judges the working tree, and GitHub Actions judges the
merge. Each is a strict subset of the one after it.

`prek` owns fast repository hygiene: whitespace and end-of-file checks,
private-key and accidental-large-file checks, YAML, TOML, and Markdown
validation where applicable, and Go formatting. Developers may install the
`prek` hook, but the hook is a convenience rather than a substitute for
`mise run check`. The project uses `prek`, never `pre-commit`.

The installed hook is bounded by what it may do, because a hook slow enough to
be bypassed protects nothing. A hook that runs a command takes its file list
from `prek`, so a commit is judged on what it stages and not on the rest of the
tree; the same configuration under `prek run --all-files` covers the repository,
which is how the gate keeps its reach. Tests, full linting, audits,
cross-compilation, image work, and the browser suites stay out of the hook and
belong to `mise run check` and GitHub Actions. Both properties are asserted, so
a later hook cannot quietly reintroduce the cost; wall-clock is not asserted,
because a timing assertion on a shared runner is flaky.

### A local check may skip work its inputs have not changed

Most checks declare the files they read, and mise skips one whose inputs have
not moved since it last succeeded, saying so in place of the run. That is a
local convenience only: **CI runs every task with `--force`, so the
authoritative gate never consults a cache.** `mise run --force <task>` does the
same locally.

Three rules decide which checks may be skipped, and they are narrower than
"whatever is slow":

- A check qualifies only when its verdict is a function of the files it names.
  `vulncheck` and `ui-audit` read an advisory database that moves without the
  tree, so an unchanged tree can still be newly vulnerable and they always run.
- Its inputs must be nameable. `hygiene` and `secret-scan` read the whole
  worktree, and a source list that broad is likelier to be wrong than those runs
  are to be slow.
- A glob names a kind of file rather than the directories that hold it today —
  `**/*.go`, not a list of packages — because a source list that misses a new
  file is a check that stops noticing it.

Four properties are what make skipping safe rather than merely fast. A task that
failed is never recorded as up to date, so a red check cannot be cached green.
Editing a task's own definition invalidates it, so a check cannot change what it
does and stay up to date. A glob matching no file would otherwise be up to date
forever, which is the one way this could retire a check silently, so
`gate-check` fails on one. And the mechanism only ever removes work from a local
run: the merge gate does not use it.

## Coverage

Coverage is measured on demand locally, where it reports rather than judges:
`mise run coverage` is not part of `mise run check`, and no local check fails on
a percentage. On a pull request one number is a merge condition, stated below.

The Go profile is collected across the service as a whole rather than per
package under test, so that a function exercised only through another package's
tests is not reported as dead. Four things are deliberately outside the measured
set, and nothing else is:

| Not measured | Why |
| --- | --- |
| `dev/` | Repository tooling rather than the service. It has its own tests, and they run in the normal suite. |
| `src/**/*.test.{ts,tsx}` and `src/test/` | The tests and their harness are the measurement, not its subject. |
| `src/**/*.d.ts` | Type-only declarations emit no runtime statement to reach. |
| `src/main.tsx` | It mounts React onto a real document and does nothing else, so a test of it would assert the framework rather than this UI. The browser does load it, and the merge drops it there too, so that the total does not depend on which collector ran. |

Code that only a browser-level test can reach is measured by a browser-level
test: the UI number is the unit suites and the whole-page suite merged. A
component the whole page exercises therefore reads as exercised rather than as a
gap the report cannot see. Both halves are V8 coverage over the same source
files, so a statement both reach counts once and the total stays a count of the
tree rather than of the collectors.

The measured half of the whole-page suite is the one that runs against the
development server, which serves every module with a source map back to the file
under `src/` it was built from. The half that runs against the embedded
production bundle is not measured: that bundle is minified and ships without a
source map, and measuring both would pay twice to learn the same thing.

The unit report is the subject of the merge on both axes: the files it contains,
and the statements it records for each of them. The browser's counts are carried
onto those, matched by source location, and what has no counterpart there is
dropped — so the merge moves the numerator and never the denominator, and the
two collectors cannot come to disagree about what is being measured. The result
is one report, merged before upload rather than uploaded as two, so that the
number a status compares is assembled the same way on every commit. Where no
browser is installed the unit half is still produced, and the run says what it
left out.

Both reports are written to one gitignored directory, so that the upload step
has a single place to look. No coverage report is committed, and none enters an
image context.

GitHub Actions measures both languages on every run it performs, under no path
filter, and publishes them to Codecov under separate flags, so that a shortfall
names the language it came from and the two are never averaged into a single
number. The browser half is collected there rather than handed over from the
path-filtered job that runs the same suite for its own sake, because a flag
assembled by a filtered job would mean one thing on the commits that job ran on
and another on the commits it skipped, and a patch status compares two commits.
The cost is that a change touching no UI still drives a browser.

Both per-flag patch statuses are required by the branch ruleset, and both judge.
Each must show that the lines a change adds or alters in its own language are
covered at least as well as the base commit's already are; beyond a rounding
threshold, a shortfall fails the check and the change does not merge. The bar is
the base rather than a chosen percentage, so no number has to be invented or
maintained and the requirement rises with the tree. A status reads only the
diff, so deleting well-covered code or moving statements between packages cannot
fail a change — the property a project-total ratchet cannot offer. A change
carrying nothing measurable in a language passes that language's status. A
change whose base commit carries no report is outside what this contract relies
on: it is rebased onto a default-branch commit that carries one, and nothing
here asserts what the status would otherwise report.

The UI status can judge because the measurement includes what the browser-level
suite reaches: a gate that failed changes to code that is in fact exercised
would have its ordinary outcome be an override. The two project statuses report
trend and block nothing.

The measurement carries no path filter because a required context has to arrive:
Codecov reports only on a commit it received a report for, so a run that
measured nothing would leave a required context that never arrives and a pull
request that nothing can un-block. The same gap seen from the other side is a
default-branch commit with no report, which is a commit no later pull request
can compare against.

A failed upload therefore costs a merge rather than a report: while Codecov is
unavailable, or for a pull request from a fork with no access to the upload
token, the contexts do not arrive and the merge waits. An enforced gate carries
a recorded exception: the branch ruleset grants the administrator role a
pull-request bypass, which permits merging a non-compliant pull request and
nothing else, and records on that pull request that it was used. Removing the
contexts from the ruleset remains the escape hatch for the arrival problem. Both
are repository settings changes rather than code ones.

The upload is constrained to the two files the run just produced. The uploader
does not search the working tree, and its plugins are disabled, because that
tree can hold a local database, configuration, and secret files, none of which
is ever an input to a coverage report. It carries no other credential.

Publishing requires one operator action that the repository cannot take for
itself: authorising Codecov for the repository and storing its upload token as
the `CODECOV_TOKEN` repository secret. Without it the coverage job still
measures both languages and still passes; only the upload is missing.

## Test results

Each suite writes a JUnit report of the run it just performed, to the same
gitignored directory as its siblings, and CI uploads those reports to the same
account the coverage reports go to, under the same two flags. The point is a
readable failure: a test that failed, and a test that fails intermittently on the
default branch, are named on the pull request rather than left in a folded log
that only the person who thinks to open it will read.

The reports are produced by the ordinary task, not by a CI-only invocation, so
the file a contributor can inspect locally is the file that was uploaded.

They are uploaded from the jobs that run the suites, which are path-filtered,
rather than from the unfiltered measurement job. Nothing compares a test report
against a base commit and no required status reads one, so a commit that ran no
suite for a language is a commit with nothing to report rather than one missing a
context that blocks it — which is the property that makes the filtered jobs the
right place, and is exactly what the coverage upload cannot rely on. A report
from those jobs also describes the suite as it normally runs rather than the
instrumented variant the measurement drives.

An upload is attempted even when the suite it describes failed, since that is the
run worth reporting. It is constrained the way the coverage uploads are: it names
its files, the uploader searches nothing, and a failed upload is reported without
failing the job. No test result decides a merge.

## Development environment

`mise run dev-setup` prepares an environment that shows real route data without
risking the deployment. It copies the deployed SQLite state into `.local/dev`
and writes a configuration beside it, so the development service and the
deployed container never share a database file. That configuration names the
deployed Cloudflare Access application, read out of the running container at
setup time, so the identity gate behaves there exactly as it does in
production.

That environment may read VeloPlanner, so a manual `POST /v1/sync` refreshes
real routes and geometry. It must never reach Wahoo, which is enforced in depth:
the state encryption key is a placeholder, so the stored refresh tokens cannot
be decrypted and a run therefore fails at the state step, which precedes any
Wahoo request; the Wahoo endpoints additionally point at an unroutable address;
and scheduled synchronisation is pushed a year out so a run only happens on
request. Pushover credentials are placeholders, so no notification is delivered.

`mise run demo` prepares the other kind of development environment: one that
needs no account, no snapshot, and nobody's routes. It writes a throwaway
configuration under `.local/demo`, seeds a fresh database with the synthetic
library in `internal/demo`, serves it with `dev/demoapi`, and runs the UI dev
server in front of it. Every stage, surface, run and target state it shows is
generated; VeloPlanner, Wahoo and Pushover all point at an unroutable address;
and no scheduler, source client or reporter is wired, so a manual
synchronisation re-seeds the synthetic library at the current instant instead of
contacting anything. The run it reports is therefore a real one and its data
still comes from nowhere.

The identity gate is not switched off there, because a demo whose gate is absent
demonstrates a service this repository does not ship. `dev/demoapi` generates a
signing key at start-up, publishes it to the production verifier through an
in-process key-set endpoint, and mints one assertion for the dev server to
present: the real signature, audience, expiry and allowed-email checks all run,
against a team that exists only inside that process.

Both of these apply to the development environment only. Neither is a substitute
for the sandbox acceptance check, and neither runs in CI.

## The browser suite

The browser UI is validated at two depths. The Vitest suites run in jsdom over
the reusable components and the API client, and are the routine cost. The
Playwright suite in `internal/webui/app/e2e` runs the whole page in a real
Chromium, and exists for what jsdom cannot observe: the map, which needs WebGL,
and the interactions that span components — scrubbing the elevation chart,
selecting a stretch off the map, following a card into a stage. It is not a
second home for logic that a component test could reach.

The suite runs against `mise run demo`, so it is subject to everything that
environment guarantees above: the synthetic library, the unroutable providers,
and the production identity gate in front of them. No test in it reads a real
route, and no personal data is involved in running it.

That one stack is driven as two projects. The `dev-server` project runs the specs
in `e2e` against the Vite dev server: the UI as it is being written, which is how
a change to it should be checked. The `bundle` project runs the specs in
`e2e/contract` against the Go service directly — the production bundle served by
`internal/webui`'s embed handler, the real routes behind it, and the cache
headers, content security policy and gates a deployment applies. It exists
because a handler test and a parser test can both keep passing while the JSON
they each assume has drifted apart: only the shipped client reading a real
response catches that, so a contract failure there is a real defect rather than a
stale fixture. Contract failures name the request they came back from, because a
mismatch that names neither the endpoint nor the field is a bug report nobody can
act on.

The bundle has to exist for that project to mean anything, and it is embedded at
compile time from a gitignored directory. `dev/demo.sh --with-bundle` builds the
UI before building `dev/demoapi`, which is how the suite guarantees the bundle
it drives is the current one; the flag is what `mise run ui-browser-test` starts
the stack with. Without it the demo still runs, and the listener says that no
bundle is embedded rather than serving a blank page.

There is no proxy in front of the service in that project, so two things the dev
server does on the way through are done by the harness instead: the identity
assertion is added to every request, and state-changing requests are forwarded
with the configured browser origin. `Origin` is browser-managed — Chromium keeps
the page's own origin whatever a test asks for — so those requests are made from
outside the browser and their answer handed back to it. That is the same hop the
dev proxy is, and the gate itself is untouched: the production verifier checks a
real signature, audience, expiry and address, and the origin check is asserted
directly by a request that presents the wrong one and is refused.

It is hermetic. The only third-party request the application makes is the basemap
style the service names, and the suite answers both the light and the dark
document from memory with a style that paints a background and causes MapLibre to
issue no further request: its one source carries an attribution and no features,
because the credit the page is obliged to show is read out of the style document
and a fixture declaring none could not exercise it. Every other cross-origin
request is refused and reported, and a test that let one out fails — so the suite
cannot quietly start depending on a provider being reachable.

No reference image is committed. A visual assertion compares the map region
against itself within the run, after waiting for two consecutive identical frames,
so it proves that an interaction changed what was painted without acquiring a
screenshot that goes stale on a renderer or font change. Everything about the
environment that a pixel depends on — viewport, device scale factor, colour
scheme, locale, time zone, motion and font stack — is pinned by the configuration
and the fixtures. Traces, failure screenshots and the HTML report are written to
the gitignored `.playwright/` directory, which both browser-driving CI jobs
upload as a short-lived artifact when they fail, and never when they pass. The
suite also reports through Playwright's `github` reporter on a runner, so a
failure annotates the pull request at the line it failed on rather than only in
the job log.

The browser and its system libraries are a download, so
`mise run ui-browser-install` is a separate target and both it and
`mise run ui-browser-test` are deferred out of the routine loop. They run in
`mise run check` and in the CI UI job, which is where they gate a merge. A
machine that cannot install a browser can still pass the routine loop; it cannot
claim the full gate.

CI installs the browser alone, without Playwright's `--with-deps`. The hosted
runner image already carries every shared library Chromium links, so on that
image the flag adds nothing but CJK, Thai and Cyrillic font packages: the
interface is Latin, and the suite's basemap names no glyph source, so nothing it
renders needs them. Nor can their absence move a comparison, because the
appearance checks compare a page against itself within one run rather than
against a stored image. `PLAYWRIGHT_INSTALL_FLAGS` remains the way to pass the
flag on a host that does need it, such as a bare container.

The suite renders the map but does not judge it. Looking at a map change remains
a human act, and a change handed over without one is reported as such.

Normal Go tests run without network access to VeloPlanner, Wahoo, Pushover,
Tailscale, or a secret system. The normal test command enables deterministic
test ordering variation with `-shuffle=on`.

The gate runs the whole Go suite a second time under the race detector, because
the two invocations answer different questions: shuffling catches tests coupled
through ordering, and the detector catches memory shared between goroutines
without synchronisation. A data race in the sync service does not
surface as a failing test but as a corrupted report or a wedged run, on a
schedule, where it is least reproducible.

The CGO-free contract for everything that ships is unaffected. The detector
needs cgo and is therefore the one command in this repository that runs with
`CGO_ENABLED=1`; it instruments a test binary that is never published. The
release build, the published image, and the container smoke test remain
`CGO_ENABLED=0` and statically linked, and the normal test command remains
CGO-free as well. The race check is deferred out of the routine local loop for
its cost, runs in the same CI job as the normal suite, and sets an explicit
`-timeout` below that job's own budget so that a hang is given up on by the
toolchain, which prints every goroutine's stack, rather than by the runner,
which prints nothing.

## Linting and static analysis

`.golangci.yml` is intentionally focused rather than an indiscriminate
all-linters configuration. It enables the standard Go checks plus rules that
protect this service's boundaries:

- `errcheck`, `govet`, `staticcheck`, and `unused` for correctness;
- `gosec` for insecure API use;
- `bodyclose`, `noctx`, and `rowserrcheck` for HTTP and database resource
  handling;
- `gocritic`, `revive`, `unconvert`, and `misspell` for maintainable Go;
- `testifylint` for correct Testify assertion use in tests; and
- the formatter supported by the pinned linter version.

Any future exclusion is narrow, adjacent to the code it covers, and explains
the invariant being preserved. The project does not lower the lint level to
accommodate generated or copied personal data; such data is not committed.

Module checks verify that `go.mod` and `go.sum` are tidy and valid.
Vulnerability analysis uses the Go vulnerability database through
`govulncheck`. A finding is triaged before a release; it is not silently
suppressed in CI.

## GitHub Actions

GitHub Actions runs for pull requests **targeting** the default branch and for
pushes **to** it, and for nothing else. There is no manual trigger: every run
answers for a specific tree that a review or a merge produced. It uses the same
pinned Mise toolchain and invokes the same `ci-*` task groups that
`mise run check` runs; CI does not reimplement a divergent list of shell
commands. It is the authoritative gate: it runs the complete validation for
every changed path, whatever a contributor chose to run locally.

The validation workflow must:

- use explicit minimal `permissions`; every job is read-only except the
  default-branch publish, which adds `packages: write`, and the deployment that
  follows it, which holds `id-token: write` alone. That token is a tailnet
  credential obtained by workload identity federation, not a signing identity:
  nothing is signed, and no long-lived tailnet secret is stored;
- pin third-party actions to immutable full commit SHAs;
- run without production credentials, Wahoo refresh tokens, or Docker secret
  files;
- fail when formatting, local hook hygiene, linting, tests, module checks,
  vulnerability analysis, or the CGO-free Linux build for the published
  architecture fail;
- build the production Dockerfile on a pull request that changes an input of the
  container build, never pushing it, and start that image once, so the runtime
  contract is answered by a container that ran rather than by a build that
  succeeded; and
- deploy what it published to the Tailnet host, over Tailscale SSH, passing the
  digest and nothing else, on every publish and not only on the commits that
  happened to touch every part of the tree: a published image the host is never
  given leaves the default branch ahead of what is running, silently and behind
  a green check; and
- aggregate every job into a single required check that is green only when each
  dependency succeeded or was skipped by a path filter, so a failed publish or
  deployment cannot be mistaken for a passing run. That check speaks only for
  the jobs this workflow runs. A verdict produced outside it — the coverage
  measurement and the secret scan — is required directly from the service that
  produced it, so the aggregate never asserts a result it did not compute; and
- record each job's result in that required check's run summary, because a
  skipped job and a job that never ran are indistinguishable on a commit status,
  and the gate is readable only if a skip is visibly a skip.

The pull-request build and the default-branch publish are the same build split
by event: a pull request proves it, a push to the default branch publishes it,
and neither runs the other's job, so a change is built once per event. A pull
request may read the shared registry build cache; it may not write to it,
because writing needs the same permission that publishes.

The two are gated by different path filters, and deliberately so. Publishing
follows every input of the running binary, because a source change must reach a
new digest. The pull-request build follows only the container inputs — the
Dockerfile, what it copies in, and the dependency manifests that resolve inside
a build stage — because re-proving an untouched Dockerfile against changed Go
source repeats what the compile check has already established. A container
break that reaches the default branch costs a red run and no new image; it costs
no availability, because a deploying host is pinned to a digest that keeps
running. The fix for such a break changes a container input by construction, so
it is proved before it merges.

The commit an image was built from is a build input, not something the build
looks up: the context excludes `.git`, so nothing inside it can work the revision
out. CI passes the commit it is running for as a `SOURCE_REVISION` build argument
and the Go link step compiles it in, which is what lets the running service name
its own source. The pull-request build passes it too, so the argument the
published build depends on is exercised by the build that proves it. A build
without it — every local one — reports no revision rather than a guessed one.

A separate code-scanning workflow analyses Go and GitHub Actions changes.
It also uses immutable action pins and least privilege.

Secret scanning runs against every pull request, and its check is required by
the branch ruleset: a scan that reports a finding, or that does not report at
all, holds the merge. That makes it a gate rather than only a report — but it
does not make it the control. The committed fixtures, logs, examples, and test
data must themselves contain no credentials or personal routes, because a
scanner is pattern matching over what was written and cannot be relied on to
notice a secret it has no pattern for. Repository-native secret scanning stays
enabled alongside it as defence in depth.

No GitHub Actions workflow invokes the live VeloPlanner account, authorises a
Wahoo account, uploads a route, or sends a Pushover notification. Sandbox FIT
and Wahoo acceptance is an explicit operator-run check with separately
provisioned non-production credentials, and its output must be redacted.

## Dependency updates

Dependencies are proposed by Renovate, configured in `renovate.json5` at the
repository root. Its job is to keep every pinned thing current: the npm tree,
the Go module graph, the toolchain in `.mise.toml`, the action commits in the
workflow, and the images the example compose files name.

Updates arrive as they are released rather than in a scheduled batch. What
bounds them is concurrency — at most two pull requests an hour and five open at
a time — because every one of them runs the browser suite and the coverage job.

The configuration is `config:best-practices` plus a short list of rules. The
preset supplies the mechanics — digests pinned for Docker and for actions,
devDependencies pinned to exact versions, a release-age hold on npm, warnings
for abandoned packages, weekly lock file maintenance — so each remaining rule
exists because of something true about this repository rather than about
dependency management in general, and there are few enough of them to read.

Most of those proposals merge without a human reading them. That is a deliberate
trade, and it rests entirely on the gate: an update is automerged through
**GitHub's own auto-merge**, never by Renovate deciding for itself that the
checks look fine. The merge therefore goes through the same mechanism a human
merge goes through and is subject to the same branch ruleset, so it cannot
happen while a required check is failing or absent. Turning auto-merge off at
the repository is sufficient to stop all of it, and the failure direction is
correct: Renovate opens the pull request and leaves it.

What is not automerged is what the gate cannot answer for:

- **Majors**, at any level of the tree. A major version is where a dependency is
  permitted to change its mind, which is what a passing suite is worst at
  noticing.
- **The map stack**, `maplibre-gl` and `react-map-gl`, at every update type. The
  browser suite renders the map but does not judge it, so a green run says
  nothing about whether the map still draws correctly, and looking at it remains
  a human act.
- **The Go and Node toolchains.** The compiler and runtime in `.mise.toml`, the
  language version in `go.mod`, the types in `@types/node` and the build stages'
  base image tags each describe one decision spread across several files, so
  each is grouped and moved by a person. It is what everything else is built
  with.
- **A published advisory.** These ignore the release-age hold and the
  concurrency limits because waiting is the greater risk, but they are read
  rather than merged: something is already known to be wrong.

Nothing is automerged until its release has been public for three days, and a
major until fourteen. This is not a quality judgement about the maintainers of
anything; it is that a compromised release is usually withdrawn well inside that
window and this repository has no reason to be an early consumer.

The base images are covered, and their digests automerge. A hardened base image
exists to be rebuilt against current system packages, so taking its next digest
promptly is the point of using one; and a pull request that changes the
Dockerfile builds the image and starts it under the container smoke test, which
makes this one of the better-proved updates the repository takes. Each `FROM`
therefore names its tag beside the digest. The digest is what resolves and the
tag changes nothing about which image is built on, but it records the stream the
digest came from, without which the next digest offered would be `latest`'s.

A tag change is offered too, but it does not arrive on its own: the Node and Go
base image tags are grouped with the toolchain entries they have to agree with,
so moving to a new compiler or runtime is one pull request that changes
`.mise.toml`, `go.mod` and the `Dockerfile` together, and a person reads it.

**An automerged update reaches production.** A merge to the default branch that
touches an input of the image publishes it, and the deployment follows the
publish; the `production` environment carries no approval gate. What stands
between a dependency update and the running service is the gate itself — the
full suite, the browser suite, the coverage statuses, the secret scan, and the
container smoke test that starts the built image and exercises its runtime
contract. Narrowing what automerges, or adding a required reviewer to that
environment, are the two places to intervene if that trade stops being worth it.

## Container contract

The production image is a multi-stage build that produces a statically linked
Linux binary with `CGO_ENABLED=0`, published for `linux/amd64` alone. That is
the architecture of the deployed host, and of the runner that builds it, so no
stage needs emulation. The accepted cost is that the published image does not
run on an arm64 host without it; the `TARGETOS`/`TARGETARCH` parameterisation in
the Dockerfile is kept, so restoring a second platform is a build argument at
each build site rather than a rewrite. A first stage builds the browser UI
bundle with Node.js, which the Go stage then embeds; Node reaches no further
than that stage and is absent from the runtime image.

Every base image is a **Docker Hardened Image** from `dhi.io`, pinned by digest:
the `-dev` variants for the Node and Go build stages, which need a shell and a
toolchain, and the minimal `static` image for the runtime. They carry SBOMs,
SLSA Build Level 3 provenance, and signatures. Because the images this project
publishes are themselves unsigned, that is the strongest verifiable link in the
chain, and it is why the base images stay pinned by digest. Each reference names
its tag beside that digest — `dhi.io/node:24-dev@sha256:…` — which resolves
identically and exists so that an automated digest refresh follows the intended
stream rather than `latest`.

`dhi.io` requires `docker login dhi.io` with a Docker Hub account and personal
access token **even on the free Community tier**. It is therefore a build-time
credential dependency: the validation and publish jobs both authenticate to it,
and a machine that builds images locally must be logged in. A deploying host is
unaffected: it pulls a published GHCR image by digest and never builds, so it
needs no `dhi.io` credential.

The runtime image:

- contains the binary and only the certificate roots and runtime files it
  genuinely needs;
- runs as an unprivileged non-root user;
- has a declared persistent volume for `/var/lib/domestique`, which holds the
  SQLite database;
- accepts secret files only at runtime under `/run/secrets`, never during the
  image build;
- has no bundled Tailscale daemon, SSH service, shell requirement, or default
  credentials; and
- is usable with a read-only root filesystem plus a temporary writable mount if
  the selected runtime needs one.

The host runs the container with a loopback-only publication such as
`127.0.0.1:8080:8080`, plus the readiness listener on the same terms
(`127.0.0.1:8081:8081`). Both are loopback-only; only the served one is given to
`tailscale serve`, which is what keeps the readiness probe available to
host-local health checking and unreachable from the authenticated public
surface. The host's Tailscale process owns `tailscale serve`
and the identity header boundary; Tailscale is not embedded in the application
container. The compose file, static configuration, Docker secret files, and
pinned image digest are operator-managed deployment state outside Git.

### Proving the runtime contract

A successful build says the image can be produced. It says nothing about whether
the service comes up inside it, which is what an operator finds out at deploy
time. `dev/container-smoke.sh`, run by `mise run container-smoke`, is the answer
to that: it starts the image with the runtime the compose example documents —
the image's own unprivileged user, a read-only root filesystem, no capabilities,
`no-new-privileges`, the documented `/tmp` tmpfs, one writable state mount, and
read-only configuration and secret files — and asserts that

- the image declares the unprivileged user, both listener ports, the state
  volume, and the service itself as its entrypoint;
- the liveness probe answers on the served listener, carrying the response
  headers every answer on that listener carries;
- the readiness probe answers ready on its own listener, with `no-store`, over a
  state directory that run created, and serves nothing else;
- an unauthenticated request to the gated surface is refused;
- the running process is that unprivileged user and not root, which is asked of
  the container runtime because the runtime image ships no shell to ask from
  inside;
- the root filesystem took no writes and the state database landed in its mount;
- no synthetic secret value reached the container log; and
- `SIGTERM` stops the service cleanly.

A failure prints the container log, with every placeholder the run mounted
replaced on the way out. The service is what should keep a secret value out of a
log line in the first place, and that is asserted separately, over the log as it
really is — but a run can fail long before it reaches that assertion, including
while starting, so what a failure prints is filtered rather than trusted.

The smoke test contacts nothing. Every credential it mounts is a placeholder it
wrote itself, each provider points at an unroutable address, no region is
configured so no map extract is downloaded and no surface index is built, the
first scheduled synchronisation is a year away, and no
request presents an Access assertion — so the lazy fetch of Cloudflare's signing
keys never happens either. Its state directory and published ports are its own,
so a host already running the deployment is untouched.

The image is an input rather than something the script builds: a local build
needs the `dhi.io` credential above, and the local gate must not require one. So
the smoke test is outside `mise run check` and `mise run quick`, and CI runs it
in the pull-request `Image` job, which already holds that credential.
`DOMESTIQUE_SMOKE_IMAGE` names the reference to run, which must already be in
the local image store; nothing pulls. In CI that reference is the image the job
just built: one platform can be loaded into the runner's image store, so what is
started is the artefact that was proved rather than a second build of it.

The repository provides non-secret, clearly placeholder deployment examples.
They must not make a direct Internet port publication easy to copy, and must not
contain an account identifier, callback hostname, token, or secret file
content.

## Published images

The project follows trunk-based development. There are no version tags and no
GitHub releases; the default branch is the only thing that publishes, and every
change reaches it through a pull request the full gate accepted.

A push to the default branch that touches an input of the image builds one GHCR
image index covering `linux/amd64` and pushes it to `ghcr.io/nobbs/domestique`
under two tags. It remains an index rather than a bare manifest even with one
architecture under it, because the software bill of materials and the `mode=max`
provenance travel as their own manifests beside the image — which is why the
operator instruction to pin the index digest is unaffected by the platform
count:

| Tag | Meaning |
| --- | --- |
| `sha-<short-commit>` | The immutable name of the image built from that commit. It never moves. |
| `latest` | A pointer to the most recent published default-branch image. It is for inspection, never a deployment instruction. |

A commit that changes nothing the image is built from publishes no image, so
`latest` may name an older commit than the branch head. That is the intended
result: an unchanged source tree does not need a new digest, and the digest
already published is still the one to deploy. There is no republish trigger,
because there is nothing a republished identical tree would give a host.

The publish job:

1. builds the same CGO-free target that pull requests prove, exactly once for
   the commit;
2. pushes the index and records its digest in the run summary, which is where an
   operator reads the value to deploy;
3. attaches BuildKit's software bill of materials and `mode=max` provenance
   attestations to the index; and
4. runs only after linting, tests, security analysis, and the browser UI checks
   have succeeded or been skipped by a path filter.

The deployment that follows it hands the Tailnet host that same index digest and
nothing else. The host composes the reference from the repository it is
configured with, pins it, restarts the service, and restores the digest it was
running if the new one fails the health gate — which asks both probes: that the
new image answers HTTP, and that it can read the state it was configured with — so an automated deployment that
goes wrong leaves the service that was already running, and CI reports the
failure. Automation therefore consumes exactly what an operator would: an
immutable digest read from the run that produced it. It has no path to a mutable
tag, another repository, or a locally built image, and the account it uses on
the host can run one command and no other.

**The images are not signed.** Sigstore keyless signing was removed together
with the tag-triggered release workflow, and GitHub artifact attestations are
not an available substitute: on a private repository they require GitHub
Enterprise Cloud, and this repository stays private. The trust argument is
therefore provenance of publication rather than a detached signature — the
package is private, only the default branch holds the permission that writes it,
the base images are Docker Hardened Images pinned by digest, and the deploying
host pins a digest it read from the run that produced it. A host must never
substitute an image from anywhere else.

A deploying host consumes only an immutable digest, never a mutable tag such as
`latest`. It needs a package-read credential for the private GHCR package and no
build credential. A machine may still build the pinned Dockerfile from a
checkout; such an image is a local build, not a published artefact, and carries
no provenance. Rollback means selecting an earlier published digest and
restarting the container; it does not restore SQLite state or bypass the
reauthorisation and safe-adoption rules.

Publish automation receives `contents: read` and `packages: write` and nothing
else. It receives no application secrets beyond the Docker Hardened Images pull
credential the build cannot proceed without, and does not contact live provider
accounts.

## Repository hygiene

The repository includes:

- the MIT `LICENSE`;
- a concise `SECURITY.md` with a private reporting channel and supported
  release policy;
- `.gitignore` entries for local configuration, databases, secrets, coverage,
  and build outputs;
- an explicit `.dockerignore` that excludes Git metadata, local state,
  configuration, secret files, and test artefacts from image contexts;
- documentation for the normal local check, manual sandbox acceptance, image
  selection and inspection, the Tailnet-host deployment boundary, and the
  automated deployment's host-side prerequisites; and
- shell-script linting for every script it carries, because both of them run
  somewhere a mistake is expensive.

A published image is deployable only when the default-branch run that produced
it was green end to end, no untriaged vulnerability finding blocks it, and the
manual sandbox acceptance still covers the current Wahoo and FIT contract.
