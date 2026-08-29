# Naming drift

Where the code disagrees with [the glossary](glossary.md), why, and what the fix
would be.

**What remains here has not been applied.** An entry is deleted as its rename
lands, so this file is the outstanding gap rather than a history — the reasoning
for an applied one is in the pull request that applied it. The renames are
listed so a later change can execute them deliberately, in an order that keeps
the wire, the database and the interface in step, rather than one file at a time
as each is noticed.

The frontend-only renames (items 3, 5, 6, 7, 8, 9, 13 and the smaller drift)
and the interface wording (4, 10, 12) have been applied and are gone from
here.

Every claim below carries the file it was read from. Line numbers are from the
commit this document was written against and will drift; the identifiers will
not.

## 1. `route` names both a whole and its part

This is the root cause, and most of what follows is downstream of it.

The API schema `Route` (`api/openapi.yaml`) is one *stage* of a source route.
The `routeId` inside it identifies the source route — a different, larger thing,
also called a route. So `Route.routeId` is not the id of that `Route`.

The consequences are visible in every layer:

| Where | What it says |
| --- | --- |
| `api/openapi.yaml` | `RouteList` has one field, and it is `stages` |
| `internal/webui/app/src/api/queries.ts` | destructures that field into a variable named `routes` |
| `internal/route/` | one file, `stage.go` |
| `internal/httpapi/routes_stages.go` | "routes" meaning HTTP routes and "stages" meaning cycling stages, in one filename |
| `internal/webui/app/src/lib/seenStages.ts` | types the same object as `Stage`, with a `localStorage` key `domestique.seen-stages` |
| `internal/webui/app/src/lib/sourceRoute.ts` | "the way back from a stored **stage** to the **route** it was made from" |
| every user-facing string | "route" — "Search 42 routes", "Open route", "Route library" |

**Proposed:** the unit stays `Route`, matching what the interface says and what
a reader calls it. The provider's parent becomes `sourceRoute`.

| From | To |
| --- | --- |
| `Route.routeId` | `Route.sourceRouteId` |
| `Route.routeName` | `Route.sourceRouteName` |
| `Route.stageName` | `Route.routeName` |
| `RouteList.stages` | `RouteList.routes` |
| `Stage` (`lib/seenStages.ts`) | `Route` |
| `StageChange`, `seenStages` | `RouteChange`, `seenRoutes` |
| `internal/route/stage.go` | `internal/route/route.go` |
| `internal/httpapi/routes_stages.go` | split by verb — see below |

`routeName` and `stageName` are the same whole-and-part pair as `routeId` and
`stageOrder`: `routeName` is the *source route's* name and `stageName` is the
piece's. So that pair is a swap rather than a rename — `routeName` means one
thing before and the other after — and it has to move in the same pass as
`routeId`, or the schema spends the interval carrying a `sourceRouteId` beside a
`routeName` that still means the source route.

`stageOrder` keeps its name: it is an ordinal within a source route, which is
genuinely a stage concept.

`routes_stages.go` is split rather than renamed. `routes_routes.go` would trade
a confusing name for a stuttering one that still says nothing about the
contents, and the sibling files — `routes_oauth.go`, `routes_settings.go`,
`routes_status.go`, `routes_weather.go` — are already named by subject. The
unambiguous cut is the five `RedirectLegacy*` handlers, about a third of the
file, which become `routes_redirects.go`. What remains — `GetRoutes`,
`GetRoute`, `GetRouteGeometry`, `ReprocessRoute` and their helpers — becomes
`routes_library.go`, after the glossary's word for the stored collection. Not
after item 13's *atlas*: that is the browser page's word, and the API has no
atlas.

This one crosses the wire, so it is the only item here that needs a coordinated
change: `api/openapi.yaml`, the Go handlers, `pnpm api:generate`, the
`localStorage` key (a deliberate reset, not a migration — see below), and
[`docs/specs`](specs). *Stage* is the word in the accepted contracts too — most
of all [service.md](specs/service.md), whose "Each VeloPlanner route stage is a
separate Wahoo route" is a contract sentence rather than a comment, and
[sync-lifecycle.md](specs/sync-lifecycle.md), which uses it throughout. Those
specifications are normative, so revising them is part of this change and not a
follow-up to it.

The `domestique.seen-stages` value is reset rather than migrated. The cost is
not free and should be taken deliberately: `seenStages.ts` reads a key it has
never seen as `"new"`, and `markSeen` runs only for the route the reader has
opened (`AtlasPage.tsx`), so a reset badges the whole library at once and
each badge clears only when that route is opened. Migrating instead would be
about five lines — the stored map is `Record<routeKey, sourceRevision>` and
`routeKey()` is built from field *values*, so the rename leaves it
byte-identical — which is the trade being declined, not one that was missed.

## 2. Seven names for one string

`routeKey()` (`src/api/types.ts`) produces `provider/routeId/stageOrder`. The
same value is then held as `pickedKey`, `openKey`, `focusKey`, `accentKey`,
`hoveredKey`, `inertKey` — and, one prop hop later, as `stageKey`
(`AtlasPage.tsx` passes `stageKey={openKey ?? undefined}` into
`RouteProfile.tsx`).

The qualified names are fine and worth keeping; they say *which* route. The
outlier is `stageKey`, which renames the concept mid-flight.

**Proposed:** `stageKey` → `routeKey`.

## 11. Phases are called halves in prose

`SyncPhase` is `"source" | "targets"` — singular for one, plural for the other —
while the prose around it calls a phase a *half*. No identifier uses "half",
and the comments mix the two: `internal/sync/reporter.go:61` reads "phase names
the half being run right now", and `:277` reads "The phase is a parameter so
each call".

**Proposed:** *phase* in both the comments and the identifiers. The enum keeps
`"source" | "targets"`: the asymmetry reflects the domain — one configured
source section, up to two target slots — and `SyncPhases` holds exactly one
`SyncPhaseRun` for each, so nothing reads the plural as a count. Making it
grammatically consistent would break `api/openapi.yaml`, `SYNC_PHASES` and Go's
`PhaseSource`/`PhaseTargets` for a cosmetic gain.

## 14. `Route` collides with react-router

`react-router` exports `Route` and `Routes`; `api/types.ts` exports a domain
`Route`. No file imports both today, but `features/routes/` is named for the
domain concept while `App.tsx`'s `<Routes>` is the router's.

**Proposed:** nothing. The collision is real but inert, and renaming either side
costs more than it saves. Noted so the next person to hit it knows it was
considered.

## Suggested order

1. Item 11, which is a prose fix in Go comments and touches nothing else.
2. `route` versus `source route` (1, 2) — last, because it crosses
   `api/openapi.yaml`, the Go handlers, a `localStorage` key and the normative
   specifications.
