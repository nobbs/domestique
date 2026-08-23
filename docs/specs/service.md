# Domestique service specification

**Status:** accepted

This document is the normative contract for the Domestique service. Where
implementation details differ from this document, this document wins until it is
deliberately revised.

## Purpose and scope

Domestique mirrors the complete route library of one private VeloPlanner
account to two separately authorised Wahoo accounts. It runs automatically and
uploads device-ready FIT courses directly to Wahoo; Ride with GPS is not part
of the service.

Each VeloPlanner route stage is a separate Wahoo route. A single-stage route
keeps its route name. A multi-stage route uses `Route — Stage` so its stages are
individually identifiable on a device.

The service is a single-tenant Docker workload for an amd64 Tailnet host, which
is the only architecture the image is published for. The long-running target is a
small Linux cloud VM. It has no CLI.

The service serves a read-only browser UI for route preview. Its HTTP surface is
read-only JSON for status, route data, and route geometry, except for the
protected Wahoo OAuth onboarding flow and the manual triggers over
synchronisation and surface enrichment. The UI is a
view onto stored state: it draws the whole stored library on one map, opens any
one source route stage over that same map, and reports synchronisation on a
second view. A route is not a page of its own — it takes over the panel the
search occupies and adds its own layers to the map already on screen — but the
route being read is carried in the address, so the view stays linkable.

Where a stage's surface classification has been cached, that view draws it: the
route is banded by ground class on the map, and the stage's split is summarised
beside its stored facts. A stage with no cached classification is drawn plainly
and said to be unclassified, never presented as unsurveyed ground. Because the
classification is a derived OpenStreetMap database, the map view carries the
ODbL attribution its share-alike terms require.

The drawn route says where a ride goes and nothing about which end it begins at,
so the view marks both terminals and the direction of travel. The start and the
finish are marked distinctly, and stay distinguishable on a stage that finishes
where it started: the two markers are separated on screen rather than drawn on
one point. Direction cues are placed along the route at a spacing measured on
screen, so a stage gets the same restrained handful of them whatever its length
and however far the camera is pulled back. Every cue is derived from the stored
geometry already drawn — no request is made to answer it — and every cue has a
textual equivalent stating which way the ride leaves, whether it returns to its
start, and which way it comes back, because the cues themselves are drawn into a
canvas that carries no text.

The key to what the view draws is also how it is questioned: a surface class or
a gradient band can be selected, and the stretches of route it covers stay lit on
the map and in the elevation chart while the rest of the ride is dimmed. One
class at a time, and selecting it again restores the whole route. This changes
what is emphasised, never what is stored.

A stretch of the ride is asked for the same way on either instrument: a drag
across the elevation chart, or a drag along the painted route, selects the
distance range it covers, and both views then show that stretch until the whole
route is asked for again. A drag that begins away from the route moves the map
instead, and moving the map selects nothing. Selecting a stretch by keyboard
remains the elevation chart's. Like the class selection, this changes what is
shown, never what is stored.

The map is the view rather than something embedded in one, so it holds its own
gestures throughout: the wheel zooms, fingers pan and zoom, and the arrow keys
answer. Nothing is printed over the cartography to ask for them. A drag that
begins on the painted route picks a stretch of the ride instead of moving the
camera, because that gesture is a question about the ride rather than about the
view.

The elevation chart floats across the foot of the map rather than sitting in a
column beside it, and it can be folded away to a pill that still states the
route's total climbing and the heights it runs between. That choice lasts as
long as the tab and is deliberately not stored.

The UI carries one outbound link, to this service's public source repository. It
is the only navigation that leaves the authenticated origin, it opens in a new
context without a referrer, and it sends nothing: no route, stage, geometry, or
origin address accompanies it.

Route editing remains explicitly out of scope. The UI presents no editing
affordance, and the service writes nothing back to VeloPlanner. Any future
change to that boundary requires revising this document first.

## Constraints and non-goals

- Sync every route in the configured VeloPlanner library; there is no selection
  by tag, prefix, or allow-list.
- Preserve no integration with Ride with GPS.
- Do not provide route editing or a command-line interface. The browser UI is
  read-only.
- Do not run a secret manager or reference a specific secret provider from Go.
- Do not back up the persistent service data. Recovery must be safe despite
  that intentional constraint.
- Do not include credentials, OAuth tokens, personal route fixtures, or the
  Wahoo client secret in this public repository.

## Deployment and access model

Docker publishes the service port only to the host's `127.0.0.1`; the
container has no public host port. The Tailnet host exposes it privately through
`tailscale serve`; it is never directly published to the Internet. All service
endpoints require the configured sole identity, apart from two loopback probes:
a liveness probe on the served listener, and a readiness probe on a second
listener of its own. Tailscale Serve and the tunnel front the served listener
only, so the readiness listener is reachable from host-local health checking and
not from the authenticated public surface. It must never be given to
`tailscale serve`. The HTTP server trusts Tailnet
identity headers only from that local proxy.

The service is single-tenant, and remains so. One person is authorised, and
there is exactly one way to prove it.

Every request must carry a `Cf-Access-Jwt-Assertion` the service verifies
itself: RS256 signature against the team's published keys, matching issuer,
unexpired, and an `aud` equal to the configured application's audience tag. The
`email` claim must equal `access.cloudflare.allowed_email`. The unsigned
`Cf-Access-Authenticated-User-Email` header is never consulted, and neither is
`Tailscale-User-Login`.

Identity is not provenance. An Access session lives in an ordinary browser, so
proving who is calling does not prove that they meant to call. Every
state-changing endpoint therefore also requires an `Origin` header exactly equal
to the origin the browser UI is served from — the scheme and host of
`wahoo.redirect_url`, which is by construction the hostname a browser reaches
this service at. A missing, malformed, `null`, or cross-site origin is refused
with 403 before any trigger runs or any state is written; a browser attaches
`Origin` to every request whose method is not GET or HEAD, including a
same-origin one, so absent means "not this UI" rather than "same origin". This
is a security requirement, not an implementation detail.

The OAuth start and callback are deliberately outside that check. The callback
is a cross-site GET the browser is redirected into, and what protects it is its
one-time, identity-bound, expiring state.

Requests reach the service through Cloudflare Access and a Cloudflare Tunnel
whose origin is this service's **Tailscale Service name**. `cloudflared` runs on
a tagged node, and Serve never populates identity headers for a tagged device,
so such a request carries no Tailnet identity to consult in the first place.

Tailscale Serve still fronts the listener, and Tailnet members can still reach
it directly. Such a request is refused like any other without an assertion.
Honouring `Tailscale-User-Login` would mean a second front door with a second
identity source behind it, and — because a tunnel forwards client headers
verbatim — a forgeable one the moment anything but Serve reached the listener.
The header is therefore not read at all. This is a security requirement, not an
implementation detail.

This adds no public listener: the container still publishes to loopback only,
and the tunnel is an outbound connection. It does not widen the gate either —
one principal before, one principal now.

The tunnel's origin must be the Tailscale Service name rather than a node
address or loopback. That is a security requirement, not a convenience: it keeps
Tailscale Serve in the path, and Serve's stripping of client-supplied
`Tailscale-*` headers is what prevents a public caller from asserting a Tailnet
identity. The `[access.cloudflare]` section is all-or-nothing; a partly
configured one is rejected at startup.

The map view introduces one deliberate, documented exception to the otherwise
Tailnet-only posture: the operator's **browser** fetches basemap tiles from a
configured third-party tile origin, and the area those tiles cover is
necessarily the viewport on screen — a viewed route's, or, once the reader
presses the map's locate button, an area centred on their own live position.
The service itself never contacts a tile origin. The raw coordinates the
locate button reads from the browser's Geolocation API go nowhere beyond
moving the camera: they are never sent to this service, a log, storage, or
the tile origin as a discrete value — only the resulting tile requests, naming
the area rather than a point, reach it, the same way a viewed route's area
already did. The default is a single keyless provider, so no credential is
exposed to the browser and the requests carry no account identity. The
basemaps are static configuration so an origin can be changed, or pointed at
a self-hosted tile source, without a code change.

An operator may configure more than one basemap so the reader can switch
between them, and a Content-Security-Policy restricts the browser to the
service's own origin plus the origin of each configured basemap. That policy
says which origins the page *may* reach; only the basemap on screen is ever
requested, so what any one provider learns is unchanged by the presence of
another in the list. A second style may be configured for a dark system colour
scheme, and must be on **its own basemap's** origin, so following the operator's
colour scheme reveals nothing to anyone new.

The reader's pick is remembered in the browser's own storage, under one key
holding the chosen basemap's name and nothing else. It is never sent anywhere:
the service is not told which basemap a browser loads, and the choice is kept
out of the address so a shared link to a route does not carry it. A browser that
refuses storage still switches; it simply starts again from the first entry on
the next visit.

Surface classification introduces **no** such exception. To learn whether a
stage runs on asphalt, paving, gravel, or a forest track, the service reads a
surface index it builds itself from OpenStreetMap regional extracts, and the
whole classification then happens on the host with no request leaving it. No
route shape is ever sent anywhere. The only outbound traffic is the scheduled
rebuild: on its own cadence the service downloads the configured regions'
published extracts from the extract host, which learns which regions this
deployment is interested in and nothing about any route. An operator who
configures no region downloads nothing at all and leaves stages unclassified.

Each stage is classified once per geometry per index build: the answer is cached
against both the stage's content hash and the generation of the index it was
read from, so a stage is reclassified when its shape changes and when the map
underneath it is rebuilt, and at no other time.

Open-Meteo is the second service the deployment itself reaches outbound, after
the OpenStreetMap extract download above — and unlike the tile origin, which
only the operator's **browser** reaches. The service asks Open-Meteo for an
hourly forecast at the coordinates and times `GET /v1/weather` was given; a
viewed route's shape and timing are what leaves, and nothing about the
account viewing it, because the request carries no identity. Operating the
service requires attributing Open-Meteo under its [CC BY 4.0
licence](https://open-meteo.com/en/licence) wherever its data is shown, the
same way surface classification carries the ODbL attribution above.

The Wahoo OAuth redirect URI is the HTTPS URL a browser returns to. Without the
public path that is the service's Tailnet URL:

```text
https://<service>.<tailnet>.ts.net/oauth/wahoo/callback
```

With the public path deployed it is the public hostname instead, because the
redirect lands in an ordinary browser that need not be on the tailnet:

```text
https://<hostname>/oauth/wahoo/callback
```

It must exactly match the URI registered with Wahoo and configured in the
service. The authorisation redirect is followed by the user's browser; Wahoo
does not need a public connection to the host.

The state-changing HTTP surface is the OAuth flow:

- `GET /oauth/wahoo/start/{target}` starts authorisation for a configured target
  slot.
- `GET /oauth/wahoo/callback` validates a one-time, expiring OAuth state and
  stores the resulting refresh token.

Both are limited to the configured principal. The state binds the
calling identity and target slot and prevents cross-account or CSRF callbacks.
The service rejects an attempt to authorise the same Wahoo account for two
target slots.

Alongside it are the operator controls over synchronization: the manual
triggers, the two switches that decide what the timer is allowed to start, the
per-stage reprocess request, and the surface-enrichment retry.
They change what the service does next; they change nothing it has stored about
routes, and they cannot make a run less safe than a scheduled one, because a
triggered run is the same run through the same gates. The enrichment retry is
narrower still — it never reads VeloPlanner or writes a Wahoo target, only
reclassifying stages already stored — and shares the same single-flight guard
as the other four triggers. Every one of them is limited to the same principal
as the rest of the surface.

A synchronization has two halves, and each is separately switched, triggered,
and reported:

- The **source** half reads the VeloPlanner library, validates it, and stores
  it. It contacts no target and needs no authorisation.
- The **target** half reconciles what is stored onto each Wahoo target. It reads
  the stored library rather than fetching a fresh one, so a target that was
  unreachable catches up from the last inventory known to be whole.

Splitting them lets an operator stop writing to devices while still refreshing
the library, or keep devices current while the source is known to be in flux,
without editing configuration on the host and restarting the service. The
switches govern the timer only: a manual trigger runs its half whether or not
the timer is allowed to, because an operator asking for a run has already
decided. Neither switch stops a run already in flight.

The read-only JSON surface is deliberately small:

- `GET /healthz` reports local process health, on the served listener.
- `GET /readyz`, on the readiness listener alone, reports whether local
  configuration and the state the process needs are usable. It verifies nothing
  else: no VeloPlanner, Wahoo, Pushover, Cloudflare, Tailscale, or tile provider
  call, and no judgement about whether a target has completed its one-time
  authorisation, because a slot waiting for a browser visit is a correctly
  running deployment. It reports a fixed category when it is not ready, never a
  path, key, or upstream detail.
- `GET /v1/status` reports current configuration readiness, last sync outcome,
  aggregate counts, target authorisation state, the two schedule switches, the
  last run of each half, and how much of the library carries a current surface
  classification together with which map build it was read from. It also reports
  whether every stored stage at its current
  revision has reached every configured target: one convergence word, safe
  aggregate current and pending counts, and the last reconciliation result per
  target, plus one overall answer that is true only when every target is
  current. Those are derived from stored revisions alone, never by asking Wahoo
  what it holds, and they describe the Wahoo accounts rather than what any
  physical head unit has downloaded. It also names the build that is running: the full public
  source commit the binary was compiled from, and the digest of the image
  carrying it when the host told the service one. That group is absent for a
  build that carries no injected revision, which is how a reader tells a
  development process from a deployed one. Only a full commit object name and a
  bare `sha256:` digest are served; a value that is neither is dropped rather
  than published, because in a browser it would become a link to nowhere. No
  registry, repository, host, tag, or path is included.
- `GET /v1/sync/runs` returns a bounded, paginated history of terminal runs,
  newest first: each one's opaque reference, half, completion time, result, safe
  failure category, and aggregate counts. Older runs are pruned as new ones are
  recorded, so it is recent history rather than a permanent record. It is the
  one read that takes query parameters — a bounded page size, and the cursor the
  previous page ended with — because a page of history cannot be decided in the
  browser from a listing it already holds.
- `GET /v1/routes` lists known source routes and stages with their titles and
  aggregate geometry facts.
- `GET /v1/routes/{source-route-id}/stages/{stage}` returns stored route
  metadata, not edit controls.
- `GET /v1/routes/{source-route-id}/stages/{stage}/geometry` returns the stored
  geometry of one stage for map rendering, together with the surface
  classification of that geometry when one has been cached.
- `GET /v1/webui/config` returns the settings the browser UI needs at runtime so
  the built assets stay static: the list of basemaps the map may be switched
  between — each with its name, style URL, optional dark style URL, and whether
  its cartography is dark in either colour scheme — and the source provider's
  base URL. The list is never empty, and its first entry is what a browser that
  has chosen nothing loads. The base URL is the whole of what is sent about the
  provider — the page builds a stage's link back to its source route from it,
  rather than the service repeating a route URL on every stage it serves. It is
  omitted when unconfigured, so the page shows no such link rather than a broken
  one. That link is only useful to the operator whose account holds the route:
  the source route is private to that account, and following it as anyone else
  reaches the provider's own refusal, not the route.
- `GET /v1/weather` returns an hourly forecast for up to 48 repeated `point`
  values, so the page can show a ride's weather without reaching Open-Meteo
  itself. Each `point` is `latitude,longitude,time`: decimal-degree latitude
  and longitude, and a full RFC3339 timestamp with an offset (or `Z`) and
  seconds. Each point is answered with the single hour of its own
  coordinate's series nearest its own requested time — never Open-Meteo's
  field names or its raw payload. A malformed point, more than 48 of them, or
  a window Open-Meteo could not answer is refused as `400` before any
  outbound call; a provider failure is `502`, carrying no upstream response
  text.
The seven endpoints below that change state — the four sync triggers, the
schedule switch, the reprocess request, and the enrichment retry — additionally
require the browser origin described above, and answer 403 without it.

- `POST /v1/sync` queues one immediate synchronization of both halves through
  the same reporting path as the schedule. It returns `202 Accepted`, or `409
  Conflict` when a scheduled or manual synchronization is already running.
- `POST /v1/sync/source` and `POST /v1/sync/targets` queue one half on the same
  terms.
- `POST /v1/sync/targets/{target}` reconciles exactly one configured target
  slot, on the same terms as `POST /v1/sync/targets` scoped to that slot alone.
  `{target}` must name a configured slot, or the request is refused as `404`.
- `POST /v1/targets/{target}/clear` deletes every route this service owns from
  exactly one configured slot and forgets that slot's stage mappings. It is the
  one deletion the per-target deletion limit does not bound, and is reachable
  only this way — nothing schedules it. It still deletes only routes carrying
  an external ID this service issued, and leaves the stored library untouched,
  so the next reconciliation rebuilds the target. It returns `202 Accepted`,
  `404` for a slot that is not configured, or `409 Conflict` while any run is
  under way.
- `PUT /v1/sync/schedule` sets both switches, and answers with the state it
  stored. A body that names only one switch is refused: the other would be left
  at whatever the caller assumed it was.
- `POST /v1/routes/{source-route-id}/stages/{stage}/reprocess` asks for one
  stage to be worked out again from scratch and starts the synchronization that
  will do it. It returns `202 Accepted`, or `404` for a stage that is not in the
  stored inventory.
- `POST /v1/sync/surface` queues one immediate surface-classification pass,
  independently of either sync half. It returns `202 Accepted`, or `409
  Conflict` when a synchronization or another such pass is already running. It
  never reads VeloPlanner or writes a Wahoo target, so it carries none of the
  other triggers' provider risk.

The browser UI is served from the same origin and the same authenticated
listener: an application entry document and immutable hashed static assets.

The response schemas are defined in
[the sync lifecycle specification](sync-lifecycle.md). They must never expose
secrets, tokens, or raw upstream response bodies.

Route geometry is served **only** on the dedicated geometry endpoint, only to
the configured principal, and only from local stored state. It must never
appear in logs, notifications, error messages, the status endpoint, or the
inventory listing.
The concrete OAuth, sync, persistence, and JSON contracts are defined in the
[sync lifecycle specification](sync-lifecycle.md).

## Configuration and secrets

The concrete file schema and validation rules are defined in the
[configuration specification](configuration.md).

The service has a provider-neutral configuration contract:

- One read-only static configuration file holds non-secret values: VeloPlanner
  account identity and endpoint, Wahoo client ID and API endpoints, target slot
  labels, Access team domain, application audience tag and allowed address, sync
  cadence, deletion limit, data path, and public callback URL.
- Sensitive static values are loaded by Koanf from Docker-style files or the
  documented direct environment variables: VeloPlanner credentials, Wahoo
  client secret, Pushover application token and user key, and a 32-byte
  state-encryption key.
- Dynamic Wahoo refresh tokens are not static configuration. They are encrypted
  at rest in the local state database with an authenticated cipher and the
  state-encryption key. Access tokens are held only in memory.
- Configuration must not understand `op://`, `env:`, provider URIs, provider
  credentials, fnox, or another provider-specific reference syntax. The service
  stays CGO-free.

Docker Secrets, read-only bind mounts, and direct environment values are valid
secret sources. A deployment tool such as `fnox` may provision files, but does
not become an application dependency.

The static configuration defines two named Wahoo target slots. The target's
actual Wahoo user identity is learned and persisted during OAuth onboarding.
Sync remains disabled until every configured target is authorised.

## Persistent state and recovery

A SQLite database on a Docker volume stores:

- Wahoo target identities and encrypted refresh tokens;
- source route/stage identity, source revision, content hash, and Wahoo
  `external_id`;
- a cache of source stage titles and geometry for the route map view, written
  only when a stage's content hash changes and kept in its own table so it can
  be dropped without touching deletion-safety state;
- a cache of the surface classification of that geometry, in its own table for
  the same reason, recorded against both the content hash and the surface-index
  generation it was measured for. The two are not acted on alike: a changed
  content hash withdraws the cached answer, whose ranges are positions in a
  geometry the stage no longer has, while a newer index leaves it on display and
  marks the stage for re-measurement on the next enrichment pass — those ranges
  still fit, so a rebuild costs the library nothing while the rare road that was
  actually resurfaced is still corrected;
- when the last surface index build finished and which generation it produced, so
  the rebuild interval is time between builds rather than time since this process
  started;
- the corresponding remote Wahoo route identity where available;
- last successful source inventory and last sync outcome; and
- expiring OAuth states.

The database holds no plaintext credential or token. It is intentionally not
backed up.

The schema is versioned by an append-only migration history, and migrations run
forward only. Because there is no backup, a deployment that migrates the state
and then fails its health gate must still be able to roll back onto the previous
image, so:

- every migration must leave the previous release's binary able to read and
  write what it already did — additive with defaults. A migration must not drop
  or rename anything an earlier binary uses, add a `NOT NULL` column without a
  default to a table an earlier binary inserts into, tighten a `CHECK` or add a
  `UNIQUE` index an earlier binary's writes could violate, or change what an
  existing column's values mean; and
- a binary opens a state file up to one migration ahead of itself, and refuses
  one further ahead than that with a distinct error. A release that must stay
  rollable therefore appends one migration.

The rollback path never restores or replaces state; the tolerance is what makes
the previous image usable against a state file the failed deploy migrated.

One migration is a deliberate exception to the first rule, and it is recorded
here rather than left to be discovered during an incident. The migration that
gave a stage's identity its provider widened the primary key of
`target_stages`, `stage_geometry`, `stage_surface`, and `stage_reprocess` to
include the new column. A previous release's binary writes those four tables
through `ON CONFLICT (route_id, stage_order)`, and SQLite requires that column
list to name an existing unique constraint exactly, so those statements no
longer prepare against the widened key. A rollback onto that binary can still
**read** all four tables — no state is lost or rewritten — but cannot write
them, which leaves synchronisation failing until the deployment rolls forward
again. The narrower alternative, leaving provider out of those keys, would stop
four of the six stage-keyed tables from holding two providers' stages at once
and so defeat the migration's purpose. A future migration carries the same
obligation to be additive; this exception licenses that one change, not a
general relaxation.

It does hold the operator's route geometry in plaintext as a rendering cache.
That is personal data rather than a credential, and losing it is harmless — the
next sync refills it from the source — but it raises the sensitivity of the
volume and should inform where the host keeps it.

If the database or volume is lost, the operator must re-authorise every Wahoo
targets. The service must reconcile using the Domestique-owned `external_id`
before it creates or removes routes. It must not treat lost local state as
authority to delete routes from a Wahoo account.

## Source and course representation

VeloPlanner is an unofficial, session-cookie-based source integration using a
private account's username and password. Every sync obtains a fresh authenticated
session before listing routes. The source adapter must retain the proven
baseline behaviour for route geometry:

- read VeloPlanner's ordered stages and segments;
- join each stage's decoded `[longitude, latitude, elevation]` points in order;
- preserve elevation when present;
- emit one course per non-empty stage; and
- use the source route ID and stage order as stable identity, never the route
  title.

The service normalizes fully-elevated source-stage elevations before device
export: it samples the elevation profile at 25-metre intervals and applies a
centred 100-metre moving median to remove isolated altitude spikes. It retains
the original route geometry. The resulting profile is the single source for
FIT elevations and Wahoo ascent/descent metadata. Each FIT record carries
cumulative route distance in metres as well as coordinates and elevation, so
Wahoo can derive an elevation profile and gradients. The FIT adapter uses
[`github.com/muktihari/fit`](https://github.com/muktihari/fit), isolated behind
the course encoder boundary and without vendoring Garmin SDK files or test data.
The source code remains MIT-licensed; third-party notices remain with their
respective dependencies. Operating the service requires compliance with the
[FIT Protocol License](https://www.thisisant.com/developer/ant/licensing/flexible-and-interoperable-data-transfer-fit-protocol-license).

An acceptance test decodes the generated FIT, uploads it to the Wahoo sandbox,
and confirms the resulting course is usable by a Wahoo account; it is run by the
operator, as [the FIT acceptance guide](../fit-sandbox-acceptance.md) describes.
An encoder change is adopted only once that check covers it. No personal route
data belongs in repository fixtures.

Komoot is a second, independent source integration against the same unofficial
category of interface, described here ahead of the configuration and
composition-root work that will actually offer it to an operator: until that
work lands, the purpose and scope above still hold, and a deployment mirrors
VeloPlanner alone. The adapter reads a private account's own email and
password, exchanged at runtime for a session token, with no OAuth machinery
involved. It is unofficial
because Komoot's OAuth2 partner interface is issued only under a partner
contract and is not available to an individual operator; the unofficial and
partner interfaces serve the same documented resource shapes, so only the
authentication differs. A Komoot tour has no stage subdivision, so it maps to
exactly one stage — order 1, no stage name — rather than leaving the concept
unused. The adapter issues no HTTP method other than `GET`: the session token
Komoot returns is not read-scoped, and a write against it would delete a route
from the operator's own library with no undo.

Both sources hand over geometry and metadata only. Neither source's FIT, GPX,
surface classification, way types, or turn directions are consumed, even where
the provider offers them: FIT is produced once, by this service's own encoder,
for every source alike, and surface classification is computed once, against
this service's own OpenStreetMap index, for every source alike. A provider that
supplied any of these directly would let two identical roads disagree depending
on which source found them, and would give two sources unequal elevation
handling. This rule is provider-agnostic and applies to any future source.

## Wahoo synchronisation

Wahoo is the only destination. The service uses the approved confidential Wahoo
Cloud application with `routes_read`, `routes_write`, and `user_read` scopes.
The configured Wahoo environment may be sandbox; it is valid for this service
within its lower rate limits.

For every source stage and Wahoo target, Domestique derives a deterministic
`external_id` from the source route ID and stage order. It supplies that ID and
the source revision as `provider_updated_at` when it creates or updates a FIT
route. A changed source stage updates the existing owned Wahoo route rather
than using an upload-and-delete replacement.

OAuth refresh is serialised per Wahoo account. A refresh token returned by a
successful refresh replaces the prior stored token atomically before any later
request can use stale credentials. The Wahoo application is rate-limited across
all configured targets: API calls are serial, obey advertised limits, and resume only
when safe to do so.

Domestique deletes only Wahoo routes it owns through its `external_id`. A
source-stage deletion removes the corresponding owned Wahoo route from both
targets. It never deletes manually created Wahoo routes.

## Sync lifecycle and safety

The detailed state transitions and safety gates are defined in the
[sync lifecycle specification](sync-lifecycle.md).

The service attempts one sync shortly after a healthy startup and then hourly.
At most one sync may run at a time. It fetches the source inventory once, then
processes configured Wahoo targets serially so one account's failure does not stop
an attempted update of the other. The overall run is failed if either target
fails.

No automatic run may delete more than five owned Wahoo routes from a target.
Before any deletion, the source inventory is checked against the last trusted
inventory. Authentication failure, malformed source data, an empty result after
a previously non-empty library, or a suspicious shrink stops deletion and raises
an alert. A normal small, authenticated source deletion is mirrored.

Source authentication and inventory safety are more important than making a
deletion immediate. A final-library deletion that would appear as an empty
source requires an explicit static configuration acknowledgement before it can
delete Wahoo routes.

Every run records a terminal outcome. Pushover receives:

- a concise aggregate success notification after a successful sync, as much of
  it as the configured success policy delivers — every run, none while healthy,
  or one aggregate digest per period;
- the first success following any run that did not succeed — a failure, a
  blocked run, or a half left needing onboarding — whatever that policy says;
  and
- the first failure notification immediately, followed by suppression of
  identical failures for six hours.

Notifications contain aggregate counts and a safe failure category only. They
contain no route names, tokens, credentials, or raw upstream errors.

## Go design

The concrete package boundaries, interface rules, manual composition root, and
delivery sequence are defined in the
[implementation architecture specification](implementation-architecture.md).

The implementation is package-oriented with manual wiring in the server
entrypoint. Packages own their own data types and expose narrow interfaces only
at consumer boundaries. There is no dependency-injection framework, shared
`interfaces` package, or global service locator.

The intended dependency direction is:

```mermaid
flowchart LR
    HTTP["HTTP API"] --> App["application / sync orchestration"]
    HTTP --> WebUI["embedded browser UI assets"]
    Scheduler["scheduler"] --> App
    App --> Source["VeloPlanner source adapter"]
    App --> Course["course and FIT encoder"]
    App --> Wahoo["Wahoo OAuth and route adapter"]
    App --> State["SQLite state adapter"]
    App --> Notify["Pushover adapter"]
    Config["static config and secret files"] --> HTTP
    Config --> App
    Config --> State
```

`application` defines the source, destination, state, and notification
interfaces it consumes. Adapters do not import each other. `cmd/domestique`
only loads configuration, constructs concrete adapters, starts the scheduler,
and performs graceful shutdown.

## Quality and delivery requirements

The concrete local quality gate, GitHub Actions, container hardening, and
published image contract is defined in the
[delivery specification](delivery.md).

Implementation must include a project-local Go toolchain declaration, focused
`golangci-lint` configuration, `prek` local checks, and GitHub Actions that run
the same essential validation. Normal tests use deterministic fakes for every
external service. A separately invoked sandbox acceptance check validates the
FIT/Wahoo contract and never receives production secrets through CI.

Docker images are published to GHCR from each default-branch change that alters
an input of the image, and deployed to the Pi by immutable digest. They are not
signed; the [delivery specification](delivery.md) states why and what stands in
its place.
The macOS MVP may build locally from the checkout; its configuration and Docker
secret files remain outside Git.

## Acceptance criteria

- Two Wahoo accounts can be authorised through the Tailnet-only OAuth flow.
- An hourly run mirrors every valid VeloPlanner stage to every configured target as FIT.
- Edits preserve the stage's `external_id`; source deletions remove only owned
  destination routes and respect the deletion guard.
- A failed source inventory cannot cause a destructive Wahoo deletion.
- Lost state cannot cause deletion of unknown Wahoo routes.
- The service logs and notifications do not reveal secrets or route details.
- Every state-changing HTTP interaction additionally proves it came from this
  service's own browser UI, so an authenticated session cannot be driven from
  another site.
- Every HTTP interaction is identity-gated, to one principal, by a signature the
  service verifies itself. Beyond OAuth, the only ones
  that change anything are the synchronization triggers, the two schedule
  switches, the reprocess request, which discards derived answers so they are
  worked out again, and the surface-enrichment retry, which reclassifies stored
  stages without reading VeloPlanner or writing a Wahoo target. Nothing on the
  surface edits route data, in this service or at the source.
- The browser UI renders stored source stages on a map, is reachable only by
  the configured identity, and offers no editing affordance. Selecting a surface
  class or gradient band in its key only changes what the map and the chart
  emphasise. Searching the library does the same for the listing it already
  holds: it is decided in the browser, over the safe display names the inventory
  listing already carries, and it adds no query parameter or any other
  server-side query surface.
- The only outbound link the UI offers is to the public source repository, and
  following it discloses neither the origin nor anything about the route on
  screen.
- Stage geometry is cached locally and rewritten only when a stage's content
  hash changes, so an unchanged library does not rewrite the cache on every run.
- Losing the geometry cache degrades only the map view; it cannot affect sync
  safety or cause a destructive Wahoo operation.
- The codebase has reproducible local and GitHub validation before an image is
  published.
