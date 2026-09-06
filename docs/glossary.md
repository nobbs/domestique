# Glossary

The words this project uses, and the one meaning each of them has.

This file describes the vocabulary the code should use. Where the code
disagrees, [naming-drift.md](naming-drift.md) records the disagreement and what
the fix would be; an entry is deleted from it as the rename lands.

Definitions that carry contractual weight live in
[`docs/specs`](specs) and are linked from here rather than restated.

## The unit of the application

**route** — one ride's worth of geometry, the thing the library holds, the map
draws and a reader opens. It is identified by `provider`, `sourceRouteId` and
`stageOrder` together; no one of those is an identity on its own.

**source route** — the thing at the provider that a route was made from. A
VeloPlanner route may be divided into several ordered pieces, and each piece
becomes one route here. A Komoot tour has no such division, so it becomes
exactly one route (see [service.md](specs/service.md), "Source and course
representation").

The API type `Route` is the *piece*; the id inside it names the *whole*. The
whole is a **source route**, and the field is `sourceRouteId`.

**stage** — VeloPlanner's word for a source route's ordered pieces, and the
right word when talking about the provider or about storage. It is not the word
for the thing a reader opens; that is a route. `stageOrder` keeps its name as an
ordinal within a source route, which is a stage concept. The `:stage:` segment
of a Wahoo external ID is frozen: ownership is matched against it, and
respelling it would orphan the library.

**course** — the FIT-file rendering of a route, and only that. It belongs to the
encoder boundary and to Wahoo, never to the browser UI.

**ride** — what a person does with a route. It names the model and the reader's
own inputs (ride model, "Ride start"), never a stored entity and never a
duration: a duration is *moving time*.

**activity** — a ride a rider's device recorded and Wahoo holds, read back as a
summary and, later, its FIT record stream. It is stored against the target
whose account recorded it; Wahoo's own word for one, *workout*, stays inside
the `wahoo` adapter and never crosses into the service or the UI.

## Where routes come from and go

**provider** — the upstream service a route was read from: VeloPlanner or
Komoot. This is the word on the wire (`/v1/providers/{provider}/…`) and the word
in code.

**source** — the read half of synchronisation, and the settings that configure
it. "Source" is the role; "provider" is the identity. A sentence about *which*
service says provider; a sentence about *reading* says source.

**target** — a Wahoo destination that routes are written to. This is the word on
the wire, in types, and in the interface. It is not a *slot* and not, on its
own, an *account*. Belongs to exactly one subject — see *rider* — except a
target authorised before ownership existed, which belongs to none until an
admin assigns one by hand.

**slot** — a target's own identity: the value every stored authorisation,
target route, and recorded run is keyed by. For a self-service target this is
the same value as the owning subject's, not a name an operator chose (see
[configuration.md](specs/configuration.md)); a slot with no matching subject is
one authorised before ownership existed. This is configuration and storage's
word; the wire and the interface say *target*.

**account** — a set of credentials, and only that. A source has one (a library
is read with a login of its own) and a target has one (an authorised Wahoo
connection, belonging to whichever subject owns that target). "Account" alone
never identifies which side is meant: say *source account* or *target
account*, or name the side directly. The destination itself is a *target*, in
the interface as on the wire — "what the targets hold", not "what the accounts
hold".

## The library

**library** — the collection of routes this service holds. Nothing else. A
remote service is a *source* or a *provider*; MapLibre is *MapLibre*.

**atlas** — the entry page: the whole library drawn on one map, with one route
opened over it. This is the reader's word for that page and its nav label. What
the atlas draws is the *library*; what it draws on is a *map*. It is a browser
UI word only — the API has no atlas.

**catalogue** — the second view of the same library: every route written out as
a table, ranked by whatever the reader sorted on. The atlas answers *where does
this ride go*; the catalogue answers *which of these rides is the one I want*.
Its nav label and its path are both "catalogue", and like the atlas it is a
browser UI word only — the API has no catalogue, and the page asks it for
nothing the atlas does not already ask for.

## Identity and the map

**route key** — the string `provider/sourceRouteId/stageOrder` that identifies
one route. Produced by `routeKey()`. Every variable holding one is a *key*, and
the qualifier says which route it is: `openKey`, `hoveredKey`, `pickedKey`.
There is no *stage key*; it is the same string.

**legend** — the colour reference for gradient bands and surface classes, which
is also the control that filters by them. The component is `RouteLegend` and its
swatch is a `LegendChip`. A legend is not a key; `routeKey()` produces the other
thing entirely.

## Reading a route

**selection** — a stretch of the route the reader has picked out, expressed as a
`DistanceWindow`. Picking a *row* in the library is not a selection; that route
is *picked*, or *open*, or *hovered*, or *focused*.

**distance window** — a start and end distance along one route. Always written
in full; `window` on its own is the browser's.

**highlight** — a gradient band or surface class held above the others. Distinct
from a selection: a highlight is a class of ground, a selection is a stretch of
one route.

**moving time** — the predicted time in motion for a route, `movingSeconds` on
the wire. `cumulativeSeconds` is the same quantity accumulated per coordinate.
There is no separate "ride time" or "elapsed time"; those are the same thing
under other names.

**ascent** — total metres climbed over a route, shown as "Ascent".

**descent** — total metres lost over a route, `descentMetres` on the wire. The
route panel pairs it with ascent in one "Ascent" figure rather than giving it
a visible label of its own; an up/down arrow and a screen-reader-only word
tell the two apart.

**climb** — one named sustained ascent within a route, of the kind `ClimbsList`
enumerates. A route's total ascent is not a climb.

**weather grid** — Open-Meteo's spatial forecast files, relayed by
`/v1/weather-grid/*` and read straight into the map's own wind, temperature,
rain and cloud overlays over the whole viewport. Distinct from a route's own
forecast, which is a series of point readings along one route's geometry, not
a grid over an area.

## Synchronisation

**sync** — the process, spelled this way everywhere: the nav label, the route,
the Go package and the settings section. Not "synchronisation" in one place and
"sync" in another.

**run** — one recorded execution of a sync phase.

**phase** — one half of a sync: `source` (reading) or `targets` (writing).
"Half" is fine in prose; the identifier is always *phase*.

**convergence** — how far a target's held routes have caught up with the
library: `current`, `lagging`, `failed` or `unauthorized`. See
[sync-lifecycle.md](specs/sync-lifecycle.md).

## Sign-in

**subject** — the OIDC `sub` claim Auth0 asserts for a signed-in identity, such
as `github|123456`. It is the wire word and the code word alike; it is never
called a *principal*, a *user*, or an *account* — those name other things this
project already uses those words for.

**allowed subject** — a subject the tenant's post-login Action asserted the
access claim for, and therefore authorised. That decision is made once, at
sign-in, not re-checked against anything live on every later request; the
fixed session lifetime below is what bounds how long it can go stale.

**admin** — a subject the tenant's post-login Action asserted the admin claim
for. Distinct from *operator*: an admin holds cross-subject rights within the
running service, asserted the same way *allowed subject* is; an operator runs
the service and holds its configuration. The two are often the same person,
but a sentence meaning one does not mean the other.

**session** — the browser's proof of a subject's sign-in: the
`__Host-domestique_session` cookie and the `web_sessions` row its hash
identifies. Fixed at a 24-hour lifetime, not renewed on use. See
[configuration.md](specs/configuration.md#runtime-state) for what a session
shares the state database's fate with.

## People

**reader** — the person using the browser UI.

**operator** — the person running the service and holding its configuration.
They are the same person here, but not the same role, and a sentence usually
means one of them.

**rider** — a signed-in subject who owns a Wahoo target, self-service created
by their own first "Connect." Every reader with an authorised Wahoo account is
a rider; not every reader has connected one yet, and an admin sees every
rider's target where a rider sees only their own.

**rider profile** — one rider's own body and equipment: maximum, resting and
threshold heart rate, functional threshold power, and rider and bike mass. Kept
per subject, not per target, and never another rider's. Not a *setting*: the
settings are the service's and shared, this is one person's own.

**suggestion** — a figure a rider's own recent rides imply for one profile
parameter, offered beside its field and stored nowhere. It is not a value until
the rider has saved it as one.

## Spelling

British spelling in prose and in identifiers this project owns:
*authorisation*, *synchronisation*, *colour*. American spelling only where it is
someone else's wire value — `authorized`, `unauthorized` and
`needs_reauthorization` arrive that way and are quoted, not translated.

The browser UI is *the browser UI* in prose, `webui` in package and path names,
and `WebUI` in exported Go and TypeScript identifiers.
