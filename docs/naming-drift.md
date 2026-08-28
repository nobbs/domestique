# Naming drift

Where the code disagrees with [the glossary](glossary.md), why, and what the fix
would be.

**Nothing here has been applied.** This is a survey and a proposal. The renames
are listed so a later change can execute them deliberately, in an order that
keeps the wire, the database and the interface in step, rather than one file at
a time as each is noticed.

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
opened (`RoutesPage.tsx:296`), so a reset badges the whole library at once and
each badge clears only when that route is opened. Migrating instead would be
about five lines — the stored map is `Record<routeKey, sourceRevision>` and
`routeKey()` is built from field *values*, so the rename leaves it
byte-identical — which is the trade being declined, not one that was missed.

## 2. Seven names for one string

`routeKey()` (`src/api/types.ts`) produces `provider/routeId/stageOrder`. The
same value is then held as `selectedKey`, `openKey`, `focusKey`, `accentKey`,
`hoveredKey`, `inertKey` — and, one prop hop later, as `stageKey`
(`RoutesPage.tsx:622` passes `stageKey={openKey ?? undefined}` into
`RouteProfile.tsx:76`).

The qualified names are fine and worth keeping; they say *which* route. The
outlier is `stageKey`, which renames the concept mid-flight.

**Proposed:** `stageKey` → `routeKey`.

## 3. `RouteKey` and `routeKey()` are unrelated

`routeKey()` is the identity string. `RouteKey`
(`src/components/route/RouteKey.tsx`) is the colour legend for gradient bands
and surface classes, which doubles as a filter. They differ only by leading
case and share no meaning. `KeyChip` is a legend swatch, not an identity chip.

**Proposed:** `RouteKey` → `RouteLegend`, `KeyChip` → `LegendChip`. Frontend
only; nothing crosses the wire.

## 4. `target` on the wire, "account" in the interface

The wire and the types say **target** throughout: `TargetStatus`, `TargetRun`,
`/v1/settings/targets`, `/v1/sync/targets/{target}`,
`/oauth/wahoo/start/{target}`.

The interface never says it. It says *account*: the card heading "What the
accounts hold", the settings section "Wahoo accounts", the field "Accounts, one
per line", "That account could not be reconciled."

`AccountRow` straddles the two — the component is named for the interface and
its prop (`AccountRow.tsx:68`) is `target: TargetStatus`.
`docs/specs/configuration.md` uses a third word, *slot*, from line 74 onward.

**Proposed:** *target* wins, because it is already the wire word and because the
credentials are a property of the target rather than the thing itself. Rename
the interface strings and `AccountRow` → `TargetRow`. See the glossary entry
for *account*, which stays a real word for the credentials on both sides.

*Slot* stays, and is a glossary entry rather than drift. It is not a loose third
spelling of *target*: `configuration.md` uses it for the configured list entry
as distinct from the durable record that entry creates — "Removing a slot from
the list keeps that record" — and no other word in the vocabulary draws that
line. It is configuration's word alone; the wire and the interface say
*target*.

## 5. `TargetConvergence` is both a component and a type

`src/api/types.ts:62` exports `type TargetConvergence = TargetStatusConvergence`
(`current | lagging | failed | unauthorized`).
`src/features/sync/TargetConvergence.tsx` exports a React component of the same
name. Importing both into one file requires renaming one at the import.

**Proposed:** the component becomes `TargetConvergenceCard`, matching the card
it renders. The type keeps the bare name.

## 6. `Library` means three things

1. The stored collection — `LibraryMap`, `LibraryRoutes`, `lib/library.ts`,
   `<h1>Route library`.
2. A remote source provider — `ServiceSettings.tsx:485` is literally
   `function Library({ provider, settings })`, and the OpenAPI documents
   `SourceSettings.baseUrl` as "the library's own web application".
3. MapLibre, imported as `maplibre` in `lib/maplibre.ts`.

Reading `ServiceSettings` you cannot tell from the name whether `Library`
configures the local collection or a remote service. It is the latter.

**Proposed:** `function Library` → `function SourceSettingsSection`, and the
OpenAPI description reworded to "the source's own web application". Meaning 1
keeps the word. Not `SourceLibrary`: that keeps *library* for a remote service,
which is the collision this item exists to remove.

## 7. `selection`, `window` and `highlight`

`lib/selection.ts` holds a stretch of route. `lib/mapSelection.ts` holds the
map-side gesture that produces the same `DistanceWindow` — two modules split by
*view* rather than by concept. Meanwhile `selectedKey` in `RoutesPage`,
`LibraryMap` and `LibraryRoutes` means *which route is picked in the list*,
which is a different thing entirely. `lib/highlight.ts`'s doc comment calls the
picked legend class "the selection" as well, though its type is `Highlight`.

`DistanceWindow` sits beside 236 uses of the browser's own `window`
(`window.matchMedia`, `refetchOnWindowFocus`) with nothing distinguishing them
at the identifier level.

**Proposed:** `selectedKey` → `pickedKey`, so *selection* means only the
distance stretch. Not `activeKey`, because `active` is already a `Button` prop,
and not `focusedKey`, which would collide both with DOM focus in a
keyboard-navigable list and with the existing `focusKey`.

Rename `mapSelection.ts` to `selectionGesture.ts` rather than folding it into
`selection.ts`. The two are split by view, but they are also split by kind:
`selection.ts` is 86 lines of pure geometry over `DistanceWindow`, while
`mapSelection.ts` is 325 lines of imperative MapLibre wiring — a `SelectableMap`
interface and a `routeSelection()` that returns a teardown. Folding them would
give the pure half a MapLibre dependency it does not have today.

Correct the `highlight.ts` doc comment. Keep `DistanceWindow` spelled in full
and never abbreviate it to `window`.

## 8. Four names for moving time

| Name | Where |
| --- | --- |
| `movingSeconds` | the wire, and the "Moving time" label |
| `rideSeconds` | `RouteProfile` and `StartTimePicker` props |
| `cumulativeSeconds` | `RouteGeometry`, per coordinate |
| `elapsedSeconds` | the parameter `cumulativeSeconds` is passed into, in `lib/forecastSamples.ts` |

`elapsedSecondsForWindow()` returns the same unit again, assigned to
`selectionMovingSeconds` and passed as `movingSecondsOverride`.

**Proposed:** `movingSeconds` everywhere for the scalar, `cumulativeSeconds` for
the per-coordinate accumulation, and delete `rideSeconds` and `elapsedSeconds`
as parameter names.

## 9. Ascent and climbs

`formatAscent()` formats `ascentMetres` and is rendered under the label
**"Climbing"** (`RoutePanel.tsx`, `RouteCard.tsx`). `lib/climbs.ts` and
`ClimbsList` mean named sustained ascents, rendered under **"Climbs"**. The doc
comment on `formatAscent` in `lib/format.ts` calls it "Total climbing" — using
the other concept's word in the one place that distinguishes them.

**Proposed:** the label becomes "Ascent"; "Climbs" stays for the list. Fix the
doc comment.

## 10. `sync` and `synchronisation`

The nav label is "Sync", the route is `/sync`, the Go package is
`internal/sync` — and the settings section on the adjacent screen is titled
"Synchronisation", with `lib/syncState.ts` documenting "the state of
synchronisation".

**Proposed:** "Sync" everywhere a reader sees it.

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

## 12. British identifiers over American values

`lib/targetAuthorisation.ts` (British) exports `TARGET_AUTHORISATIONS` whose
members are `"not_authorized"`, `"authorized"`, `"needs_reauthorization"`.
`TargetStatus.authorisation` is a British field whose sibling `convergence`
enum includes `"unauthorized"`. `ServiceSettings.tsx:472` shows the reader
"every authorization, route and recorded run" while `AccountRow` says
"authorisation".

**Proposed:** as the glossary says — British in prose and in identifiers this
project owns, American only where quoting a wire value. The one real fix is the
user-visible "authorization" in `ServiceSettings.tsx`.

## 13. The entry page has three competing names

| Layer | Name |
| --- | --- |
| component | `RoutesPage` |
| nav label | "Map" |
| Storybook | `Features/Route library` |

**Proposed:** the page is the **atlas** — the whole library drawn on one map.
`RoutesPage` → `AtlasPage`, the nav label becomes "Atlas", and the Storybook
title becomes `Features/Atlas/*`. The glossary gains the word.

Four further names on this page are *not* drift and keep their wording. The
visually hidden `<h1>` says "Route library" because a screen reader needs the
page's contents named rather than the widget that draws them — and *library* is
exactly the glossary's word for that collection. The drawer trigger "Browse
routes" names an action on the list inside the page. `GetIndex` serves the
application document at the root and, by its own doc comment, for the other
pages too, so it names the document rather than this page. The route `/` is a
path.

Related: `/routes/:provider/:routeId/:stage` is no longer a page. It redirects
to `/?route=…`, but the Go handler is still `GetRoutePage` and its doc comment
still says it "serves the application document for one stage's address".

## 14. `Route` collides with react-router

`react-router` exports `Route` and `Routes`; `api/types.ts` exports a domain
`Route`. No file imports both today, but `features/routes/` is named for the
domain concept while `App.tsx`'s `<Routes>` is the router's.

**Proposed:** nothing. The collision is real but inert, and renaming either side
costs more than it saves. Noted so the next person to hit it knows it was
considered.

## 15. Smaller drift

- **`PROVIDER_LABELS` is defined twice**, with identical contents, in
  `lib/provider.ts:10` and `features/settings/ServiceSettings.tsx:78`. The
  second should import the first — not quite a pure delete, since the types
  differ: `provider.ts` is a `Record<string, string>` behind a `providerLabel()`
  that falls back to the wire value, while `ServiceSettings` is a
  `Record<SourceProvider, string>` and exhaustive. Sharing one trades
  exhaustiveness for the fallback.
- **`components/route/` and `features/routes/`** both hold route UI with no
  stated rule for which belongs where — `RouteProfile` is in the feature
  directory, `ElevationProfile` in the component one. Meanwhile `LibraryRoutes`,
  which is library-specific, lives in `components/map/`. The rule to state:
  `components/` is presentational and reusable across pages, `features/` is
  page-bound composition and the data access feeding it. `LibraryRoutes` moves
  out of `components/map/` under that rule, and `RouteProfile` and
  `ElevationProfile` are each placed by the test rather than by where they
  happened to land.
- **`components.json` declares an alias `"hooks": "@/hooks"`** for a directory
  that does not exist. Ten `lib/` modules export a `use*` hook, but only three
  are hook-only files (`useEscapeKey`, `useInView`, `useElementWidth`); the
  other seven — `basemap`, `mediaQuery`, `overlayInsets`, `seenStages`,
  `startTime`, `units` and `theme` — export a hook beside non-hook helpers.
  Delete the alias rather than create the directory: honouring a shadcn default
  nothing uses is not worth splitting seven modules in half.
- **`lib/useInView.ts`'s doc comment** describes "the library grid". There is no
  grid; the library is a column of `ResultRow`s.
- **`PageShell.stories.tsx` and `RouteLibrary.stories.tsx`** have no
  `PageShell.tsx` or `RouteLibrary.tsx` beside them — `PageShell` is exported
  from `Layout.tsx`, and `RouteLibrary` is a name no component has. Name each
  story file for the component it renders: `RouteLibrary.stories.tsx` becomes
  `AtlasPage.stories.tsx`, following item 13, and `PageShell.stories.tsx`
  folds into the existing `Layout.stories.tsx`.
- **Storybook titles are three-way inconsistent** for one feature:
  `Features/Routes/*`, `Features/Route library`, `Features/Route Library/Map`.
  Item 13 settles the word, so they become `Features/Atlas/*`.
- **`components/Button.tsx` renames variants the primitive already names.**
  Of the six — `primary`, `standard`, `panel`, `ghost`, `danger`, `warning` —
  three are pure respellings: `primary`, `standard` and `danger` map straight
  onto shadcn's `default`, `outline` and `destructive` and add nothing but a
  second word. `ghost` already shares its name. `panel` and `warning` are the
  genuine additions: each routes through a primitive (`outline` and `ghost`) but
  layers its own classes over it, which no primitive variant offers. Aligning
  the three respellings would leave the wrapper purely additive — the icon slot,
  the size inferred from the children, the two extra variants and the link forms
  — rather than a naming layer as well. That is the stronger boundary, because
  "do not bypass the icon slot" is an invariant while "we spell it `standard`"
  is not. Align them: this is a proposal, not a note. The roughly 28 call sites
  are why it belongs here rather than in the refactor that drew the layer line,
  but unlike item 14 the cost buys a boundary that holds afterwards.

## Suggested order

1. The frontend-only renames (3, 5, 6, 7, 8, 9, 13, 15) — no wire change, and
   each is independently reviewable.
2. The interface wording (4, 10, 12) — user-visible strings, one pass.
3. `route` versus `source route` (1, 2) — last, because it crosses
   `api/openapi.yaml`, the Go handlers, a `localStorage` key and the normative
   specifications, and because settling it first would make every earlier rename
   conflict with it.
