# Domestique sync lifecycle specification

**Status:** accepted

This specification is subordinate to [the service contract](service.md). It
defines the durable lifecycle of OAuth onboarding, synchronisation, and the
read-only HTTP JSON surface.

## Stable identities

A route is identified by the triple:

~~~text
provider + source route ID + stage order
~~~

The provider is the upstream that issued the source route ID. `veloplanner` is
the only provider served.

A route's deterministic Wahoo external ID is:

~~~text
domestique:<provider>:<route-id>:stage:<stage-order>
~~~

A VeloPlanner route's external ID is therefore:

~~~text
domestique:veloplanner:<route-id>:stage:<stage-order>
~~~

Titles, descriptions, content hashes, and Wahoo route IDs are mutable metadata;
they never identify a route. The external-ID format is not configurable.

## Durable records

SQLite persists these conceptual records. Concrete table names are an
implementation detail.

| Record | Purpose | Sensitive fields |
| --- | --- | --- |
| target | configured target slot, Wahoo user ID, authorisation state | encrypted refresh token |
| oauth transaction | target slot, caller identity, state digest, expiry, consumed status | none |
| route | stable identity, source revision, metadata, content hash | none |
| target route | route key, target, external ID, Wahoo route ID, last applied revision | none |
| trusted inventory | complete validated route set and observed time | none |
| route geometry | cached titles, geometry, length, and extent for the map view | none |
| route surface | cached surface classification of one stored geometry, as index ranges plus matched length, against the content hash and the surface-index generation it was measured for | none |
| surface index | when the last index build finished and which generation it produced | none |
| sync run | opaque reference, half, start, end, terminal state, aggregate counts, safe failure category | none |
| sync schedule | whether the timer may start each half | none |
| reprocess request | one route an operator has asked to have redone | none |
| notification state | last delivered failure category and suppression deadline | none |
| runtime settings | the settings an operator edits while the service runs, including the basemap list and the surface regions | none |

Every recorded run carries an opaque reference of random bytes. It is the only
detail of a run that may appear in a notification, and it is what an operator
matches a served record against. A message about no single run — a digest —
names no reference.

Run records are bounded to a fixed number of the most recent runs, pruned in
the same transaction that records a run. The newest run of each half is never
pruned, whatever its age; that record is what `GET /v1/status` reports for a
half. Pruning touches nothing else: it never affects the trusted inventory,
target route mappings, OAuth state, or a deletion gate.

OAuth state is stored as a digest. Refresh tokens are encrypted before being
written. Access tokens, OAuth authorisation codes, CSRF state values, raw
upstream bodies, and FIT bytes are never persisted.

The route geometry cache is written during the same transaction that stores the
trusted inventory, from data the run already holds, and makes no extra source
request. A route whose content hash is unchanged is **not rewritten**. Rows
whose route has left the inventory are pruned.

The geometry cache is a separate record from the route itself. The trusted
route set backs the deletion guard and is replaced wholesale each run; the
geometry cache serves the map view alone and may be dropped at any time.
Losing it degrades the map until the next run and affects neither sync safety
nor any deletion.

The route surface cache is filled by a separate pass: automatically after every
successful source read, and on request through the manual retry below. It reads
the ways lying under each route from the locally built surface index. It runs
after a source read, belongs to no half, and cannot change any run outcome.

The whole inventory is walked in one pass. A route already classified against
both its current content hash and the generation of the live index is skipped
without being read. A rebuilt index reclassifies the library over the run or
two following the build.

A pass with no index behind it does nothing. No route is recorded as unsurveyed
while no index has been built. That is the state of a deployment with no
regions configured, and of one whose first build has not landed.

A route that fails does not end the pass. Each route is attempted independently
of the one before it, and the inventory is walked in the same order every pass.

No part of the pass may fail a run. A pass that leaves work undone writes one
log line carrying the counts and whether it ran to the end; it logs no route
name, no geometry, and nothing the endpoint returned. `GET /v1/status` reports
two counts alongside it: how many stored routes carry a classification of the
geometry they currently hold, which is durable, and how many the most recently
completed pass could not classify, which is not — that count answers from one
pass alone and reads zero after a restart until a pass has run. A route in
neither count is waiting its turn. A route whose geometry has been re-planned is
reclassified; its cached ranges are positions in a coordinate array that has
been replaced.

## OAuth lifecycle

The configuration has one or two target slots. Each begins in `not_authorized`.
Automatic sync does not start until every configured slot is `authorized`.
State names are spelled here as the wire spells them.

Three of the four states are stored on the slot. `pending` is not: it is derived
at read time from an OAuth transaction that has neither expired nor been
consumed. A slot that is already `authorized` stays so while a fresh flow runs,
and its refresh token remains valid until that flow replaces it.

~~~mermaid
stateDiagram-v2
    [*] --> not_authorized
    not_authorized --> pending: protected start request
    pending --> authorized: valid callback and token exchange
    pending --> not_authorized: abandoned flow, from an unconnected slot
    pending --> needs_reauthorization: abandoned flow, from a rejected slot
    authorized --> needs_reauthorization: Wahoo invalidates refresh token
    needs_reauthorization --> pending: protected start request
~~~

1. The configured Tailnet user requests
   GET /oauth/wahoo/start/{target}.
2. The service creates 32 random bytes of state, saves its digest with the
   target, caller identity, ten-minute expiry, and unused status, then redirects
   the browser to Wahoo.
3. The callback accepts code and state only through the configured Tailnet
   callback URL. It verifies the caller identity, target, expiry, digest, and
   unused status before exchanging the code.
4. The service obtains the Wahoo user identity with the existing user_read
   grant. It rejects an account already associated with the other target slot.
5. It atomically encrypts and stores the new refresh token, marks the transaction
   consumed, and redirects with 303 See Other to the browser UI at /. The
   redirect removes the authorisation code and state from the browser URL.

A callback failure returns a generic error response. It never echoes the code,
state, token, Wahoo account identity, or upstream response.

## Manual trigger

A manual trigger is a state change and carries the browser-origin requirement of
every state-changing route.

The configured Tailnet user can request `POST /v1/sync` to start an immediate
synchronisation of both halves, or `POST /v1/sync/source` and
`POST /v1/sync/targets` to start one. `POST /v1/sync/targets/{target}`
reconciles exactly one configured target slot without touching the source read
or any other target. `{target}` must name a configured slot; a request naming
any other is refused as not found, as the OAuth start route refuses one.

Each trigger uses the same reconciliation, durable run record, and Pushover
notification path as scheduled work. A single-target request applies the same
ownership, ordering, update-before-delete, and deletion-limit rules the target
phase always applies to that slot.

The service returns `202` only when no scheduled or manual run is active. A full
synchronisation and a single-target one share one mutual exclusion; neither may
start while the other is in flight. Otherwise the service returns `409` and
starts no provider work. A refused trigger changes nothing, the status included:
the run already in flight remains the one described, and no second run state
comes into being.

A manual trigger runs its half whether or not the schedule is allowed to start
it. The switches govern unattended runs only.

## Retrying enrichment

`POST /v1/sync/surface` asks for one immediate classification pass on the same
terms as a manual trigger: it carries the browser-origin requirement, and the
service returns `202` only when no synchronisation or other classification pass
is active, `409` otherwise. It shares one mutual exclusion with every other
manual trigger above — `POST /v1/sync`, `POST /v1/sync/source`,
`POST /v1/sync/targets`, and `POST /v1/sync/targets/{target}`. None of the five
may run while another is in flight.

It never reads VeloPlanner and never writes a Wahoo target. It reclassifies the
routes already stored against the local surface index and cache alone, which is
the pass a successful source read runs automatically. It can neither create,
update, nor delete a route on any target, and carries none of the safety gates
or notification traffic a synchronisation does.

## Reprocessing one route

A route carries three derived answers the service reuses while they still look
current: the geometry it derived and stored, the revision it last pushed to each
target, and its surface classification.

A reprocess request discards all three for that route and starts a
synchronisation of both halves. The route is read again, derived again, encoded
again, pushed to every target regardless of the revision recorded there, and
classified again.

It is not a delete and not a create. The Wahoo route identity is kept, so the
route the service already owns is rewritten in place through the same ownership
rules an ordinary update uses. The request touches no source data: VeloPlanner
is read, never written.

The request is recorded before the run is asked for, and survives a refused
start. It waits for a pass that will honour it, and is consumed by that pass, so
it is met exactly once.

A request for a route that is not in the stored inventory is refused.

## Schedule switches

Two durable switches decide what the timer starts: one for the source half, one
for the target half. Both are on until an operator turns one off.

A switch governs the next tick only. It never stops a run in flight, never
changes what a manual trigger does, and never relaxes a safety gate. A half that
does run, runs exactly as it always does.

A schedule that cannot be read starts nothing, and is recorded and notified as a
failed source run. Off and unreadable are different answers, and the timer must
not act on the second as though it were the first.

## Wahoo token use

Wahoo access and refresh tokens are handled per target:

1. Immediately before a Wahoo API request, the service decrypts that target's
   refresh token and obtains a fresh access token.
2. It transactionally writes the replacement refresh token before making a
   later request, so a crash cannot leave only a stale token on disk.
3. It performs the required API request with the in-memory access token.
4. A rejected refresh token sets only that target to
   `needs_reauthorization`; the other target is still attempted.

All Wahoo calls are serial across configured targets. The client observes
advertised limits and request-response boundaries before the next target call
begins. It observes rate limits and waits or ends the run safely; it never
issues parallel retries.

An advertised quota that reaches zero holds the next request back until it
refills, whether or not the destination said when that will be. A reported reset
of zero means the responding request was not itself limited, not that the quota
is already back. When the wait would exceed what one run holds itself open for,
the run ends and reports the limit rather than sleeping through it. Each route is
recorded as its own write succeeds, so the next scheduled run resumes from stored
state and the library converges over successive runs.

## Sync lifecycle

A healthy process schedules one delayed startup run, then one hourly run. A
single in-process lock prevents overlap across both halves. A second trigger
records no work and returns without modifying state.

Each half is a run of its own: its own record, its own outcome, its own
notification. Failure suppression is keyed by half and category together.

~~~mermaid
flowchart TD
    Start["scheduled tick"] --> SourceOn{"source half switched on?"}
    SourceOn -- yes --> Inventory["read each configured source, independently"]
    Inventory --> Guard{"a source trusted and safe?"}
    Guard -- no --> Block["record that source blocked or failed, keep its last-known routes"]
    Guard -- yes --> Store["store that source's inventory and geometry"]
    Store --> TargetsOn{"target half switched on?"}
    Block --> TargetsOn
    SourceOn -- no --> TargetsOn
    TargetsOn -- yes --> Read["read the merged stored inventory"]
    Read --> Readable{"readable whole?"}
    Readable -- no --> StateFail["record failed target run and notify"]
    Readable -- yes --> A["reconcile target A"]
    A --> B["reconcile target B"]
    B --> Result{"all targets succeeded?"}
    Result -- yes --> Success["record success and notify"]
    Result -- no --> Failure["record failure and notify"]
    TargetsOn -- no --> Enrich["classify surfaces, if the source half stored"]
    Success --> Enrich
    Failure --> Enrich
    StateFail --> Enrich
~~~

The stored inventory is the handover between the halves. The target half reads
it back rather than fetching a fresh one, and the library it reconciles is the
last one validated as whole.

An inventory that cannot be read back whole fails the target half as a state
failure and deletes nothing.

### Multiple sources

The source half reads every configured source in order, one at a time. Each
source is its own attempt: one source's failure neither stops the others from
being read nor widens into a deletion of another source's routes.

A source that is read and validated successfully has its own share of the
stored inventory replaced wholesale. A source that fails — an unreachable or
invalid read, or an empty result blocked by the gate below — keeps the routes it
was last known to have, and those routes remain part of the merged inventory the
target half reconciles from, authoritative-as-last-known rather than absent.

The empty-source deletion gate is evaluated per source against that source's own
prior route count. A source that had routes and now reports none is blocked for
that source alone unless the operator's empty-source acknowledgement is set, and
every other configured source proceeds independently of it.

A route's identity carries the provider that issued it. Two sources reporting the
same source route ID and stage order store as two distinct routes.

The run's result names which sources were read and which failed or were blocked,
each against its own provider, plus the count of routes each contributed when it
was read successfully. The run's own outcome and failure category are the worst
of what its sources reported. No per-route detail crosses this boundary: a
source result is a provider name, an outcome, a failure category, and a count.

A source inventory is trusted only when the service has a fresh successful login,
all listing pages complete, every new or changed route detail is valid, and each
unchanged route is backed by a prior trusted revision. Every resulting route must
have usable geometry. State-loss recovery fetches every route detail afresh. A
malformed route or incomplete pagination invalidates the whole inventory; it
produces no destination mutation.

For each target, Domestique first reads that target's routes once, keyed by
external ID, and answers every question below from that one reading. One reading
establishes what the target holds for every route at once, so a library where
nothing changed costs one request per target rather than one per route. The
destination's request quota is shared across every configured target.

A route the reading returns without an external ID was not created here, and is
left out entirely. It can never be matched, updated, or deleted.

Working from that reading, Domestique processes desired routes in stable
source-ID and stage-order sequence:

1. Create a missing Wahoo route with its external ID, FIT data, and source
   revision.
2. Update an owned Wahoo route when its source revision changed.
3. Recreate an unchanged desired route if its recorded Wahoo route vanished —
   a vanished route is one the reading did not return.
4. Delete an owned target route only after all required creates and updates for
   that target succeeded.

An update never uses upload-and-delete replacement. If a create or update fails
for a target, the service skips every deletion for that target in that run. A
failure for one target does not prevent an attempted reconciliation of the
other; the aggregate run remains failed unless both succeed.

A fully validated source inventory is saved as trusted even if a Wahoo target
fails. Per-target route mappings change only after their corresponding remote
operation succeeds. This permits a later run to complete only the lagging
target without replaying destructive work.

### Clearing a target

An operator may clear one target: delete every route this service owns there
and forget that slot's route mappings, leaving it as though it had never been
written to.

It is the only deletion the per-target deletion limit does not bound. Nothing
schedules it, and it is reachable only from an explicit manual request naming
that slot.

Its limits are those of any other deletion:

- it deletes only routes carrying an external ID this service issued, so a
  route created by hand in the same account is invisible to it;
- it touches one slot; another target's routes and mappings are unaffected;
- it leaves the library alone — routes, their geometry, and the trusted
  inventory are untouched, so the next reconciliation rebuilds the target from
  stored state rather than from a fresh read; and
- it removes the remote routes before forgetting the local record of them, so a
  clear interrupted partway is safe to repeat: a mapping still naming an
  already-deleted route is re-cleared harmlessly.

A clear waits out a spent request quota rather than ending and resuming later.
It is finished only when the target is empty. On a small quota it may take many
minutes and several refills. The count it reports is accurate even when it ends
early, and repeating it continues from what is left.

It shares the single-flight guard with every other run, so it can neither race a
synchronisation nor be started while one is under way, and a long clear holds
off the scheduled runs behind it until it finishes. It is recorded and notified
as its own run.

## Deletion gates

A target deletion is permitted only when all conditions hold:

- the source inventory is trusted;
- the route was previously tracked for that target and is now absent from the
  desired route set;
- its Wahoo external ID exactly matches the Domestique external-ID format and
  route identity;
- the target has completed all required creates and updates in the run; and
- the deletion plan contains at most five routes for that target.

A source inventory that was populated and becomes empty is blocked while the
empty-source deletion gate is closed. The gate is closed by default. It is
opened on the settings page, takes effect from the next run, does not bypass the
remaining checks, and stays open until it is closed again.

Any larger shrink, missing source authentication, malformed geometry, or
incomplete listing blocks all deletions and yields a safe failure category. The
service never deletes a manually created Wahoo route.

## State-loss recovery

When state is absent or cannot be decrypted, sync is disabled until both
targets are authorised again. The first trusted inventory then reconciles by
looking up the deterministic external IDs for currently desired routes:

- a matching remote route may be adopted into fresh state;
- a missing desired route may be created; and
- no unmatched remote route may be deleted.

## HTTP JSON contract

The route inventory, status, sync, OAuth, browser, asset, media-type, and
error-envelope wire details are authoritative in
[`api/openapi.yaml`](../../api/openapi.yaml). All Domestique JSON properties
there use camelCase; this specification defines the lifecycle and safety rules
those responses describe.

All routes on the served listener except the loopback liveness probe require the
configured Tailnet identity, and the readiness probe is not on that listener at
all. Every state-changing route additionally requires an `Origin` header
equal to the origin the browser UI is served from, as set out in the
[service specification](service.md). Responses use application/json;
charset=utf-8, include Cache-Control: no-store, and never include secrets or raw
upstream errors.

### GET /healthz

Returns 200 with:

~~~json
{"status":"ok"}
~~~

It verifies the process can serve requests. It does not test VeloPlanner,
Wahoo, Pushover, or the database.

### GET /readyz

Served on the readiness listener (`http.readiness_address`, default `:8081`) and
on no other. Returns 200 with:

~~~json
{"status":"ready"}
~~~

while local configuration and the state the process needs are usable, and 503
with a category and nothing more when they are not:

~~~json
{"status":"unready","reason":"state_unreadable"}
~~~

The categories are `state_unreadable`, when the local state cannot be read, and
`state_incomplete`, when a configured target has no state row. Readiness makes
no upstream call of any kind. Target authorisation does not affect it: an
unauthorised slot leaves the service ready.

The two probes answer different questions on different sockets. Liveness reports
that the process is answering HTTP; readiness reports that it can do its job with
what the host gave it. The readiness listener is never fronted by Tailscale Serve
or the tunnel. It is reachable by Docker and host-local health checking and not
by the authenticated public surface.

### GET /v1/status

Returns 200 while the service can read state. The minimum shape is:

~~~json
{
  "ready": true,
  "converged": false,
  "targets": [
    {
      "id":"rider-a",
      "authorisation":"authorized",
      "convergence":"current",
      "routes":{"current":12,"pending":0},
      "last_run":{"completed_at":"2026-08-16T12:00:04Z","result":"succeeded"}
    },
    {
      "id":"rider-b",
      "authorisation":"authorized",
      "convergence":"lagging",
      "routes":{"current":11,"pending":1},
      "last_run":{
        "completed_at":"2026-08-16T12:00:04Z",
        "result":"failed",
        "failure":"destination"
      }
    }
  ],
  "sync": {
    "state":"idle",
    "last_completed_at":"2026-08-16T12:00:00Z",
    "last_result":"succeeded",
    "source_routes":12,
    "created":0,
    "updated":1,
    "deleted":0,
    "schedule":{"source":true,"targets":true},
    "trusted_inventory":{
      "fresh":true,
      "last_success_at":"2026-08-16T12:00:00Z",
      "age_seconds":14400,
      "max_age_seconds":86400
    },
    "surface":{
      "generation":"9f2c41ab77de",
      "built_at":"2026-08-17T03:41:00Z",
      "classified":12,
      "total":12,
      "incomplete":0
    },
    "phases":{
      "source":{
        "last_completed_at":"2026-08-16T12:00:00Z",
        "last_result":"succeeded",
        "source_routes":12,
        "created":0,
        "updated":0,
        "deleted":0
      },
      "targets":{
        "last_completed_at":"2026-08-16T12:00:04Z",
        "last_result":"succeeded",
        "source_routes":12,
        "created":0,
        "updated":1,
        "deleted":0
      }
    },
    "wahoo_rate_limit":{
      "remaining":187,
      "resets_at":"2026-08-16T12:05:00Z"
    }
  }
}
~~~

`wahoo_rate_limit` is Wahoo's most recently advertised request quota, read from
its last response rather than totalled by this service. It is absent until a
request has reached Wahoo and carried a quota header back, and it is shared
across every configured target rather than reported per target. `resets_at` is
absent whenever Wahoo's last response carried no usable reset, or the last one it
did carry has already passed.

Authorisation is one of `not_authorized`, `pending`, `authorized`, or
`needs_reauthorization`. `pending` is derived at read time as the OAuth
lifecycle above describes, and replaces only `not_authorized` or
`needs_reauthorization`. Sync state is `not_ready`, `idle`, `queued`,
`running`, `delayed`, `succeeded`, `failed`, or `blocked`. Timestamps are
RFC 3339 UTC.

Three of those states describe work that has not finished, and each outranks
whatever else `state` would have said. `queued` is a run accepted before its
first half starts, `running` is a half in flight, and `delayed` is a run held
back by the initial startup delay. The hourly interval is never reported as a
delay.

While a run is under way, `state` describes that run rather than the outcome of
the last one to finish. The recorded fields keep their values; only `state`
changes, and `active` appears beside it:

~~~json
"active": {
  "phase": "targets",
  "targets": 2,
  "routes": {"current": 11, "pending": 1}
}
~~~

`active` is present only in those three states. `phase` is absent until a half
has started. `starts_at` appears in `delayed` alone, carrying the instant the
held-back run is due, and never beside a phase. `targets` is how many accounts
are configured, and `routes` is the aggregate of the per-target counts above.
That is the whole of the progress reported. It is derived from local state
alone, and it is counts only: no route is named.

`surface.classified` counts stored routes carrying a classification measured
against the geometry they hold now. A classification of an earlier shape of a
route does not count.

`surface.incomplete` counts routes the most recently completed classification
pass could not classify, and tells those apart from routes waiting their turn.
It answers from the last completed pass alone, whether that pass ran on the
schedule or through `POST /v1/sync/surface`. It is not durable and reads zero
after a restart until a pass has run again.

`surface.generation` and `surface.built_at` name the surface index those
classifications were read from: the build live in this process, not the last one
recorded. A service whose index file did not survive a restart omits them. Both
are absent when no region is configured and until a first build has landed.

`converged` and the per-target `convergence`, `routes`, and `last_run` report
whether every stored route at its current revision has been applied to every
configured target. They are derived from local state alone: the stored source
revision of each route against the revision each target was last given, plus the
recorded result of each target's last reconciliation. A status request never
contacts Wahoo and can be answered while Wahoo is unreachable.

Convergence describes the Wahoo accounts, not physical device download. A head
unit fetches routes from its account on its own schedule, and the service cannot
observe whether it has.

`convergence` is one of:

- `current` — every stored route is on that account at its stored revision, and
  its last reconciliation succeeded.
- `lagging` — routes remain to be written or removed there.
- `failed` — its last reconciliation did not succeed.
- `unauthorized` — the slot is not authorised, so nothing can be written until
  the one-time browser visit happens. This outranks the others.

`routes.current` counts stored routes that account holds at the stored revision.
`routes.pending` counts the remaining stored routes plus any route the account
still holds that has left the library. `last_run` is absent until that account
has been reconciled once, which is not the same as a reconciliation that had
nothing to do. Neither carries a Wahoo identifier, route name, or URL.

`converged` is true only when every configured target reads `current`. An empty
library converges.

`schedule` carries the two switches. A phase under `phases` is absent until that
half has finished a run, and carries `last_failure` with the safe failure
category when its last run did not succeed. The fields outside `phases` describe
the most recent run of either half.

#### Trusted inventory freshness

`trusted_inventory` is always present. `sync.stale_after` is a runtime setting
carrying a default rather than an optional key, so there is always a bound to
report against.

The block reports the age of the trusted source inventory — the stored routes the
source phase last replaced wholesale — against that bound. It is derived from
local state alone: the last source-phase run that recorded a success, compared to
the current instant. Reading it starts no provider work. It is evaluated on every
scheduled tick, whether or not the source phase ran on that tick.

`last_success_at` is absent until a source phase has succeeded once, and `fresh`
is `true` in that case. `age_seconds` is always present, including an age of
exactly zero read immediately after a successful refresh, and is never negative:
a recorded success later than the reporting instant is clamped to zero.
`age_seconds` reads `0` before any success, which the absence of
`last_success_at` distinguishes from a true zero age. `fresh` is
`age_seconds < max_age_seconds`.

A stale reading never relaxes a deletion gate and implies nothing about what any
target holds. Convergence and the deletion gates are unaffected by it.

### PUT /v1/sync/schedule

Sets both switches. The request body names both:

~~~json
{"source":true,"targets":false}
~~~

Returns 200 with the stored state in the same shape. A body naming only one
switch, or carrying an unknown field, is refused with 400. It never starts,
stops, or alters a run in flight.

### GET /v1/sync/runs

Returns one page of the recorded run history, newest first:

~~~json
{
  "runs": [
    {
      "reference": "1a2b3c4d5e6f",
      "phase": "targets",
      "completed_at": "2026-08-18T06:30:00Z",
      "result": "failed",
      "failure": "destination",
      "source_routes": 0,
      "created": 1,
      "updated": 0,
      "deleted": 0
    }
  ],
  "next": "412"
}
~~~

`failure` is present only when the run did not succeed, and is the same safe
category the status response reports. `next` is the cursor for the page after
this one, and is absent when the history ends with this page. A cursor is a
position rather than a name, and still resolves after the run it was taken from
has been pruned.

`limit` sets the page size, defaulting to 20 and capped at 100. `after`
continues from a cursor. A page size outside that range, or a cursor this
service did not issue, is refused with 400 rather than answered with the newest
page.

A record carries only what the run itself came to. It names no route, and
carries no geometry, no Wahoo identity, and nothing a provider said.

### GET /v1/routes

Returns known source metadata in stable source-route order. Each entry names
its provider. It may include a source route title, a route title, source
revision, last synchronised time, and per-target state. It does not include a FIT
payload, unbounded source geometry, OAuth state, Wahoo IDs, route URLs, or edit
controls.

### GET /v1/providers/{provider}/sourceRoutes/{source-route-id}/routes/{stage-order}

Returns one route's metadata object using the same safe field set. Unknown
routes return 404 with the standard error shape. `GET .../geometry` and
`POST .../reprocess` are addressed the same way.

A request naming a provider this service does not serve — anything but
`veloplanner` — is answered 404 rather than passed to state as a lookup.

### Redirected route addresses

Two further shapes of a route's address resolve, each redirecting to the current
one. The redirect is 308, which preserves the reprocess request's `POST` method
and body.

`GET /v1/providers/{provider}/routes/{source-route-id}/routes/{stage-order}`,
its `/geometry`, and its `/reprocess` redirect to the same route under the same
provider.

`GET /v1/routes/{source-route-id}/routes/{stage-order}`, its `/geometry`, and
its `/reprocess` redirect to the same route under `veloplanner` in one hop. The
browser address `GET /routes/{source-route-id}/{stage-order}` redirects to
`GET /routes/veloplanner/{source-route-id}/{stage-order}`.

### Errors

Application errors use:

~~~json
{"error":{"code":"not_found","message":"route was not found"}}
~~~

Messages are stable and safe for display. They must not contain source account
identifiers, route names, provider response text, file paths, tokens, or
credentials. Authentication failures use 401; an authenticated but unpermitted
identity uses 403; a request that does not come from the browser UI's origin
also uses 403; malformed client input uses 400.

The OAuth start, callback, the protected `POST /v1/sync` triggers, the protected
`PUT /v1/sync/schedule` switch, the protected
`POST /v1/providers/{provider}/sourceRoutes/{source-route-id}/routes/{stage-order}/reprocess`
request, and the protected `PUT /v1/settings/*` section writes are the only
state-changing endpoints. A settings write changes what the service does next and
nothing it has stored about a route; it reaches the runtime settings
[the configuration specification](configuration.md#runtime-settings) defines and
no other configuration. There is no HTTP or CLI endpoint for route deletion,
static configuration or secret mutation, or Wahoo target removal.

Every one of them except the OAuth start and callback is refused with 403 unless
its `Origin` header equals the browser UI's origin. Identity is settled first, so
an unverified caller is answered 401 whatever origin it names. The OAuth callback
is outside the check; it is a cross-site redirect, and its state is one-time,
identity-bound, and expiring.

## Notifications

Every terminal run updates the stored run record. The first occurrence of a
failure category in a half sends one failure message; matching failures in that
half are suppressed for six hours. The first following success is the recovery
signal. Suppression is keyed by half and category together.

### The channel switch

`notifications.enabled` is a runtime setting, and it suppresses every signal
below. While it is off nothing is sent: a failure, a blocked run, a recovery, and
a stale inventory are held back exactly as a routine success is.

A suppression window is recorded only by a message that was actually sent. While
the channel is off no suppression window is recorded, so switching the channel
back on finds no window standing behind an unsent alert.

### Trusted inventory staleness

Trusted-inventory staleness is an independent check, evaluated on every tick
regardless of what that tick's phases did, and it starts no provider work. Each
tick compares how long it has been since the source phase last succeeded against
`sync.stale_after`.

The first tick the age crosses `sync.stale_after` sends one message. A further
stale tick is suppressed for six hours, using the same window and the same
`notification_state` category an ordinary phase failure uses, kept apart from any
real phase-and-failure pair. A source phase that then succeeds sends the recovery
signal unconditionally, never held back by the success policy, and closes the
suppression record: a later success with no new stale incident in between sends
nothing further. A service whose source phase has never succeeded has no trusted
inventory to call stale and is never notified as one.

### The success policy

`notifications.success_policy` decides what a *routine* success sends. A routine
success is one whose half's previous recorded run also succeeded. The recovery
signal is not routine, and neither is a failure or a blocked run. No policy
suppresses those three; only the channel switch does.

| Policy | A routine success sends |
| --- | --- |
| `every` | one message carrying the counts that half actually produced |
| `quiet` | nothing |
| `digest` | nothing directly; it is totalled into the next digest |

Whether a success is a recovery is read from the half's own previous recorded
run, so the answer survives a restart and matches the history
`GET /v1/sync/runs` returns. Any recorded outcome other than a success makes the
next one a recovery, not only a failure or a blocked run: a half left needing
onboarding records that it is not ready and notifies nothing, so the alert that
preceded it is still open and the success that follows closes it. A history that
cannot be read is treated as a recovery.

### The digest

Under `digest`, one aggregate message covers each `notifications.digest_interval`.
It carries how many runs of each half succeeded in the period and the totals of
the routes they created, updated, and deleted. It names no route, no target, no
run reference, and no failure category.

The digest is sent by the first pass to finish after the interval elapses, rather
than by a timer of its own. It is considered once per pass, after both halves are
recorded, so a window never closes between the two halves of one pass.

An interval that no run succeeded in sends nothing and still moves the window on.

The first digest of a newly configured policy sends nothing and starts the clock.
The window it starts is durable and belongs to the digest rather than to the
policy in force: switching away from `digest` and back resumes from the last
window this service closed, and the runs in between are reported in the first
digest that follows, as far back as the retained run history reaches. A digest
totals recorded runs and reports no run that pruning has already removed.

The window is bounded by run identity rather than by the clock; two runs of one
pass are recorded within the same second.

Notification content contains only the run result, target count, aggregate
counts, a safe failure category, and — for a message about a single run — that
run's opaque reference. It never contains a route title, source identity, Wahoo
identity, credential, token, secret path, or upstream body. The reference names
which run the message is about and resolves to that run's record in
`GET /v1/sync/runs`.

## Required tests

The implementation test suite must cover at least:

- OAuth callback expiry, reuse, wrong Tailnet identity, and duplicate Wahoo user;
- refresh-token replacement persistence and reauthorisation of only one target;
- missing or malformed source inventory causing zero Wahoo deletions;
- an update failure causing zero deletions for its target;
- a source removal of up to five owned routes and a sixth deletion being blocked;
- one configured source failing while another succeeds, without the failing
  source's last-known routes being deleted or the healthy source's read being
  stopped;
- the empty-source deletion gate blocking only the source that emptied out,
  independently of a sibling source that still has routes;
- manual Wahoo route preservation;
- state loss adopting matching desired external IDs without deleting unknown
  routes;
- each half running, and being triggered, without the other;
- a single-target trigger reconciling only the named slot — keeping the same
  ownership, ordering, update-before-delete, and deletion-limit rules — while
  every other configured target is left completely untouched; an unconfigured
  target name being refused as not found; and a single-target trigger sharing
  the same mutual-exclusion, run recording, and notification path as a full
  target phase;
- a stored inventory that cannot be read back whole causing zero deletions;
- a switched-off half being skipped by the timer and still run by a manual
  trigger;
- a reprocess request rewriting its route on every target while keeping the Wahoo
  route it already owns, and being consumed exactly once;
- a classification pass continuing past a route that failed, and stopping when
  the endpoint reports it has no capacity;
- a status request during a run reporting that run rather than the outcome of
  the last one to finish, and a refused duplicate trigger leaving that report
  unchanged;
- the run history being read a page at a time, refusing a cursor this service
  did not issue, staying bounded as runs accumulate while keeping the newest run
  of each half, and naming runs recorded before references existed;
- JSON responses and Pushover messages containing no secret or raw upstream
  data;
- each success policy delivering what it states for a routine success, while
  `quiet` and `digest` still deliver failures, blocked runs, and the first
  success that ends one — including across a half left needing onboarding;
- a switched-off channel sending none of those, staleness included, and leaving
  no suppression window behind for the moment it is switched back on;
- a digest totalling one interval of successful runs, carrying no run reference
  or target identity, starting its clock without reporting prior history, and
  passing over an interval that nothing succeeded in without leaving its runs to
  be counted twice;
- trusted-inventory staleness: no alert before any successful source run, one
  alert once the age crosses `sync.stale_after`, that alert suppressed for six
  hours, an unconditional recovery message on the next source success, and
  `GET /v1/status` reporting the same age and freshness from local state alone;
  and
- `POST /v1/sync/surface` running a classification pass without reading the
  source or writing a target, refusing to start one alongside a synchronisation
  or another such pass in either direction, and `GET /v1/status` reporting
  `incomplete` from the most recently completed pass, reading zero again after
  a restart until a pass has run.
