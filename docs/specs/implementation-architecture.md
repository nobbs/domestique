# Domestique implementation architecture specification

**Status:** accepted v1 design

This is a subordinate specification to [the service contract](service.md).
It defines the Go module layout, package ownership, manually wired dependencies,
and implementation sequence. It does not introduce a framework, a DI container,
or a public library API.

## Architectural decision

Domestique is a small private HTTP service with a single binary. It uses a
right-sized ports-and-adapters style rather than a generic clean-architecture
framework:

- the route model and synchronisation rules are independent of HTTP, SQLite, and
  upstream protocols;
- each upstream or infrastructure concern has one concrete adapter package;
- interfaces appear only in the consuming package where tests or a real adapter
  boundary need substitution; and
- main is the composition root and calls constructors explicitly.

Manual constructor injection is final for v1. There is no Wire, Dig, Fx, Do,
service locator, global registry, or package initialisation with side effects.

## Module and directory layout

The module is github.com/nobbs/domestique. All implementation packages are
private to this service.

~~~text
.
├── cmd/
│   └── domestique/
│       └── main.go                 process composition and lifecycle only
├── internal/
│   ├── config/                     Koanf loading, secret input, validation
│   ├── route/                      source-stage value types and invariants
│   ├── sync/                       reconciliation use case and its interfaces
│   ├── oauth/                      Wahoo OAuth use case and its interfaces
│   ├── schedule/                   delayed-start and hourly execution
│   ├── httpapi/                    Tailnet-gated handlers and JSON mapping
│   ├── elevation/                  device-export elevation normalization
│   ├── surface/                    OSM surface classification and snapping
│   ├── veloplanner/                VeloPlanner HTTP source adapter
│   ├── fit/                        FIT encoding adapter
│   ├── wahoo/                      Wahoo OAuth and route HTTP adapter
│   ├── sqlite/                     encrypted durable-state adapter
│   ├── pushover/                   notification adapter
│   └── webui/                      embedded browser UI
│       └── app/                    TypeScript source and its build output
├── docs/
│   └── specs/
└── testdata/                       non-personal, sanitised fixtures only
~~~

The browser UI is compiled into the binary with `go:embed`, so the service ships
as one signed artefact and the API and UI cannot drift apart in a deployment.
`internal/webui/app/dist` is build output; only its placeholder is tracked.

There is no public pkg directory, internal/common package, interfaces package,
models package, or generic repository package. A package is added only when it
owns a distinct responsibility in this tree.

## Package responsibilities

| Package | Owns | Must not own |
| --- | --- | --- |
| config | TOML and environment layering, file-secret resolution, validation, immutable runtime settings | HTTP clients, business decisions, provider-specific secret syntax |
| route | source-stage identity, geometry, revision, and validation types | SQL, HTTP, FIT, Wahoo details |
| sync | inventory reconciliation, deletion gates, target progress, aggregate run result | HTTP handlers, SQL queries, Wahoo URLs |
| oauth | one-time callback state, target onboarding, duplicate-account rejection | HTTP routing, SQL queries, Wahoo URL formatting |
| schedule | startup delay, hourly cadence, no-overlap guard, cancellation | sync decisions or notification content |
| httpapi | Tailnet identity gate, routing, request parsing, JSON status and error mapping, response security and cache headers | OAuth exchange, sync logic, or how the UI is built |
| webui | the embedded browser bundle and serving it; the TypeScript application | HTTP routing, identity, or any knowledge of persistence |
| elevation | sampling and median-filtering the exported elevation profile | source fetching, storage, FIT bytes |
| surface | OSM surface and tracktype classification, Overpass querying, snapping a stage to the ways under it, caching policy | SQL, HTTP routing, what the UI draws |
| veloplanner | login, listing, detail decoding, route conversion | SQLite and Wahoo concerns |
| fit | deterministic FIT bytes for one valid route stage | VeloPlanner requests, OAuth, HTTP |
| wahoo | authorisation URL, exchange, refresh, user lookup, FIT route operations, rate headers | route-source parsing, SQLite queries, Pushover |
| sqlite | migrations, encrypted token storage, snapshots and commits | Wahoo or VeloPlanner HTTP |
| pushover | delivery of an already safe notification | run aggregation or secret resolution |

Config, route, and the use-case packages have no dependency on concrete
infrastructure adapters. Adapters may depend on route types only where
translation requires them.

## Consumer-defined interfaces

Interfaces are small, declared beside the use case that consumes them, and
created because an adapter boundary or test double requires them. Constructors
accept interfaces and return concrete structs.

The sync package owns these conceptual seams:

~~~go
type Source interface {
    Inventory(ctx context.Context, previous Snapshot) (Inventory, error)
}

type Encoder interface {
    Encode(ctx context.Context, stage route.Stage) ([]byte, error)
}

type Target interface {
    Reconcile(ctx context.Context, change Change) (Result, error)
}

type State interface {
    Load(ctx context.Context) (Snapshot, error)
    Commit(ctx context.Context, mutation Mutation) error
}

type Notifier interface {
    Notify(ctx context.Context, notification Notification) error
}
~~~

Within sync, the reporter consumes a runner seam with one method per half plus
the enrichment pass, rather than one method taking a phase argument. The halves
differ in what they contact and what they may not do, so the type system, not a
switch statement, is what keeps a caller from asking for a half that does not
exist.

The schedule package is unchanged by the split: it still starts one run per tick
through a one-method interface. Which halves that run performs is a policy the
sync package owns, because it is the package that knows what a half costs and
what it may not skip.

The OAuth package owns separate, narrow Wahoo and state interfaces for
authorisation state. It does not reuse the sync interfaces merely because SQLite
and Wahoo implement both sets of behaviour.

The schedule package owns a one-method Runner interface so its tests can observe
triggering without starting a sync. The HTTP package depends on concrete sync
and OAuth application services unless a test requires a smaller local interface.
It also declares a two-method `Assets` interface for serving the browser bundle,
so it stays independent of how that bundle is built or embedded.

Read models that cross the persistence boundary — the stage summary served by
the routes endpoints — are declared in `route`, not exported from `sqlite`. That
keeps the arrow one-way: `httpapi` and `sqlite` share a value vocabulary without
`httpapi` importing an adapter.

## Browser UI composition

The TypeScript application is component-driven so later features extend it
rather than rewrite it:

| Directory | Owns |
| --- | --- |
| `src/api` | the typed client, query definitions, and response validation |
| `src/components` | reusable presentational pieces with no feature knowledge |
| `src/features` | feature-scoped composition, one directory per area |
| `src/lib` | formatting and other shared helpers |

`components` must not import from `features`. Each feature owns its own data
fetching. `src/api` is the single place that knows URL shapes, and it validates
every response so a drift from the Go DTOs fails at the boundary with a clear
error rather than surfacing as `undefined` inside a component.

Concrete adapters do not export speculative interfaces. In particular, wahoo
exports concrete clients, sqlite exports a concrete Store, and pushover exports
a concrete Client. Compile-time satisfaction checks belong next to adapter types
only when an implementation intentionally imports the consumer interface
without creating an import cycle; ordinary constructor assignment is sufficient.

## Dependency direction

~~~mermaid
flowchart LR
    Main["cmd/domestique"] --> Config
    Main --> HTTP
    Main --> Schedule
    Main --> Sync
    Main --> OAuth

    HTTP["httpapi"] --> Sync["sync"]
    HTTP --> OAuth["oauth"]
    Schedule["schedule"] --> Sync

    Main --> WebUI["webui"]

    Sync --> Route["route"]

    VP["veloplanner"] --> Route
    FIT["fit"] --> Route

    Main --> VP
    Main --> FIT
    Main --> Wahoo["wahoo"]
    Main --> SQLite["sqlite"]
    Main --> Pushover["pushover"]
~~~

Only main imports an application use case and its concrete adapters together.
No adapter imports sync, oauth, schedule, or httpapi. This keeps dependency
arrows one-way and prevents adapter-to-adapter coupling.

## Composition and lifecycle

Main performs the following in order:

1. Load and validate configuration, including static secret inputs.
2. Open SQLite, run migrations, and create the concrete Store.
3. Construct VeloPlanner, FIT, Wahoo, and Pushover clients with explicit HTTP
   timeouts and the shared structured logger.
4. Construct concrete sync and OAuth services from their consumer interfaces.
5. Construct the scheduler and Tailnet-gated HTTP handler.
6. Start the HTTP server and scheduler under one signal-derived context.
7. On cancellation, stop scheduling new runs, let bounded in-flight work observe
   context cancellation, shut down HTTP, and close SQLite.

No constructor starts a goroutine, reads global configuration, calls an upstream,
or invokes log.Fatal. Startup errors return to main; main alone decides the
process exit code.

## Data and boundary rules

- All external calls accept a context supplied by the caller and use a bounded
  timeout owned by the adapter.
- Route values are immutable after validation. SQLite snapshots and mutations are
  copied or treated as immutable across package boundaries.
- Sensitive values use unexported fields or dedicated types and never receive
  JSON tags. HTTP response structs are separate from persistence and adapter
  structs.
- Every JSON field is explicitly tagged. No SQLite or upstream response struct
  is marshalled directly.
- The scheduler, HTTP server, and SQLite Store are pointer types and must not be
  copied after construction.
- Package-level state is limited to immutable constants and precompiled regular
  expressions. There are no mutable globals or init functions.

## Testing ownership

Tests live with the package under test:

| Package | Primary test style |
| --- | --- |
| route and fit | table-driven unit tests plus FIT decode validation |
| sync and oauth | in-memory fakes for their local consumer interfaces |
| veloplanner and wahoo | httptest servers with malformed, rate-limit, and retry cases |
| sqlite | temporary database and migration/recovery tests |
| httpapi | handler tests for identity on every route, JSON shape, safe errors, and the security and cache headers |
| webui | serving the embedded bundle, and reporting an unbuilt one |
| webui/app | Vitest and Testing Library over reusable components and the API client's parsing and error paths |
| schedule | deterministic clock or trigger seam, no wall-clock sleeping |
| pushover | HTTP request shape and redaction tests |

No normal test contacts VeloPlanner, Wahoo, Pushover, Tailscale, or a secret
provider. The sandbox FIT/Wahoo acceptance test is separate from normal CI.

## Delivery sequence

Implementation proceeds in these reviewable layers:

1. Bootstrap the module and developer quality gate: CGO-free Go toolchain,
   Make targets, golangci-lint, prek, GitHub Actions, MIT licence, and
   repository hygiene.
2. Implement configuration, route values, secret input, SQLite migrations, and
   focused unit tests.
3. Port and harden the VeloPlanner adapter; implement FIT encoding and a
   sandbox-only acceptance harness.
4. Implement Wahoo OAuth, encrypted token state, direct route operations, and
   rate-limit handling.
5. Implement sync, scheduler, Pushover, Tailnet-gated read-only HTTP API, and
   full safety regression tests.
6. Add Docker, macOS MVP and Pi deployment configuration, signed GHCR release
   delivery, and end-to-end sandbox verification.

A later layer may depend on earlier code, but it may not weaken a safety rule
already specified. Route preview remains a future addition after this sequence.

## Definition of architectural compliance

A change complies with this specification when:

- cmd/domestique is the only composition root and contains no business logic;
- every package has one owner responsibility and no import cycle;
- every consumer interface has a real boundary or test-double reason;
- no interface is shared merely for naming convenience;
- application code stays CGO-free and provider-agnostic;
- all normal external effects are reachable through injected dependencies; and
- code, local hooks, and GitHub CI enforce the same quality checks.
