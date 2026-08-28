# Backend layout

Where the Go tree has drifted from [the architecture
specification](specs/implementation-architecture.md), what is oversized, and
the shape `internal/` should take instead.

**Nothing here has been applied.** This is a survey and a proposal, in the same
shape as [naming-drift.md](naming-drift.md): the work is listed so a later
change can execute it deliberately rather than one file at a time as each is
noticed.

Every claim carries the file it was read from. Sizes and counts are from the
commit this was written against and will drift; the shapes will not.

## The specification's tree is out of date in both directions

[implementation-architecture.md](specs/implementation-architecture.md) prints a
normative tree containing `cmd/`, `internal/`, `docs/` and `testdata/`. The
repository has `api/`, `cmd/`, `deploy/`, `dev/`, `docs/` and `internal/`.

| Directory | In the spec tree | On disk |
| --- | --- | --- |
| `api/` | no | yes — the OpenAPI document both sides generate from |
| `deploy/` | no | yes |
| `dev/` | no | yes — six `main` packages |
| `testdata/` | yes | no |

`dev/` is the largest omission: `demoapi`, `fitter`, `gatecheck`,
`patchcoverage`, `coveragesummary` and `ridemodel`. The specification knows they
exist — it names `dev/fitter` twice in prose, at lines 84 and 282 — but its
layout section does not, so the tree that is normative describes a repository
that has not existed for some time.

**Proposed:** the tree gains `api/`, `deploy/` and `dev/`, and loses
`testdata/`. This is a specification revision, which AGENTS.md says must be
deliberate and called out rather than quietly matched to the code.

## 1. The packages are not what is flat

`internal/` holds 23 packages, and the import fan-in is what the specification's
ports-and-adapters description predicts rather than a flat pile:

| Package | Imported by | Reading |
| --- | --- | --- |
| `route` | 47 files | the domain core, as specified |
| `surface` | 17 | classification is used almost everywhere geometry is |
| `runtimeconfig` | 10 | the live settings snapshot |
| `sqlite` | 4 | one adapter behind consumer-side interfaces |
| `httpapi` | 3 | delivery, near the composition root |
| everything else | 0–2 | one owner each |

`demo` reads as zero from inside `internal/`; it is imported by
`dev/demoapi/main.go`, outside both `internal/` and `cmd/`.

That distribution is the shape the specification asks for, and
[implementation-architecture.md:64](specs/implementation-architecture.md) makes
the flatness explicit: "There is no public pkg directory, internal/common
package, interfaces package, models package, or generic repository package. A
package is added only when it owns a distinct responsibility in this tree."

So the packages are flat by contract, not by neglect. The flatness worth fixing
is one level down, inside them.

## 2. The files are what is flat

| File | Lines | What it holds |
| --- | --- | --- |
| `internal/sqlite/store.go` | 3003 | 78 functions — the entire durable-state adapter |
| `internal/sync/service.go` | 951 | 13 types, 7 interfaces, the passes, and their helpers |
| `internal/httpapi/handler.go` | 875 | 11 consumer-side interfaces, `Options`, `Handler`, the mux |
| `internal/wahoo/client.go` | 741 | OAuth, route writes, transport, rate limiting, geometry maths |
| `internal/config/config.go` | 514 | 19 functions |
| `internal/route/stage.go` | 400 | the whole package |

`store.go` alone is a sixth of the non-test backend.

### `internal/sqlite/store.go`

The seams are already there in the method order; the file just never took them.
Reading the declarations top to bottom, the groups are:

| Proposed file | Methods |
| --- | --- |
| `store.go` | `Open`, `Close`, `configure`, `migrate`, `schemaMigrations`, `closeDatabase`, `closeRows`, `rollback` |
| `crypto.go` | `encrypt`, `decrypt` |
| `targets.go` | `EnsureTargets`, `Targets`, `ForEachTarget`, `Target`, `validateTargetIDs` |
| `authorization.go` | `TargetAuthorization`, `AuthorizeTarget`, `RefreshToken`, `ReplaceRefreshToken`, `MarkNeedsReauthorization`, `BeginAuthorization`, `ConsumeAuthorization`, `ForEachPendingAuthorization` |
| `stages.go` | `ForEachSourceStage`, `ForEachStageSummary`, `StageGeometry`, `storeStageGeometry`, `RequestStageReprocess`, `requestedReprocessing`, `decodeCoordinates`, `encodeCoordinates` |
| `surface.go` | `StageSurface`, `StageSurfaceHash`, `StoreStageSurface`, `pruneStageSurface`, `SurfaceCoverage`, `SurfaceIndexBuild`, `RecordSurfaceIndexBuild` |
| `duration.go` | `StageDurationFingerprint`, `StoreStageDuration`, `PruneStageDurationsWithDifferentFingerprint`, `pruneStageDuration` |
| `runs.go` | `LastSyncRun`, `ForEachPhaseRun`, `ForEachSyncRun`, `lastSyncRunID`, `RecordSyncRun`, `newSyncRunReference`, `pruneSyncRuns`, `RecordTargetRun`, `ForEachTargetRun`, `LastPhaseOutcome`, `LastSuccessfulPhaseCompletion`, `ForEachSuccessfulRunAfter` |
| `notifications.go` | `LastFailureNotification`, `RecordFailureNotification`, `LastDigestNotification`, `RecordDigestNotification` |
| `inventory.go` | `TrustedInventory`, `TrustedInventoryCount`, `StoreTrustedInventory`, `ForEachTargetStage`, `UpsertTargetStage`, `DeleteTargetStage` |
| `settings.go` | `RuntimeSettings`, `SetRuntimeSettings`, `RuntimeSecrets`, `SetRuntimeSecrets`, `runtimeBasemaps`, `runtimeTargets`, `runtimeSources`, `runtimeSurfaceRegions`, `SyncSchedule`, `SetSyncSchedule` |

No signature changes and no package boundary crossed: this is one file becoming
eleven in place, which is why it can be reviewed at all. `store_test.go` should
be cut along the same lines.

### `internal/httpapi/handler.go`

Most of it is not a handler. Eleven consumer-side interfaces — `OAuth`, `Sync`,
`Assets`, `State`, `TargetState`, `StageState`, `RunState`, `SettingsState`,
`ScheduleState`, `AccessVerifier`, `Weather` — sit above `Options`, `Handler`,
`New` and the mux registration.

**Proposed:** the interfaces move to `ports.go`, leaving `handler.go` as the
type, its options and its routing. The specification's rule that "interfaces
appear only in the consuming package where tests or a real adapter boundary need
substitution" is why they live here; it does not ask them to share a file with
the router.

### `internal/sync/service.go`

Same shape: seven interfaces (`Source`, `Encoder`, `Processor`, `Target`,
`Annotator`, `Predictor`, `State`) and six result types (`Phase`, `Outcome`,
`FailureCategory`, `TargetResult`, `SourceResult`, `Result`) ahead of the
service itself.

**Proposed:** `ports.go` for the interfaces, `result.go` for the outcome and
result types, `inventory.go` for `normalizeInventory`, `missingStages`,
`targetStage` and `counts`; `service.go` keeps `Service`, `New` and the passes.

### `internal/wahoo/client.go`

Five concerns in one file: OAuth (`AuthorizationURL`,
`ExchangeAuthorizationCode`, `RefreshAccessToken`, `oauthContext`, `tokenPair`,
`classifyTokenError`, `parseCallbackURL`), route writes (`CreateRoute`,
`UpdateRoute`, `ListOwnedRoutes`, `DeleteOwnedRoutes`, `DeleteRoute`,
`writeRoute`), transport (`New`, `RoundTrip`, `newRequest`, `doJSON`,
`endpoint`, `parseOrigin`), rate limiting (`observeRateLimit`, `RateLimit`,
`rateLimit`, `lowestRateLimit`, `withQuotaPatience`, `waitBudget`, `waitFor`)
and geometry maths (`calculateMetrics`, `haversine`, `formatFloat`).

**Proposed:** `oauth.go`, `routes.go`, `transport.go`, `ratelimit.go`, and the
maths into `metrics.go` — or deleted in favour of an existing helper, since
`haversine` is unlikely to be the only one in the tree.

## 3. Grouping `internal/`

`internal/` becomes four groups by role, with four packages left flat.

~~~text
internal/
├── core/            route, sync, oauth, schedule,
│                    elevation, surface, ridemodel, readiness
├── upstream/        veloplanner, komoot, wahoo,
│                    openmeteo, pushover, osmindex
├── adapter/         sqlite, fit, cfaccess
├── serve/           httpapi, webui
├── config/          runtimeconfig/
└── build/           demo/
~~~

### The membership rules

Each group needs a rule crisp enough that the next package has a home without a
debate:

- **`core/`** — owns a rule about routes or synchronisation and performs no I/O
  of its own. This is the set the specification already describes as
  "independent of HTTP, SQLite, and upstream protocols".
- **`upstream/`** — makes outbound calls to somebody else's service. Six
  packages, and the rule admits no argument about any of them.
- **`adapter/`** — wraps infrastructure this service depends on but does not
  own: the database, the FIT encoding, the identity check.
- **`serve/`** — what the service exposes to a caller.

`config`, `runtimeconfig`, `build` and `demo` stay flat. They are the residue:
configuration is read by every layer and belongs to none, and `build` and `demo`
are not product code at all. A two-member group named for what its members are
*not* would read worse than four flat entries.

### What the specification must say first

Three edits to
[implementation-architecture.md](specs/implementation-architecture.md), in the
same change as the move and not after it:

1. The directory tree, replaced with the one above — including the `api/`,
   `deploy/` and `dev/` corrections from the top of this document.
2. The sentence at line 64. As written it forbids this grouping outright, and
   its purpose — keeping out `common`, `models` and `repository` packages that
   own nothing — survives a rewrite that permits grouping by role while still
   refusing grouping by nothing.
3. The responsibility table, which covers 19 packages and omits four:
   `build`, `cfaccess`, `demo` and `openmeteo`. `cfaccess` is the identity gate
   and its absence from the normative table is the more serious of the four.

### What it costs

The Go half is mechanical and safe: **61 of 147 files** import at least one
moved package, across **110 import lines**. A rename tool plus `goimports`
handles it, and the compiler proves the result.

The non-Go half is not evenly spread, and one package carries almost all of it:

| Package | Non-Go references |
| --- | --- |
| `webui` | 86, across 11 files |
| `httpapi` | 5 |
| `route` | 1 |
| `sqlite` | 0 |

`internal/webui` is named in `.github/workflows/ci.yml`,
`.github/paths-filter.yml`, `codecov.yml`, `.gitleaks.toml`, `.mise.toml`,
`mise-tasks.toml`, `playwright.config.ts`, the e2e fixtures, `AGENTS.md`, and
both [delivery.md](specs/delivery.md) and
[implementation-architecture.md](specs/implementation-architecture.md). Nothing
there is hard, but a missed path in `paths-filter.yml` or `codecov.yml` fails
open — the job simply stops running against those files — so it is the one move
that cannot be proved by compiling.

That is why `serve/webui` is sequenced last and alone. Every other package in
the tree moves under compiler protection.

## Suggested order

1. Revise the specification's tree to match the repository (`api/`, `deploy/`,
   `dev/` in; `testdata/` out). It is wrong today regardless of anything else
   here, and every later item is read against it.
2. `internal/sqlite/store.go` and its test. Largest gain, no boundary crossed,
   and it is the file that makes the package unreviewable today.
3. `handler.go`, `service.go` and `client.go` — the interface-and-type headers
   move out of each. Independently reviewable, one package at a time.
4. `config.go` and `route/stage.go` if they still read long afterwards.
5. The specification revision from section 3 — tree, the line-64 rule, and the
   four missing table rows — followed by the moves under compiler protection:
   `core/`, `upstream/`, `adapter/`, and `serve/httpapi`.
6. `serve/webui` last and on its own, with the eleven non-Go files it is named
   in. It is the only move whose correctness the compiler cannot check.
