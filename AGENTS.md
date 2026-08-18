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
mise exec -- make check
~~~

`make check` is the complete gate and is exactly what CI runs: `prek`, lint,
markdownlint, shellcheck, actionlint, `go vet`, tests, TypeScript type checking, the UI lint
and test suites, `go mod tidy -diff`, `go mod verify`, `govulncheck`,
`npm audit`, `gitleaks`, and a cross-compile check for each published
architecture. Individual targets (`make test`, `make lint`, `make fmt`,
`make ui-test`, `make build-check`) are available while iterating, but run the
full gate before reporting work complete.

Tests run with `CGO_ENABLED=0` and `-shuffle=on`. They must stay deterministic
under shuffling.

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

Browser UI tests are Vitest plus Testing Library over the reusable components in
`src/components` and the API client's parsing and error paths. The map component
itself is not unit-tested — it needs WebGL.

Checking a map change by running the app is currently skippable, so a change may
be handed over without ever having been rendered. Say so plainly when it has
not, and say what specifically went unseen, so nobody reads a green `make check`
as a change that was looked at. Everything a test can still cover — the style URL
chosen, the palette selected, the geometry handed to a layer — belongs in a test
whether or not the map itself was opened.

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
