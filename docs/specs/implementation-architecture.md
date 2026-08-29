# Domestique implementation architecture specification

**Status:** accepted

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

Manual constructor injection is final. There is no Wire, Dig, Fx, Do, service
locator, global registry, or package initialisation with side effects.

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
│   ├── route/                      route value types and invariants
│   ├── sync/                       reconciliation use case and its interfaces
│   ├── oauth/                      Wahoo OAuth use case and its interfaces
│   ├── schedule/                   delayed-start and hourly execution
│   ├── httpapi/                    Tailnet-gated handlers and JSON mapping
│   ├── readiness/                  loopback readiness probe, local state only
│   ├── elevation/                  device-export elevation normalization
│   ├── surface/                    OSM surface classification and snapping
│   ├── osmindex/                   OSM extract download, index build, schedule
│   ├── veloplanner/                VeloPlanner HTTP source adapter
│   ├── komoot/                     Komoot HTTP source adapter
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
as one artefact and the API and UI cannot drift apart in a deployment.
`internal/webui/app/dist` is build output; only its placeholder is tracked.

There is no public pkg directory, internal/common package, interfaces package,
models package, or generic repository package. A package is added only when it
owns a distinct responsibility in this tree.

## Package responsibilities

| Package | Owns | Must not own |
| --- | --- | --- |
| config | TOML and environment layering, the state key's file-secret resolution, validation of the file's own fields | HTTP clients, business decisions, provider-specific secret syntax, anything an operator edits while the service runs |
| runtimeconfig | the settings and credentials held in the database: their types, the rules both the write path and startup check them against, and the live snapshot readers copy from | SQL, HTTP routing, the file's fields, and any decision made *from* a setting |
| route | route identity — including which provider issued it — geometry, revision, and validation types | SQL, HTTP, FIT, Wahoo details |
| sync | inventory reconciliation, deletion gates, target progress, aggregate run result, per-target run result | HTTP handlers, SQL queries, Wahoo URLs |
| oauth | one-time callback state, target onboarding, duplicate-account rejection | HTTP routing, SQL queries, Wahoo URL formatting |
| schedule | startup delay, hourly cadence, no-overlap guard, cancellation | sync decisions or notification content |
| httpapi | Tailnet identity gate, routing, request parsing, JSON status and error mapping, per-target convergence derived from stored revisions, serving and writing the runtime settings, response security and cache headers | OAuth exchange, sync logic, how a course is encoded, or how the UI is built |
| readiness | the loopback readiness probe: whether local configuration and state are usable | any upstream call, identity, routing of the served surface, or authorisation state |
| webui | the embedded browser bundle and serving it; the TypeScript application | HTTP routing, identity, or any knowledge of persistence |
| elevation | sampling and median-filtering the exported elevation profile | source fetching, storage, FIT bytes |
| surface | OSM surface and tracktype classification, snapping a route to the ways under it, caching policy | SQL, HTTP routing, what the UI draws, where the ways come from |
| osmindex | downloading regional OSM extracts, packing them into a cell-partitioned surface index, the rebuild schedule, serving the ways near a route | classification rules, SQL of the state store, what a route is |
| ridemodel | the hybrid coefficient type and its loading, the forward model — fixed physics averaged with a rides-calibrated route correction — that turns a route's geometry into a predicted moving time, caching that prediction against geometry, surface, and coefficient fingerprints | calibrating the route correction from a ride corpus (`dev/fitter`'s job), SQL, HTTP routing, how a route's surface is classified |
| veloplanner | login, listing, detail decoding, route conversion | SQLite and Wahoo concerns |
| komoot | login, listing, detail decoding, route conversion | SQLite and Wahoo concerns |
| fit | deterministic FIT bytes for one valid route | VeloPlanner or Komoot requests, OAuth, HTTP |
| wahoo | authorisation URL, exchange, refresh, user lookup, FIT route operations, rate headers | route-source parsing, SQLite queries, Pushover |
| sqlite | migrations, encrypted token storage, snapshots and commits | Wahoo, VeloPlanner, or Komoot HTTP |
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
    Encode(ctx context.Context, route route.Route) ([]byte, error)
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

`Source.Inventory` returns routes already carrying their own `route.Provider`,
so a second provider is a new adapter satisfying this same interface with its
own provider value — not a change to the interface, the identity type, or any
package that consumes a `route.Key`.

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
so it stays independent of how that bundle is built or embedded, and a
two-method settings seam — read the live values, replace them whole — satisfied
by `runtimeconfig.Current`. The handler reads through that seam on every request
rather than holding a copy, because an edit has to reach the page's
configuration and the Content-Security-Policy header at once.

`runtimeconfig` declares its own one-pair store interface for the same reason,
satisfied by `*sqlite.Store`: the settings package owns the rules, and the
adapter owns the rows.

Read models that cross the persistence boundary — the route summary served by
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

Ordinary action buttons come from the shared `components/Button` rather than
being restyled per feature, and reworked component styles are co-located CSS
Modules. `src/index.css` keeps the theme tokens, the reset, the MapLibre
integration, and the feature styles not yet moved.

Concrete adapters do not export speculative interfaces. In particular, wahoo
exports concrete clients, sqlite exports a concrete Store, and pushover exports
a concrete Client. Compile-time satisfaction checks belong next to adapter types
only when an implementation intentionally imports the consumer interface
without creating an import cycle; ordinary constructor assignment is sufficient.

## Tier ownership

Package ownership settles where a computation lives within a tier. This settles
which tier it belongs to.

An answer that depends only on the route belongs to Go. It is the same for every
viewer, every session and every device, so it can be computed once and stored:
route distance, ascent and maximum gradient in `Summary`; surface classification
in `surface.Match`, which needs the OpenStreetMap index; the elevation
normalisation the FIT encoder exports.

An answer that depends on the person reading belongs to the browser. There is no
single answer to store, because it differs by reader, by pointer position or by
preference: the gradient bands and the sampled profile; `surfaceKindAt`, which
answers what the cursor is over; the library's text filter; the theme and unit
preferences kept per browser.

Note that `surface` determines what the ground is and `lib/surface.ts` presents
what was determined. The same subject divides cleanly along this line, and so
does gradient: Go computes the stored maximum, the browser computes the bands it
is drawn in.

The rule reads a computation's inputs, not the shape of its output. A function
returning coordinates still belongs to the browser when which coordinates it
returns depends on the reader.

Two conditions override it:

- **The boundary pulls work backwards.** Work needing a credential, or that
  would add a host the page itself reaches, belongs to Go however
  reader-dependent it is. Adopting an outbound service to answer a per-reader
  question is a deliberate exception to this section, and the issue adopting it
  says so.
- **Interaction pulls work forwards.** Route-only arithmetic that has to run on
  every hover or drag frame is implemented in the browser as well.
  `haversineMetres` and cumulative distance therefore exist on both sides: the
  Go one feeds the stored `Summary`, the browser one feeds the live profile.
  Both use the same spherical model, so the axis agrees with the distance shown
  beside it. A second implementation without that agreement is a defect, not a
  mirror.

What crosses the boundary follows from the rule: route-shaped values do,
reader-shaped values do not. Coordinates, distances and timestamps are sent;
rider mass, sustained power, unit preference, theme and zoom are not, and no
endpoint accepts them.

### Predicted moving time is a deliberate departure

A route's predicted moving time — `internal/ridemodel`, computed once and
stored on `Summary` in the manner of surface classification — depends on rider
mass and sustained power, which by the rule above are reader-shaped and belong
to the browser. It is stored in Go instead, and the reason is worth recording
rather than leaving as a contradiction between this document and the code.

The rule sends reader-dependent work to the browser *because* there is no
single answer to store: an earlier proposal, #136, asked the operator to type
a flat-road speed, a mass, and a sustained power into `localStorage`, and a
different reader — or the same reader on a different device — would type
different ones. That premise does not hold once there is one configured
hybrid profile — fixed physical priors averaged with a route correction
calibrated once against a corpus of recorded rides ([#239](https://github.com/nobbs/domestique/issues/239))
— rather than a value each reader guesses: there is one operator, one profile,
and therefore exactly one answer per route, which is precisely the shape of
thing this section already sends to Go. The conclusion moves because the
premise does, not because it is being worked around.

The guarantee the rule exists to protect survives intact by a different
route: no endpoint accepts a rider-shaped value. The profile arrives as a
setting naming a file on the host — `ridemodel.coefficients_file` — never as a
request field, so
"rider mass, sustained power... are not [sent], and no endpoint accepts them"
remains true of every HTTP request this service answers. What changed is only
where the one answer the profile produces is computed and kept.

This supersedes #136's placement. A later reader finding both documents
should trust this one: the moving-time model's physics is substantially the
same forward solve and descent cap #136 specified. Two deliberate changes
came later: drag, rolling resistance and power are now settled reference
values rather than fitted per-deployment from coasting and climbing samples
the corpus could not reliably identify, and a two-parameter route correction
carries what the corpus can — see #239 and #240. Only where it runs and where
its inputs come from otherwise changed.

The same boundary holds for the profile's measured unseen-route error
([#217](https://github.com/nobbs/domestique/issues/217)): `internal/httpapi`
reads it once, straight off the loaded `ridemodel.Coefficients`, and attaches
it to every route response that carries a prediction. It is metadata about
the frozen profile, not a value this service computes at serve time —
`dev/fitter -recalibrate` is what measures it, the same way it measures
`seconds_per_km` and `seconds_per_ascent_m`.

## Dependency direction

~~~mermaid
flowchart LR
    Main["cmd/domestique"] --> Config
    Main --> RuntimeConfig["runtimeconfig"]
    Main --> HTTP
    Main --> Schedule
    Main --> Sync
    Main --> OAuth

    HTTP["httpapi"] --> Sync["sync"]
    HTTP --> OAuth["oauth"]
    Schedule["schedule"] --> Sync

    Main --> WebUI["webui"]
    Main --> Readiness["readiness"]

    Sync --> Route["route"]

    VP["veloplanner"] --> Route
    Komoot["komoot"] --> Route
    FIT["fit"] --> Route

    Main --> VP
    Main --> FIT
    Main --> Wahoo["wahoo"]
    Main --> SQLite["sqlite"]
    Main --> Pushover["pushover"]
~~~

Only main imports an application use case and its concrete adapters together.
The readiness probe has one dependency, the local state it reads, so nothing it
is given could reach a provider. No adapter imports sync, oauth, schedule,
httpapi, or readiness. This keeps dependency arrows one-way and prevents
adapter-to-adapter coupling.

## Composition and lifecycle

Main performs the following in order:

1. Load and validate the configuration file, including the state key input.
2. Open SQLite, run migrations, and create the concrete Store.
3. Load and validate the stored runtime settings and credentials into the live
   snapshot every component below reads through a function rather than a held
   value, so an edit reaches the next run or the next request instead of the
   next restart. Settings the migrations seeded are checked here by the rules
   that guard the write path, so a hand-edited database fails startup naming the
   setting.
4. Construct the clients whose configuration cannot change: FIT, Open-Meteo,
   and Pushover, the last reading its origin and credentials through the
   snapshot on each send.

   The source and Wahoo clients are not among them. They are built from the
   snapshot per run, by providers `main` owns, because everything they need is
   an editable setting: the Wahoo client is rebuilt only when those settings
   actually change, since it carries the request budget observed from Wahoo's
   own responses and a fresh one would spend real requests rediscovering it.
   A provider whose settings are not filled in yet builds nothing and reports
   the run not ready — which is what a service nobody has configured does, and
   is not a startup failure.
5. Construct concrete sync and OAuth services from their consumer interfaces.
6. Construct the scheduler and Tailnet-gated HTTP handler.
7. Start the HTTP server and scheduler under one signal-derived context.
8. On cancellation, stop scheduling new runs, let bounded in-flight work observe
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
| komoot | httptest servers with malformed listing, pagination, and authentication-failure cases, and an assertion that no request uses a method other than GET |
| sqlite | temporary database and migration/recovery tests |
| httpapi | handler tests for identity on every route, JSON shape, safe errors, and the security and cache headers |
| readiness | handler tests for the ready, unreadable-state, and incomplete-state answers, and container tests that the image, the compose example, and the deploy gate still name the probe's own port |
| webui | serving the embedded bundle, and reporting an unbuilt one |
| webui/app | Vitest and Testing Library over reusable components and the API client's parsing and error paths |
| schedule | deterministic clock or trigger seam, no wall-clock sleeping |
| pushover | HTTP request shape and redaction tests |

No normal test contacts VeloPlanner, Komoot, Wahoo, Pushover, Tailscale, or a
secret provider. The sandbox FIT/Wahoo acceptance test is separate from normal
CI.

## Definition of architectural compliance

A change may depend on existing code, but it may not weaken a safety rule
already specified. It complies with this specification when:

- cmd/domestique is the only composition root and contains no business logic;
- every package has one owner responsibility and no import cycle;
- every consumer interface has a real boundary or test-double reason;
- no interface is shared merely for naming convenience;
- application code stays CGO-free and provider-agnostic;
- all normal external effects are reachable through injected dependencies; and
- code, local hooks, and GitHub CI enforce the same quality checks.
