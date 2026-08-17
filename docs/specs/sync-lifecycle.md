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
| sync run | start, end, terminal state, aggregate counts, safe failure category | none |
| notification state | last delivered failure category and suppression deadline | none |

OAuth state is stored as a digest. Refresh tokens are encrypted before being
written. Access tokens, OAuth authorisation codes, CSRF state values, raw
upstream bodies, and FIT bytes are never persisted.

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

The configured Tailnet user can request `POST /v1/sync` to start an immediate
synchronization. It uses the same reconciliation, durable run record, and
Pushover notification path as scheduled work. The service returns `202` only
when no scheduled or manual run is active; otherwise it returns `409` without
starting duplicate provider work.

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
single in-process lock prevents overlap. A second trigger records no work and
returns without modifying state.

~~~mermaid
flowchart TD
    Start["scheduled run"] --> Login["fresh VeloPlanner login"]
    Login --> Inventory["fetch complete source inventory"]
    Inventory --> Guard{"trusted and safe?"}
    Guard -- no --> Block["record blocked run and notify"]
    Guard -- yes --> A["reconcile target A"]
    A --> B["reconcile target B"]
    B --> Result{"all targets succeeded?"}
    Result -- yes --> Success["record success and notify"]
    Result -- no --> Failure["record failure and notify"]
~~~

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
identity. Responses use application/json; charset=utf-8, include
Cache-Control: no-store, and never include secrets or raw upstream errors.

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
    "deleted":0
  }
}
~~~

Authorisation is one of "not_authorised", "pending", "authorised", or
"needs_reauthorisation". Sync state is "not_ready", "idle", "running",
"succeeded", "failed", or "blocked". Timestamps are RFC 3339 UTC.

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
identity uses 403; malformed client input uses 400.

The OAuth start and callback are the only state-changing endpoints. There is no
HTTP or CLI endpoint for manual sync, route deletion, configuration mutation, or
Wahoo target removal.

## Notifications

Every terminal run updates the stored run record. A success sends one Pushover
message with aggregate create/update/delete counts. The first occurrence of a
failure category sends one failure message; matching failures are suppressed for
six hours. The first following success is the recovery signal.

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
  routes; and
- JSON responses and Pushover messages containing no secret or raw upstream
  data.
