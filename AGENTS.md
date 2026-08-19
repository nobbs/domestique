# AGENTS.md

Guidance for AI coding agents working in this repository. Human contributors
should read [CONTRIBUTING.md](CONTRIBUTING.md) first; this file covers the same
ground with the details an agent needs to act without additional context.

## What this service is

`domestique` mirrors one private VeloPlanner route library to one or two Wahoo
accounts as device-ready FIT courses. It is a single-tenant, CGO-free Linux
Docker workload for a Tailnet host, published for `linux/amd64` and
`linux/arm64`, with no CLI. It also serves a **read-only browser UI** that
renders one stored route stage at a time on a map. Its only state-changing HTTP
surface is the Wahoo OAuth onboarding flow and the manual `POST /v1/sync`
trigger.

## Specifications are normative

[`docs/specs`](docs/specs) contains the accepted v1 contracts. Read the relevant
one before changing behavior:

| Specification | Covers |
| --- | --- |
| [service.md](docs/specs/service.md) | The overall contract; it wins over the implementation until deliberately revised |
| [implementation-architecture.md](docs/specs/implementation-architecture.md) | Package ownership, interface rules, composition root |
| [configuration.md](docs/specs/configuration.md) | File schema, secret inputs, validation |
| [sync-lifecycle.md](docs/specs/sync-lifecycle.md) | State transitions, safety gates, JSON contracts |
| [delivery.md](docs/specs/delivery.md) | Quality gate, container hardening, published images |

When an implementation detail contradicts a specification, treat the
specification as correct and say so rather than quietly matching the code. When
a change genuinely requires a different contract, update the specification in
the same change and call it out.

## Commands

The toolchain is pinned in [`.mise.toml`](.mise.toml). Run everything through
Mise; do not install tools globally or reach for a different Go version.

~~~sh
mise install
mise exec -- make quick
~~~

**GitHub Actions is the authoritative gate.** It runs the complete validation
for every changed path on every pull request, and its aggregate check is what a
merge must satisfy. Running a gate locally buys an earlier answer, not a
different one.

`make quick` is the routine loop, and what to run while iterating. It runs
everything `make check` runs except five checks it defers: `build-check`, which
cross-compiles both published architectures; `vulncheck` and `ui-audit`, which
need the network and a current advisory database; and `ui-browser-install` and
`ui-browser-test`, which download a browser and then drive it over the demo stack
for minutes. Nothing else is left out, and `make gate-check` fails if that stops
being true.

`make check` is the full gate: `prek`, lint, markdownlint, shellcheck,
actionlint, `go vet`, tests, TypeScript type checking, the UI lint and test
suites, the browser suite, `go mod tidy -diff`, `go mod verify`, `govulncheck`,
`npm audit`, `gitleaks`, a commit-hook cost check, a local-gate structure check, and a
cross-compile check for each published architecture. Individual targets
(`make test`, `make lint`, `make fmt`, `make ui-test`, `make build-check`) are
also available while iterating.

Run `make check` before reporting work complete. If part of it genuinely cannot
run — no network for the advisory databases, say — run `make quick`, say plainly
which checks did not run, and leave them to CI rather than implying a full gate
passed.

Tests run with `CGO_ENABLED=0` and `-shuffle=on`. They must stay deterministic
under shuffling.

`make coverage` writes a Go coverage profile to `.coverage/go.out` and the UI's
LCOV report to `.coverage/ui/lcov.info`, and prints a summary for each. It is not
part of `make check`, and nothing local fails on a percentage. CI publishes both
to Codecov under the `go` and `ui` flags, where the two patch statuses do decide
a merge: each requires the lines a change adds or alters to be covered at least
as well as the base commit's already are. The UI number is the Vitest suites and
the browser suite merged, so a change to code only the whole page exercises is
judged on coverage it actually has. `make coverage-ui` drives a browser and is
correspondingly slow; with none installed it keeps the unit half, says what it
left out, and still succeeds. See [CONTRIBUTING.md](CONTRIBUTING.md) for what is
measured and what is left out.

**The browser UI** lives in `internal/webui/app` (TypeScript, React, Vite,
MapLibre) and is compiled into the binary with `go:embed`, so `make build`
depends on `make ui-build`. Use `make ui-dev` for hot reload — it proxies the API
to a locally running service and forwards the Cloudflare Access assertion in
`DOMESTIQUE_DEV_ASSERTION`, so the identity gate behaves as it does in
production. Without that variable every proxied request answers 401; there is
deliberately no way to switch the gate off. The proxy also names the API's
configured browser origin, because state-changing routes require it; that
defaults to what `dev/setup.sh` writes, and `DOMESTIQUE_DEV_ORIGIN` overrides it
when `DOMESTIQUE_DEV_API` points at the deployed container. Building images requires
`docker login dhi.io`, because the base images are Docker Hardened Images.

**To check that the production image still runs**, use `make container-smoke`. It
starts the image the way `docs/compose.example.yml` starts it — unprivileged,
read-only root, no capabilities, one tmpfs, one state mount — and asserts the
probes, the response headers, the refusal of an anonymous caller, the process's
own uid, that nothing was written outside the state mount, and that no secret
value reached the log. It builds nothing: point `DOMESTIQUE_SMOKE_IMAGE` at an
image already in the local store, or build `domestique:smoke` first. It is
outside `make check`, because building an image needs the `dhi.io` login above;
CI runs it in the pull-request `Image` job. Every credential it mounts is a
placeholder it writes itself, and it reaches no provider.

**To develop against no data at all**, run `make demo`. One command writes a
throwaway configuration under `.local/demo`, seeds a database with the synthetic
library in `internal/demo`, and starts the API and the UI dev server against it.
It needs no account, no secret and no snapshot, and it cannot reach VeloPlanner,
Wahoo, Pushover or a deployment. The identity gate still runs in full: the demo
mints an assertion with a key it generates at start-up and verifies it with the
production verifier. Use it for UI work, and prefer it over a snapshot whenever
the change does not depend on real routes. `./dev/demo.sh --with-bundle` builds
the browser UI first, so the API also serves a current production bundle at its
own port — the arrangement a deployment runs, and what the browser suite's bundle
project drives.

**To develop against real data**, run `make dev-setup` once (snapshots the
deployed SQLite state into `.local/dev`), then `make dev-api` and `make ui-dev`.
The dev service reads VeloPlanner but **cannot reach Wahoo** — its encryption
key is a placeholder, so a run fails at the state step before any Wahoo request,
and its Wahoo endpoints are unroutable. Never weaken those guards to "make sync
work" in development; use the sandbox acceptance check instead.

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
  key; access tokens live only in memory.
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
enforces this through `make lint`. Do not use Testify's `mock` or `suite`
packages — deterministic hand-written fakes remain the convention.
[`internal/route`](internal/route) is the worked example; packages still using
plain `testing.T` checks are converted separately.

Browser UI tests come in two suites. `make ui-test` is Vitest plus Testing
Library over the reusable components in `src/components` and the API client's
parsing and error paths: jsdom, no browser, and a second to run — anything it can
reach belongs there.

`make ui-browser-test` is Playwright over a real Chromium, and covers what jsdom
cannot observe: the MapLibre map, which needs WebGL, and the interactions that
span components rather than living inside one — scrubbing the elevation chart,
dragging a stretch out of the map, following a card into a route. The specs are
in `internal/webui/app/e2e`, and `make ui-browser-install` downloads the browser
they need. It runs against `dev/demo.sh`, so it reads the synthetic library in
`internal/demo` and never a real route, and its fixtures answer the one
third-party request the application makes — the basemap style — from memory and
fail the test if anything else leaves the page. No screenshot is stored: a visual
assertion compares the map against itself within the run, after waiting for two
identical frames.

It drives that one stack as two projects. `dev-server` runs the specs in `e2e`
against the Vite dev server. `bundle` runs the specs in `e2e/contract` against
the Go service itself — the production bundle from `internal/webui`'s embed
handler, and the routes, gates and headers a deployment serves it with — so that
the shipped client is seen parsing a real response rather than a fixture. Its
stack is started with `dev/demo.sh --with-bundle`, which builds the UI first so
the embedded bundle is the current one, and its harness adds the identity
assertion the dev proxy would and forwards state-changing requests with the
configured origin, because a browser will not let a test set its own.

That suite renders the map, but it does not look at it. Checking a map change by
eye is still skippable, so a change may be handed over without anyone having seen
it. Say so plainly when it has not, and say what specifically went unseen, so
nobody reads a green `make check` as a change that was looked at.

## Files an agent must not touch or read out

`config.toml`, `.local/`, `secrets/`, `*.db*`, `.cache/`, `node_modules/`, and
`internal/webui/app/dist/` are gitignored local artifacts; the first four may
hold real credentials and private route data. Do not read them for context, copy
their values into the repository, or commit them.
[`config.example.toml`](config.example.toml) is the tracked reference and
references secret **files** — it never embeds secret values.

Never commit credentials, OAuth tokens, the Wahoo client secret, personal route
fixtures, generated FIT files, SQLite state, or Raspberry Pi deployment files.

## Commits and pull requests

Use [Conventional Commits](https://www.conventionalcommits.org/en/v1.0.0/) with
the scopes already established in the history (`sync`, `elevation`, `fit`,
`wahoo`, `http`, `config`, `runtime`, `container`, `macos`, `hetzner`,
`release`). Keep changes focused. A pull request should explain any change to
the normative
specifications, to a safety gate, or to the operator action required for
deployment.
