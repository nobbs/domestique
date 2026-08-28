# AGENTS.md

Guidance for AI coding agents working in this repository. Human contributors
should read [CONTRIBUTING.md](CONTRIBUTING.md) first. Where a rule has a
reason, [the delivery specification](docs/specs/delivery.md) holds it; this file
gives the command and what turns on it.

## What this service is

`domestique` mirrors one private VeloPlanner route library to one or two Wahoo
accounts as device-ready FIT courses. It is a single-tenant, CGO-free Linux
Docker workload for a Tailnet host, published for `linux/amd64` alone, with no
CLI. It also serves a **read-only browser UI** that
draws the whole stored library on one map and gives each route a page of its
own, plus a settings page. Its state-changing HTTP surface is the Wahoo OAuth
onboarding flow, the manual run triggers, and the `PUT /v1/settings/*` section
writes, which store the runtime settings the operator edits from that page. Nothing on the surface
edits a route.

## Specifications are normative

[`docs/specs`](docs/specs) contains the accepted contracts. Read the relevant
one before changing behavior:

| Specification | Covers |
| --- | --- |
| [service.md](docs/specs/service.md) | The overall contract; it wins over the implementation until deliberately revised |
| [implementation-architecture.md](docs/specs/implementation-architecture.md) | Package ownership, interface rules, composition root |
| [configuration.md](docs/specs/configuration.md) | File schema, the one secret input, validation, and the runtime settings and credentials held in the database |
| [sync-lifecycle.md](docs/specs/sync-lifecycle.md) | State transitions, safety gates, JSON contracts |
| [delivery.md](docs/specs/delivery.md) | Quality gate, container hardening, published images |

When an implementation detail contradicts a specification, treat the
specification as correct and say so rather than quietly matching the code. When
a change genuinely requires a different contract, update the specification in
the same change and call it out.

[`docs/glossary.md`](docs/glossary.md) fixes the one meaning each domain word
has. Read it before naming anything, and prefer its word over a new one — the
terms that already mean several things got that way because each was defined
locally, in the module that needed it. Where the code disagrees with the
glossary today, [`docs/naming-drift.md`](docs/naming-drift.md) records the
disagreement; add to it rather than quietly renaming across a boundary.
[`docs/backend-layout.md`](docs/backend-layout.md) does the same for the Go
tree: which files have outgrown their package, and which parts of the
architecture specification's layout no longer match the repository.

## Commands

The toolchain is pinned in [`.mise.toml`](.mise.toml) and every command is a
Mise task in [`mise-tasks.toml`](mise-tasks.toml). Run everything through Mise;
do not install tools globally or reach for a different Go version. There is no
Makefile and no runner-free entry point: `mise run <task>` is how this
repository is built, tested, and checked.

~~~sh
mise install
mise run quick
~~~

A green run still emits several kilobytes of task output. When iterating, keep
success to one line and read the log only when it is not:

~~~sh
mise run -q quick > .local/quick.log 2>&1 && echo OK || tail -40 .local/quick.log
~~~

**GitHub Actions is the authoritative gate.** It runs the complete validation
for every changed path on every pull request, and its aggregate check is what a
merge must satisfy. Running a gate locally buys an earlier answer, not a
different one.

`mise run quick` is the routine loop, and what to run while iterating. It runs
everything `mise run check` runs except six checks it defers — `build-check`,
`test-race`, `vulncheck`, `ui-audit`, `ui-browser-install` and
`ui-browser-test` — so a green `quick` is not a full gate. `mise run check` is
that full gate; reach for it when a change specifically implicates one of the
six — the release build, concurrent code, a dependency, or the browser suite —
rather than as routine. [The delivery specification](docs/specs/delivery.md)
says why each is deferred and how the subset is asserted.

Run `mise run quick` before reporting work complete, and say plainly which
checks you ran. Reporting a green `mise run quick` as a full gate is the failure
this rule exists to prevent.

A second run says `sources up-to-date, skipping` for every check whose files
have not moved, which is why the loop is cheap to repeat. It is a local
convenience and nothing more: CI runs every task with `--force`. Use
`mise run --force quick` when you want the checks run regardless — after
switching branches with a dirty state directory, or when you suspect the cache
rather than the code.

Tests run with `CGO_ENABLED=0` and `-shuffle=on`. They must stay deterministic
under shuffling. `mise run test-race` runs the same suite under the race
detector, which asks a different question and needs cgo. Reach for it after
touching anything concurrent — the sync service and its reporter, the Wahoo
client, the Access verifier, or the composition root. Nothing it does reaches
the shipped artefact.

`mise run coverage` writes a Go coverage profile to `.coverage/go.out` and the
UI's LCOV report to `.coverage/ui/lcov.info`, and prints a summary for each.
Both halves are unit suites, so it takes seconds and needs no browser. CI
publishes both to Codecov under the `go` and `ui` flags, where `patch/go` — the
one enforced status — requires the Go lines a change adds or alters to be
covered at least as well as the base commit's already are. An uncovered Go
change does not merge.

**Run `mise run patch-coverage` before the first push of a change.** It measures
both halves and then grades the patch the way that status will, against the
merge base with `main` and including the working tree, naming each uncovered
added line as `file:line`. A Go shortfall fails the task. Learning the same
thing from CI costs a push, a five-minute run and a round of polling, which is
where deliveries in this repository have historically lost the most time. Its
UI number reproduces Codecov's exactly; its Go one is an estimate that reads a
couple of tenths of a point low. A patch within a point below the project total
is reported as too close to call and fails the task, because deciding that band
needs the base's own report — so it is stricter than the status by under a
point and never looser.
[The delivery specification](docs/specs/delivery.md#coverage) states what is
measured, what is deliberately not, and why the statuses are shaped that way.

**The browser UI** lives in `internal/webui/app` (TypeScript, React, Vite,
MapLibre) and is compiled into the binary with `go:embed`, so `mise run build`
depends on `mise run ui-build`. Use `mise run ui-dev` for hot reload — it
proxies the API to a locally running service and forwards the Cloudflare Access
assertion in `DOMESTIQUE_DEV_ASSERTION`, so the identity gate behaves as it does
in production. Without that variable every proxied request answers 401; there is
deliberately no way to switch the gate off. The proxy also names the API's
configured browser origin, because state-changing routes require it; that
defaults to what `dev/setup.sh` writes, and `DOMESTIQUE_DEV_ORIGIN` overrides it
when `DOMESTIQUE_DEV_API` points at the deployed container. Building images
requires `docker login dhi.io`, because the base images are Docker Hardened
Images.

**To check that the production image still runs**, use
`mise run container-smoke`. It starts the image under the runtime
`docs/compose.example.yml` documents and asserts that contract, so a service
that builds but cannot come up fails here rather than at deploy time. It builds
nothing: point `DOMESTIQUE_SMOKE_IMAGE` at an image already in the local store,
or build `domestique:smoke` first. It is outside `mise run check` because
building an image needs the `dhi.io` login above; CI runs it in the
pull-request `Image` job.
[The delivery specification](docs/specs/delivery.md#proving-the-runtime-contract)
lists everything it asserts.

**To develop against no data at all**, run `mise run demo`. One command writes a
throwaway configuration under `.local/demo`, seeds a database with the synthetic
library in `internal/demo`, and starts the API and the UI dev server against it.
It needs no account, no secret and no snapshot, and it cannot reach VeloPlanner,
Wahoo, Pushover or a deployment. The identity gate still runs in full: the demo
mints an assertion with a key it generates at start-up and verifies it with the
production verifier. Use it for UI work, and prefer it over a snapshot whenever
the change does not depend on real routes. `./dev/demo.sh --with-bundle` builds
the browser UI first, so the API also serves a current production bundle at its
own port — the arrangement a deployment runs, and what the browser suite's
bundle project drives.

**To develop against real data**, run `mise run dev-setup` once (snapshots the
deployed SQLite state into `.local/dev`), then `mise run dev-api` and
`mise run ui-dev`. The dev service reads VeloPlanner but **cannot reach Wahoo**
— its encryption key is a placeholder, so a run fails at the state step before
any Wahoo request, and its Wahoo endpoints are unroutable. Never weaken those
guards to "make sync work" in development; use the sandbox acceptance check
instead.

## Architecture rules

`cmd/domestique/main.go` is the only composition root and holds no business
logic. Everything else is an `internal/` package with one responsibility.

- Manual constructor injection only. No Wire, Dig, Fx, Do, service locator, or
  global registry.
- Interfaces are declared in the **consuming** package, are small, and exist
  only where a real adapter boundary or test double needs them. Constructors
  accept interfaces and return concrete structs.
- Adapters (`veloplanner`, `fit`, `wahoo`, `sqlite`, `pushover`) never import
  each other, and never import `sync`, `oauth`, `schedule`, or `httpapi`.
  Dependency arrows stay one-way.
- No mutable package-level state and no `init` functions. Package-level state is
  limited to immutable constants and precompiled regular expressions; the
  `gochecknoinits` and `gochecknoglobals` linters enforce this.
- No constructor starts a goroutine, contacts an upstream, reads global
  configuration, or calls `log.Fatal`. Startup errors return to `main`, which
  alone decides the exit code.
- Every external call takes a caller-supplied `context.Context` and a bounded
  timeout owned by the adapter.
- The code stays CGO-free and provider-agnostic. Do not introduce a secret
  provider syntax (`op://`, `env:`, fnox references) into configuration.

Do not add a `pkg/`, `internal/common`, `interfaces`, `models`, or generic
repository package. Add a package only when it owns a distinct responsibility.

## Browser UI component layers

`internal/webui/app/src/components/ui` is shadcn registry output for the
`base-nova` style, on Base UI. **Never edit a file in it.** Needing different
behaviour is not a reason to: compose above the primitive, or hand it what you
want through its `render` prop. A vendored file that imports application code
inverts the dependency and conflicts with every later `shadcn add`.

Application code imports primitives directly — `field`, `popover`, `card`,
`alert` and the rest are used where they are needed, with no wrapper. A wrapper
around a primitive whose vocabulary was not renamed is only a second name for
it.

`ui/button` is the one exception. Only `components/Button.tsx` may import it,
because that file renames the variants (`panel` and `warning` have no primitive
equivalent) and a second button vocabulary has cost this UI before. Biome's
`noRestrictedImports` enforces this; the exemption list in `biome.json` is the
whole of it.

Before hand-rolling a control, check whether the registry ships one — several
here were built by hand beside a component that already did the job. The
opposite also holds: the map and chart layer (`components/map`,
`components/route`) owes shadcn nothing, and a MapLibre layer or an SVG chart
should not be forced through a primitive that does not fit it.

## Comments

Comment what the code cannot say: a constraint that is not visible locally, a
unit or invariant a signature does not carry, an upstream quirk a reader would
take for a mistake. Go doc comments on exported identifiers always belong.

Everything else is noise, and this repository has too much of it. Do not argue
with yourself in a comment — weighing alternatives and defending the choice
made is pull-request prose, and it goes stale in the file. Do not restate a
line that already reads clearly, and do not narrate a test above it when its
name and assertion messages already say what it proves. A comment longer than
the code it introduces is the signal to stop; two lines is the working ceiling,
and going past it needs knowledge a reader could not get from the code — a
documented upstream behavior, a safety gate's reason, a bug worked around.

When a block needs a paragraph to be comprehensible, make the block
comprehensible instead: a named helper or intermediate value stays correct as
the code changes, and a comment does not.

## Safety rules that must not be weakened

These are the reason the service exists in this shape. A change that relaxes one
of them needs an explicit specification revision, not a quiet edit.

- **Ownership before deletion.** Domestique deletes only Wahoo routes it owns
  through its deterministic `external_id`. Lost local state is never authority
  to delete an unknown Wahoo route — the reconciler re-adopts by `external_id`
  before it creates or removes anything.
- **Deletion gates.** No automatic run deletes more than the configured maximum
  owned routes per target. A previously populated library that becomes empty is
  blocked unless the operator set the explicit empty-source acknowledgement.
  Clearing a target is the one deliberate exception, and only because it is
  never automatic: it deletes every owned route from one slot, at an operator's
  direct request, and nothing schedules it. It does not weaken ownership —
  it removes only routes carrying an external ID this service issued, so a
  hand-made route is as untouchable there as anywhere else.
- **A failed source inventory is never destructive.** Authentication failure,
  malformed data, or a suspicious shrink stops deletion and raises an alert.
- **Secrets stay out of everything observable.** Logs, notifications, and error
  messages carry aggregate counts and a stable failure category only — never
  tokens, credentials, route names, geometry, or raw upstream response bodies.
  Sensitive config values use dedicated types with unexported fields and no JSON
  tags.
- **Geometry is served only by its own endpoint**, only to the gated identity,
  and only from local stored state. It must not appear in the inventory listing,
  the status endpoint, logs, or notifications.
- **Refresh tokens are encrypted at rest** in SQLite with the state-encryption
  key; access tokens live only in memory. Every credential entered on the
  settings page is kept the same way, and is write-only: no endpoint serves one
  back, in any form, to any caller.
- **All non-OAuth HTTP is read-only and identity-gated to one principal.** The
  only identity the handler accepts is a signed `Cf-Access-Jwt-Assertion` it
  verifies itself, as set out in
  [docs/cloudflare-access.md](docs/cloudflare-access.md). Do not add a public
  listener or loosen the gate. In particular, do not reintroduce trust in
  `Tailscale-User-Login`: Serve still fronts the listener and Tailnet members can
  still reach it, so honouring that header would be a second front door, and a
  tunnel forwards client headers verbatim, so it would be a forgeable one. The
  tunnel adds no listener — it dials outward — and no principal. Its origin must
  stay the Tailscale Service name, which is what keeps the tunnel node from
  addressing the host directly.

## Testing

Tests live beside the package under test and use deterministic in-memory fakes
or `httptest` servers. **No normal test contacts VeloPlanner, Wahoo, Pushover,
Tailscale, or any network service.** The FIT/Wahoo sandbox acceptance check in
[`internal/fit/wahoo_sandbox_test.go`](internal/fit/wahoo_sandbox_test.go) is
invoked separately and never receives production secrets through CI.

Add a regression test for every behavior change, especially for safety gates.
Use the `schedule` package's trigger seam rather than sleeping on the wall
clock. Fixtures must contain no personal route data.

Go assertions use [Testify](https://github.com/stretchr/testify). Use `require`
for setup and preconditions, so a failed one stops the test before it cascades,
and `assert` for independent expectations, so a single run reports every
mismatch. Prefer the semantic assertion over a hand-rolled comparison:
`require.ErrorIs`, `require.ErrorAs`, and `require.ErrorContains` for errors,
and `assert.InDelta` for floating-point values. The `testifylint` linter
enforces this through `mise run lint`. Do not use Testify's `mock` or `suite`
packages — deterministic hand-written fakes remain the convention.
[`internal/route`](internal/route) is the worked example; packages still using
plain `testing.T` checks are converted separately.

Browser UI tests come in two suites, and which one a test belongs in is a
decision. `mise run ui-test` is Vitest plus Testing Library over the reusable
components in `src/components` and the API client's parsing and error paths:
jsdom, no browser, and a second to run — anything it can reach belongs there.
`mise run ui-browser-test` is Playwright over a real Chromium, with the specs in
`internal/webui/app/e2e` and `mise run ui-browser-install` for the browser they
need; it is for what jsdom cannot observe — the MapLibre map, which needs WebGL,
and the interactions that span components — and is not a second home for logic a
component test could reach. It reads the synthetic library in `internal/demo`
and never a real route.
[The delivery specification](docs/specs/delivery.md#the-browser-suite) covers its
two projects, its hermeticity, and how a visual assertion works without a stored
screenshot.

That suite renders the map, but it does not look at it. Checking a map change by
eye is still skippable, so a change may be handed over without anyone having seen
it. Say so plainly when it has not, and say what specifically went unseen, so
nobody reads a green gate as a change that was looked at.

## Files an agent must not touch or read out

`config.toml`, `.local/`, `secrets/`, `*.db*`, `.cache/`, `node_modules/`, and
`internal/webui/app/dist/` are gitignored local artifacts; the first four may
hold real credentials and private route data. Do not read them for context, copy
their values into the repository, or commit them.
[`config.example.toml`](config.example.toml) is the tracked reference. It names
one secret **file** — the state encryption key — and never embeds a secret
value; every other credential is entered on the running service's settings page
rather than configured.

Never commit credentials, OAuth tokens, the Wahoo client secret, personal route
fixtures, generated FIT files, SQLite state, or host deployment files.

## Commits and pull requests

Use [Conventional Commits](https://www.conventionalcommits.org/en/v1.0.0/) with
the scopes already established in the history (`sync`, `elevation`, `fit`,
`wahoo`, `http`, `config`, `runtime`, `container`, `hetzner`, `release`). Keep
changes focused. A pull request should explain any change to the normative
specifications, to a safety gate, or to the operator action required for
deployment.
