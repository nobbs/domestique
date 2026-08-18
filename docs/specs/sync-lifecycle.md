# Domestique sync lifecycle specification

**Status:** accepted v1 design

This is a subordinate specification to [the service contract](service.md).
It defines the durable lifecycle of OAuth onboarding, synchronisation, and the
read-only HTTP JSON surface.

## Stable identities

A source stage is identified by the pair:

~~~text
VeloPlanner route ID + stage order
~~~

Its deterministic Wahoo external ID is:

~~~text
domestique:veloplanner:<route-id>:stage:<stage-order>
~~~

Titles, descriptions, content hashes, and Wahoo route IDs are mutable metadata;
they never identify a stage. The external-ID format is not configurable.

## Durable records

SQLite persists these conceptual records. Concrete table names are an
implementation detail.

| Record | Purpose | Sensitive fields |
| --- | --- | --- |
| target | configured target slot, Wahoo user ID, authorisation state | encrypted refresh token |
| oauth transaction | target slot, caller identity, state digest, expiry, consumed status | none |
| source stage | stable identity, source revision, metadata, content hash | none |
| target stage | source-stage key, target, external ID, Wahoo route ID, last applied revision | none |
| trusted inventory | complete validated source-stage set and observed time | none |
| stage geometry | cached titles, geometry, length, and extent for the map view | none |
| stage surface | cached surface classification of one stored geometry, as index ranges plus matched length, against the content hash it was measured for | none |
| sync run | half, start, end, terminal state, aggregate counts, safe failure category | none |
| sync schedule | whether the timer may start each half | none |
| reprocess request | one stage an operator has asked to have redone | none |
| notification state | last delivered failure category and suppression deadline | none |

OAuth state is stored as a digest. Refresh tokens are encrypted before being
written. Access tokens, OAuth authorisation codes, CSRF state values, raw
upstream bodies, and FIT bytes are never persisted.

The stage geometry cache is written during the same transaction that stores the
trusted inventory, from data the run already holds, so it needs no extra source
request. A stage whose content hash is unchanged is **not rewritten**, which
keeps an unchanged library from rewriting the whole cache every hour; rows whose
stage has left the inventory are pruned.

It is deliberately a separate record from the source stage. The source-stage set
backs the deletion guard and is replaced wholesale each run, whereas this cache
serves only the map view and may be dropped at any time. Losing it degrades the
map until the next run and can never affect sync safety or authorise a deletion.

The stage surface cache is filled by a separate pass, after every half a tick
intended to run and only when the source half stored something new. Unlike the
geometry, it needs data no run holds: a request per stage to the configured
Overpass endpoint. It comes last because getting routes onto a device is what a
synchronization is for, and enrichment must never delay that. It belongs to no
half, cannot change any outcome, is bounded to a few stages per pass so a first
sync of a large library neither stalls nor leans on a volunteer-run server, and
skips any stage already classified against its current content hash.

A stage that fails does not end the pass. A public endpoint refuses a share of
queries under load, and a stage is classified only if every one of the queries
covering it lands, so a long stage fails far more often than a short one. Ending
the pass at the first failure let one such stage starve every stage after it, in
that pass and in every pass after, because the inventory is always walked in the
same order. Each stage gets its own attempt whatever happened to the one before.

Rate limiting is the exception and does end the pass: it is the endpoint saying
it has no capacity, which is an answer about the server rather than about a
stage. A refused query is retried a small number of times first, with a pause
that grows after each refusal and honours any the endpoint asked for.

Because none of this may fail a run, a pass that leaves work undone writes one
log line carrying the counts and whether it ran to the end — never a stage name,
never geometry, never anything the endpoint said — and `GET /v1/status` reports
how many stored stages carry a classification of the geometry they currently
hold. Without those, a stage that fails every pass is indistinguishable from one
nobody has asked about. A stage whose geometry has been
re-planned is reclassified, because the cached ranges are positions in the
coordinate array that was replaced.

## OAuth lifecycle

The configuration has one or two target slots. Each begins in
"not_authorised"; automatic sync does not start until every configured slot is
"authorised".

~~~mermaid
stateDiagram-v2
    [*] --> not_authorised
    not_authorised --> pending: protected start request
    pending --> authorised: valid callback and token exchange
    pending --> not_authorised: expiry, denial, or exchange failure
    authorised --> needs_reauthorisation: Wahoo invalidates refresh token
    needs_reauthorisation --> pending: protected start request
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
   consumed, and redirects with 303 See Other to /v1/status. The authorisation
   code and state are thereby removed from the browser URL.

A callback failure returns a generic error response. It never echoes the code,
state, token, Wahoo account identity, or upstream response.

## Manual trigger

A manual trigger is a state change, so it carries the browser-origin
requirement of every state-changing route.

The configured Tailnet user can request `POST /v1/sync` to start an immediate
synchronization of both halves, or `POST /v1/sync/source` and
`POST /v1/sync/targets` to start one. Each uses the same reconciliation, durable
run record, and Pushover notification path as scheduled work. The service returns
`202` only when no scheduled or manual run is active; otherwise it returns `409`
without starting duplicate provider work.

A manual trigger runs its half whether or not the schedule is allowed to start
it. The switches decide what happens unattended; a request is an operator who
has already decided.

## Reprocessing one stage

A stage carries three derived answers the service reuses while they still look
current: the geometry it derived and stored, the revision it last pushed to each
target, and its surface classification. Each of those caches exists so an
unchanged library costs nothing to keep in sync.

A reprocess request is the operator saying one of them is wrong. It discards all
three for that stage and starts a synchronization of both halves, so the stage is
read again, derived again, encoded again, pushed to every target regardless of
the revision recorded there, and classified again.

It is not a delete and not a create. The Wahoo route identity is kept, so the
route the service already owns is rewritten in place — the same operation an
ordinary update performs, through the same ownership rules. The request touches
no source data: VeloPlanner is read, never written.

The request is recorded before the run is asked for, and survives a refused
start. A run already in flight may be past that stage or may not include it, so
the request waits for a pass that will honour it. It is consumed by the pass that
honours it, so it is met exactly once.

A request for a stage that is not in the stored inventory is refused. There would
be nothing to redo, and a request nothing will ever consume is worse than an
answer.

## Schedule switches

Two durable switches decide what the timer starts: one for the source half, one
for the target half. Both are on until an operator turns one off, which is what
every deployment did before they existed.

They are the operator's answer to two different situations. A library being
re-planned should not be pushed to a device mid-edit, and a device that must not
change today should not stop the map from staying current. Neither case is worth
editing configuration on the host and restarting the service, and neither is
worth stopping the service, which would also stop the browser UI.

A switch governs the next tick only. It never stops a run in flight, never
changes what a manual trigger does, and never relaxes a safety gate: a half that
does run, runs exactly as it always did.

A schedule that cannot be read starts nothing, and is recorded and notified as a
failed source run. "Off" and "unreadable" are different answers, and a timer must
not act on the second as though it were the first.

## Wahoo token use

Wahoo access and refresh tokens are handled per target:

1. Immediately before a Wahoo API request, the service decrypts that target's
   refresh token and obtains a fresh access token.
2. It transactionally writes the replacement refresh token before making a
   later request, so a crash cannot leave only a stale token on disk.
3. It performs the required API request with the in-memory access token.
4. A rejected refresh token sets only that target to
   "needs_reauthorisation"; the other target is still attempted.

limits and request-response boundaries before the next target call begins.
All Wahoo calls are serial across configured targets. The client observes
advertised limits and request-response boundaries before the next target call
begins.
rate limits and waits or ends the run safely; it never issues parallel retries.

## Sync lifecycle

A healthy process schedules one delayed startup run, then one hourly run. A
single in-process lock prevents overlap across both halves. A second trigger
records no work and returns without modifying state.

Each half is a run of its own: its own record, its own outcome, its own
notification. Failure suppression is keyed by half and category together, so a
library that has been failing to load all morning is never the reason a target
stops reporting that it can no longer be written to.

~~~mermaid
flowchart TD
    Start["scheduled tick"] --> SourceOn{"source half switched on?"}
    SourceOn -- yes --> Login["fresh VeloPlanner login"]
    Login --> Inventory["fetch complete source inventory"]
    Inventory --> Guard{"trusted and safe?"}
    Guard -- no --> Block["record blocked source run and notify"]
    Guard -- yes --> Store["store trusted inventory and geometry"]
    Store --> TargetsOn{"target half switched on?"}
    SourceOn -- no --> TargetsOn
    Block --> TargetsOn
    TargetsOn -- yes --> Read["read stored inventory"]
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
it back rather than fetching a fresh one: the library it reconciles is the last
one validated as whole, which is what lets a lagging target catch up without
asking the source a second question it has already answered.

An inventory that cannot be read back whole fails the target half as a state
failure and deletes nothing. A partial library is indistinguishable from a
library whose missing stages are meant to be deleted, and the difference is not
one a reconciler may guess at.

A source inventory is trusted only when the service has a fresh successful login,
all listing pages complete, every new or changed route detail is valid, and each
unchanged route is backed by a prior trusted revision. Every resulting stage must
have usable geometry. State-loss recovery fetches every route detail afresh. A
malformed route or incomplete pagination invalidates the whole inventory; it
produces no destination mutation.

For each target, Domestique processes desired stages in stable source-ID and
stage-order sequence:

1. Create a missing Wahoo route with its external ID, FIT data, and source
   revision.
2. Update an owned Wahoo route when its source revision changed.
3. Recreate an unchanged desired stage if its recorded Wahoo route vanished.
4. Delete an owned target stage only after all required creates and updates for
   that target succeeded.

An update never uses upload-and-delete replacement. If a create or update fails
for a target, the service skips every deletion for that target in that run. A
failure for one target does not prevent an attempted reconciliation of the
other; the aggregate run remains failed unless both succeed.

A fully validated source inventory is saved as trusted even if a Wahoo target
fails. Per-target stage mappings change only after their corresponding remote
operation succeeds. This permits a later run to complete only the lagging
target without replaying destructive work.

## Deletion gates

A target deletion is permitted only when all conditions hold:

- the source inventory is trusted;
- the stage was previously tracked for that target and is now absent from the
  desired stage set;
- its Wahoo external ID exactly matches the Domestique external-ID format and
  source-stage identity;
- the target has completed all required creates and updates in the run; and
- the deletion plan contains at most five routes for that target.

A previously populated source inventory that becomes empty is blocked when
sync.empty_source_deletion equals "deny". Setting it to "allow" is a deliberate
static deployment action for removing the final source routes; it does not
bypass the remaining checks.

Any larger shrink, missing source authentication, malformed geometry, or
incomplete listing blocks all deletions and yields a safe failure category. The
service never deletes a manually created Wahoo route.

## State-loss recovery

When state is absent or cannot be decrypted, sync is disabled until both
targets are authorised again. The first trusted inventory then reconciles by
looking up the deterministic external IDs for currently desired stages:

- a matching remote route may be adopted into fresh state;
- a missing desired route may be created; and
- no unmatched remote route may be deleted.

This makes state loss safe at the cost of leaving old routes for an explicit
future reconciliation feature.

## HTTP JSON contract

All routes except the loopback liveness probe require the configured Tailnet
identity. Every state-changing route additionally requires an `Origin` header
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

### GET /v1/status

Returns 200 while the service can read state. The minimum shape is:

~~~json
{
  "ready": true,
  "targets": [
    {"id":"rider-a","authorisation":"authorised"},
    {"id":"rider-b","authorisation":"authorised"}
  ],
  "sync": {
    "state":"idle",
    "last_completed_at":"2026-08-16T12:00:00Z",
    "last_result":"succeeded",
    "source_stages":12,
    "created":0,
    "updated":1,
    "deleted":0,
    "schedule":{"source":true,"targets":true},
    "surface":{"classified":12,"total":12},
    "phases":{
      "source":{
        "last_completed_at":"2026-08-16T12:00:00Z",
        "last_result":"succeeded",
        "source_stages":12,
        "created":0,
        "updated":0,
        "deleted":0
      },
      "targets":{
        "last_completed_at":"2026-08-16T12:00:04Z",
        "last_result":"succeeded",
        "source_stages":12,
        "created":0,
        "updated":1,
        "deleted":0
      }
    }
  }
}
~~~

Authorisation is one of "not_authorised", "pending", "authorised", or
"needs_reauthorisation". Sync state is "not_ready", "idle", "running",
"succeeded", "failed", or "blocked". Timestamps are RFC 3339 UTC.

`surface` counts how many stored stages carry a classification measured against
the geometry they hold now; a classification of an earlier shape of a stage does
not count, because it describes a line the map no longer draws.

`schedule` carries the two switches. A phase under `phases` is absent until that
half has finished a run, and carries `last_failure` with the safe failure
category when its last run did not succeed. The fields outside `phases` describe
the most recent run of either half and are kept so an existing reader does not
break.

### PUT /v1/sync/schedule

Sets both switches. The request body names both:

~~~json
{"source":true,"targets":false}
~~~

Returns 200 with the stored state in the same shape. A body naming only one
switch, or carrying an unknown field, is refused with 400: a half-named schedule
would leave the other switch at whatever the caller assumed. It never starts,
stops, or alters a run in flight.

### GET /v1/routes

Returns known source metadata in stable source-stage order. It may include a
route title, stage title, source revision, last synchronised time, and per-target
state. It does not include a FIT payload, unbounded source geometry, OAuth
state, Wahoo IDs, route URLs, or edit controls.

### GET /v1/routes/{source-route-id}/stages/{stage}

Returns one route-stage metadata object using the same safe field set. Unknown
stages return 404 with the standard error shape.

### Errors

Application errors use:

~~~json
{"error":{"code":"not_found","message":"route stage was not found"}}
~~~

Messages are stable and safe for display. They must not contain source account
identifiers, route names, provider response text, file paths, tokens, or
credentials. Authentication failures use 401; an authenticated but unpermitted
identity uses 403; a request that does not come from the browser UI's origin
also uses 403; malformed client input uses 400.

The OAuth start, callback, the protected `POST /v1/sync` triggers, the protected
`PUT /v1/sync/schedule` switch, and the protected
`POST /v1/routes/{source-route-id}/stages/{stage}/reprocess` request are the only
state-changing endpoints. There is no HTTP or CLI endpoint for route deletion,
configuration mutation, or Wahoo target removal.

Every one of them except the OAuth start and callback is refused with 403 unless
its `Origin` header equals the browser UI's origin. Identity is settled first, so
an unverified caller is answered 401 whatever origin it names. The OAuth callback
stays outside the check because it is a cross-site redirect by design; its
one-time, identity-bound, expiring state is what protects it.

## Notifications

Every terminal run updates the stored run record. A success sends one Pushover
message carrying the counts its half actually produced. The first occurrence of a
failure category in a half sends one failure message; matching failures in that
half are suppressed for six hours. The first following success is the recovery
signal. Suppression is keyed by half and category together: the same category
failing in both halves is two problems, and each is worth one alert.

Notification content contains only the run result, target count, aggregate
counts, and a safe failure category. It never contains a route title, source
identity, Wahoo identity, credential, token, secret path, or upstream body.

## Required tests

The implementation test suite must cover at least:

- OAuth callback expiry, reuse, wrong Tailnet identity, and duplicate Wahoo user;
- refresh-token replacement persistence and reauthorisation of only one target;
- missing or malformed source inventory causing zero Wahoo deletions;
- an update failure causing zero deletions for its target;
- a source removal of up to five owned stages and a sixth deletion being blocked;
- manual Wahoo route preservation;
- state loss adopting matching desired external IDs without deleting unknown
  routes;
- each half running, and being triggered, without the other;
- a stored inventory that cannot be read back whole causing zero deletions;
- a switched-off half being skipped by the timer and still run by a manual
  trigger;
- a reprocess request rewriting its stage on every target while keeping the Wahoo
  route it already owns, and being consumed exactly once;
- a classification pass continuing past a stage that failed, and stopping when
  the endpoint reports it has no capacity; and
- JSON responses and Pushover messages containing no secret or raw upstream
  data.
