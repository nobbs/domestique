# AGENTS.md

Guidance for AI coding agents. Humans start at
[CONTRIBUTING.md](CONTRIBUTING.md). Rules here state the command; the linked
specs hold the reasons.

`domestique` mirrors one private VeloPlanner route library to one or two Wahoo
accounts as device-ready FIT courses, plus a read-only browser UI (library map,
per-route pages, settings). Single-tenant, CGO-free, `linux/amd64` Docker
workload on a Tailnet host; no CLI. State-changing HTTP is limited to Wahoo
OAuth onboarding, manual run triggers, and `PUT /v1/settings/*`.

## Commands

Toolchain pinned in [`.mise.toml`](.mise.toml); every command is a Mise task in
[`mise-tasks.toml`](mise-tasks.toml). No Makefile, no global installs.

| Step | Command |
| --- | --- |
| One-time setup | `mise install` |
| Routine loop while iterating | `mise run quick` |
| Full local gate | `mise run check` |
| Patch coverage (before first push) | `mise run patch-coverage` |
| Race detector (after concurrent changes) | `mise run test-race` |
| Go format | `mise run fmt` |
| Unit coverage profiles | `mise run coverage` |
| UI unit tests (jsdom) | `mise run ui-test` |
| UI browser tests (Playwright) | `mise run ui-browser-test` |
| Dev against synthetic data | `mise run demo` |
| Dev against real data | `mise run dev-setup`, then `dev-api` + `ui-dev` |
| UI hot reload | `mise run ui-dev` |
| Production image smoke test | `mise run container-smoke` |

Keep iteration output to one line:

~~~sh
mise run -q quick > .local/quick.log 2>&1 && echo OK || tail -40 .local/quick.log
~~~

- **GitHub Actions is the authoritative gate.** Local runs buy an earlier
  answer, not a different one.
- **`quick` is not a full gate.** It defers `build-check`, `test-race`,
  `vulncheck`, `ui-audit`, `ui-browser-install`, `ui-browser-test`. Run
  `check` when a change implicates the release build, concurrent code, a
  dependency, or the browser suite ([why](docs/specs/delivery.md)).
- **Report honestly.** Run `quick` before declaring work complete and say
  which checks you ran; never present a green `quick` as a full gate.
- **`patch-coverage` before the first push.** CI's `patch/go` status blocks
  uncovered Go changes; this grades the patch the same way locally, naming
  uncovered lines ([details](docs/specs/delivery.md#coverage)).
- **Tests must survive `-shuffle=on`** (always on, `CGO_ENABLED=0`).
  `test-race` needs cgo; run it after touching the sync service/reporter,
  Wahoo client, Access verifier, or composition root.
- **`ui-dev` needs `DOMESTIQUE_DEV_ASSERTION`** or every proxied request
  answers 401; there is deliberately no way to switch the identity gate off.
  Image builds need `docker login dhi.io`.
- **The dev service cannot reach Wahoo** by design. Never weaken those guards
  to "make sync work"; use the sandbox acceptance check. Prefer `demo` over a
  snapshot whenever the change does not depend on real routes.
- **`--force`** (`mise run --force quick`) re-runs regardless of the source
  cache; CI always forces.
- **The prek commit hook runs automatically** on the staged files; don't run
  it by hand. It may apply safe whitespace fixes and exit non-zero — review,
  re-stage, and commit again.

## Specifications are normative

[`docs/specs`](docs/specs) holds the accepted contracts — read the relevant one
before changing behavior:

| Specification | Covers |
| --- | --- |
| [service.md](docs/specs/service.md) | The overall contract; wins over the implementation until deliberately revised |
| [implementation-architecture.md](docs/specs/implementation-architecture.md) | Package ownership, interface rules, composition root |
| [configuration.md](docs/specs/configuration.md) | File schema, the one secret input, runtime settings and credentials |
| [sync-lifecycle.md](docs/specs/sync-lifecycle.md) | State transitions, safety gates, JSON contracts |
| [task-layer.md](docs/specs/task-layer.md) | Background activities, their state, when they run |
| [delivery.md](docs/specs/delivery.md) | Quality gate, container hardening, published images |

When code contradicts a spec, the spec is correct — say so rather than quietly
matching the code. A change that genuinely needs a different contract updates
the spec in the same change.

[`docs/glossary.md`](docs/glossary.md) fixes each domain word's one meaning —
prefer its word over a new one. [`docs/naming-drift.md`](docs/naming-drift.md)
records where code disagrees today; add there rather than renaming across a
boundary. [`docs/backend-layout.md`](docs/backend-layout.md) holds the pending
`internal/` restructure.

## Architecture rules

`cmd/domestique/main.go` is the only composition root and holds no business
logic. Everything else is an `internal/` package with one responsibility.

- **Manual constructor injection only.** No Wire, Dig, Fx, Do, service
  locator, or global registry.
- **Interfaces live in the consuming package**, are small, and exist only
  where a real adapter boundary or test double needs them. Constructors accept
  interfaces and return concrete structs.
- **Adapters** (`veloplanner`, `fit`, `wahoo`, `sqlite`, `pushover`) never
  import each other, and never import `sync`, `oauth`, `task`, or `httpapi`.
- **No mutable package-level state, no `init`** (`gochecknoinits`,
  `gochecknoglobals` enforce this).
- **Constructors are inert**: no goroutines, upstream calls, global config
  reads, or `log.Fatal`. Startup errors return to `main`.
- **Every external call** takes a caller-supplied `context.Context` and a
  bounded timeout owned by the adapter.
- **Stay CGO-free and provider-agnostic**; no secret provider syntax
  (`op://`, `env:`) in configuration.
- **No `pkg/`, `internal/common`, `interfaces`, `models`**, or generic
  repository package. A new package must own a distinct responsibility.

## Browser UI component layers

- `src/components/ui` is shadcn registry output (`base-nova`, Base UI).
  **Never edit a file in it** — compose above the primitive or use its
  `render` prop.
- Application code imports primitives directly, no wrappers. Exception:
  `ui/button` may only be imported by `components/Button.tsx` (Biome's
  `noRestrictedImports` enforces it).
- `components/` is presentational and reusable across pages; `features/` is
  page-bound composition and its data access.
- Before hand-rolling a control, check whether the registry ships one. The
  map/chart layer (`components/map`, `components/route`) owes shadcn nothing.

## Comments

Comment only what the code cannot say: non-local constraints, units or
invariants a signature lacks, upstream quirks that read as mistakes. Doc
comments on exported identifiers always belong. Two lines is the ceiling; no
weighing alternatives, restating clear lines, or narrating tests. A block that
needs a paragraph should become a named helper instead.

## Safety rules that must not be weakened

Relaxing one of these needs an explicit spec revision, not a quiet edit. Full
statements live in the linked specs.

- **Ownership before deletion**: only Wahoo routes owned via deterministic
  `external_id` are deleted; the reconciler re-adopts before creating or
  removing ([sync-lifecycle.md](docs/specs/sync-lifecycle.md#deletion-gates)).
- **Deletion gates**: per-run deletion maximum; empty-source block unless
  acknowledged. Clearing a target is the one deliberate, never-automatic
  exception.
- **A failed source inventory is never destructive** — it stops deletion and
  alerts.
- **Secrets stay out of everything observable**: logs, notifications, and
  errors carry aggregate counts and a stable failure category only — never
  tokens, route names, geometry, or raw response bodies.
- **Geometry is served only by its own endpoint**, only to the gated identity
  ([service.md](docs/specs/service.md)).
- **Refresh tokens are encrypted at rest**; access tokens in memory only;
  settings-page credentials are write-only
  ([configuration.md](docs/specs/configuration.md)).
- **All non-OAuth HTTP is read-only and identity-gated** to one
  self-verified `Cf-Access-Jwt-Assertion` principal. No public listener, no
  trust in `Tailscale-User-Login`
  ([cloudflare-access.md](docs/cloudflare-access.md)).

## Testing

- Tests live beside the package, using deterministic in-memory fakes or
  `httptest`. **No normal test contacts any network service.** The sandbox
  acceptance check
  ([wahoo_sandbox_test.go](internal/fit/wahoo_sandbox_test.go)) is invoked
  separately, never with production secrets in CI.
- **Regression test for every behavior change**, especially safety gates. Use
  the `task` package's clock seams, not wall-clock sleeps. No personal route
  data in fixtures.
- **Testify**: `require` for setup/preconditions, `assert` for independent
  expectations; prefer semantic assertions (`ErrorIs`, `ErrorAs`,
  `ErrorContains`, `InDelta`) — `testifylint` enforces. No `mock` or `suite`
  packages; hand-written fakes. [`internal/route`](internal/route) is the
  worked example.
- **UI**: `ui-test` (Vitest/jsdom) is the default home; `ui-browser-test`
  (Playwright, `e2e/`) only for what jsdom cannot observe — the WebGL map and
  cross-component interactions
  ([details](docs/specs/delivery.md#the-browser-suite)). It renders the map
  but does not look at it: say plainly when a visual change went unseen.

## Files an agent must not touch or read out

`config.toml`, `.local/`, `secrets/`, `*.db*`, `.cache/`, `node_modules/`, and
`internal/webui/app/dist/` are gitignored; the first four may hold real
credentials and private route data. Do not read them for context or commit
them. [`config.example.toml`](config.example.toml) is the tracked reference —
it names one secret **file** (the state encryption key) and never embeds a
value. Never commit credentials, OAuth tokens, personal route fixtures,
generated FIT files, SQLite state, or host deployment files.

## Commits and pull requests

[Conventional Commits](https://www.conventionalcommits.org/en/v1.0.0/) with
established scopes (`sync`, `elevation`, `fit`, `wahoo`, `http`, `config`,
`runtime`, `container`, `hetzner`, `release`). Keep changes focused. A PR must
explain any change to a normative spec, a safety gate, or the operator action
required for deployment.
