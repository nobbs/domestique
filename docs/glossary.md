# Glossary

The words this project uses, and the one meaning each of them has.

Terms here were previously defined only in module doc comments. Those comments
are good individually, and they are exactly how `key`, `library`, `selection`,
`window` and `account` each came to mean several things without anyone
noticing — a term defined locally cannot see the other definition. This file is
the one place that can.

It describes the vocabulary the code should use. Where the code disagrees today,
[naming-drift.md](naming-drift.md) records the disagreement and what the fix
would be; nothing in this file has been applied as a rename yet, so read it as
the target rather than as a description of `main`.

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

This is the distinction that has caused the most confusion, because both halves
were called `route`: the API type `Route` is the *piece*, while the `routeId`
inside it identifies the *whole*. The whole is a **source route** and its
identifier is a **source route id**.

**stage** — VeloPlanner's word for a source route's ordered pieces, and the
right word when talking about the provider or about storage. It is not the word
for the thing a reader opens; that is a route. `stageOrder` keeps its name
because it is an ordinal within a source route, which is precisely a stage
concept.

**course** — the FIT-file rendering of a route, and only that. It belongs to the
encoder boundary and to Wahoo, never to the browser UI.

**ride** — what a person does with a route. It names durations and predictions
(`rideSeconds`, ride model, "Ride start"), never a stored entity.

## Where routes come from and go

**provider** — the upstream service a route was read from: VeloPlanner or
Komoot. This is the word on the wire (`/v1/providers/{provider}/…`) and the word
in code.

**source** — the read half of synchronisation, and the settings that configure
it. "Source" is the role; "provider" is the identity. A sentence about *which*
service says provider; a sentence about *reading* says source.

**target** — a Wahoo destination that routes are written to. This is the word on
the wire and in types, and it is the word the interface should use as well. It
is not a *slot* and not, on its own, an *account*.

**slot** — one entry in the configured `wahoo.targets` list, which is a name
rather than a Wahoo identity. Naming a slot creates a target's durable record,
and removing it leaves that record standing, so a slot and the target it names
are not the same thing (see [configuration.md](specs/configuration.md)). This is
configuration's word alone; the wire and the interface say *target*.

**account** — a set of credentials. A source has one (a library is read with a
login of its own) and a target has one (an authorised Wahoo connection). Because
both have accounts, "account" alone never identifies which side is meant: say
*source account* or *target account*, or name the side directly.

## The library

**library** — the collection of routes this service holds. Nothing else. A
remote service is a *source* or a *provider*; MapLibre is *MapLibre*.

## Identity and the map

**route key** — the string `provider/sourceRouteId/stageOrder` that identifies
one route. Produced by `routeKey()`. Every variable holding one is a *key*, and
the qualifier says which route it is: `openKey`, `hoveredKey`, `selectedKey`.
There is no *stage key*; it is the same string.

**legend** — the colour reference for gradient bands and surface classes, which
is also the control that filters by them. The component is `RouteKey` only for
historical reasons; a legend is not a key.

## Reading a route

**selection** — a stretch of the route the reader has picked out, expressed as a
`DistanceWindow`. Selecting a *row* in the library is not a selection; that
route is *open*, or *hovered*, or *focused*.

**distance window** — a start and end distance along one route. Always written
in full, because `window` on its own is the browser's.

**highlight** — a gradient band or surface class held above the others. Distinct
from a selection: a highlight is a class of ground, a selection is a stretch of
one route.

**moving time** — the predicted time in motion for a route, `movingSeconds` on
the wire. `cumulativeSeconds` is the same quantity accumulated per coordinate.
There is no separate "ride time" or "elapsed time" — those are the same thing
under other names.

**ascent** — total metres climbed over a route, shown as "Climbing".

**climb** — one named sustained ascent within a route, of the kind `ClimbsList`
enumerates. A route's total ascent is not a climb.

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

## People

**reader** — the person using the browser UI.

**operator** — the person running the service and holding its configuration.
They are the same person here, but not the same role, and a sentence usually
means one of them.

## Spelling

British spelling in prose and in identifiers this project owns:
*authorisation*, *synchronisation*, *colour*. American spelling only where it is
someone else's wire value — `authorized`, `unauthorized` and
`needs_reauthorization` arrive that way and are quoted, not translated.

The browser UI is *the browser UI* in prose, `webui` in package and path names,
and `WebUI` in exported Go and TypeScript identifiers.
