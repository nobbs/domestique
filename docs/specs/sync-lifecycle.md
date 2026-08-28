# Domestique sync lifecycle specification

**Status:** accepted

This is a subordinate specification to [the service contract](service.md).
It defines the durable lifecycle of OAuth onboarding, synchronisation, and the
read-only HTTP JSON surface.

## Stable identities

A source stage is identified by the triple:

~~~text
provider + source route ID + stage order
~~~

The provider is the upstream a stage's route ID was issued by. VeloPlanner is
the only provider a stage has ever come from; naming it explicitly is what lets
a second provider issue its own route IDs later without colliding with
VeloPlanner's.

Its deterministic Wahoo external ID is:

~~~text
domestique:<provider>:<route-id>:stage:<stage-order>
~~~

A VeloPlanner stage's external ID renders exactly as it always has:

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
| stage surface | cached surface classification of one stored geometry, as index ranges plus matched length, against the content hash and the surface-index generation it was measured for | none |
| surface index | when the last index build finished and which generation it produced | none |
| sync run | opaque reference, half, start, end, terminal state, aggregate counts, safe failure category | none |
| sync schedule | whether the timer may start each half | none |
| reprocess request | one stage an operator has asked to have redone | none |
| notification state | last delivered failure category and suppression deadline | none |
| runtime settings | the settings an operator edits while the service runs, including the basemap list and the surface regions | none |

Every recorded run carries an opaque reference: random bytes, meaningless on
their own, and the one thing about a run that may be said out loud. It is what a
notification about a single run names and what an operator matches a served
record against. A message that is about no single run — a digest — names none.

Run records are the only history this service keeps, and a service that runs
every hour would otherwise grow them forever. They are bounded to a fixed number
of the most recent runs, pruned in the same transaction that records a run. The
newest run of each half is never pruned, whatever its age: that record is what
`GET /v1/status` reports for a half, and a half switched off while the other
keeps running would otherwise lose its answer. Pruning touches nothing else — it
can never affect the trusted inventory, target stage mappings, OAuth state, or a
deletion gate, none of which are derived from this history.

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

The stage surface cache is filled by a separate pass: automatically, after every
successful source read, and on request through the manual retry below. Unlike
the geometry, it needs data no run holds: the ways lying under each stage, read
from the locally built surface index. It runs after a source read rather than
delaying it, because getting routes onto a device is what a synchronization is
for. It belongs to no half and cannot change any outcome.

The whole inventory is walked in one pass. A local index answers a stage in a few
milliseconds, so there is nothing to spread over several runs; a stage already
classified against both its current content hash and the generation of the live
index is skipped without reading anything. A rebuilt index therefore reclassifies
the library rather than serving last month's reading of a resurfaced road, and it
does so over the run or two after the build.

A pass with no index behind it does nothing at all — no stage is recorded as
unsurveyed on the strength of a map that has not been built yet. That is the
normal state of a deployment with no regions configured, and the brief state of
one whose first build has not landed.

A stage that fails does not end the pass. Each stage gets its own attempt
whatever happened to the one before, so one unreadable stage cannot starve every
stage behind it — the inventory is always walked in the same order, and an early
failure that stopped the pass would stop it at the same place forever.

Because none of this may fail a run, a pass that leaves work undone writes one
log line carrying the counts and whether it ran to the end — never a stage name,
never geometry, never anything the endpoint said. `GET /v1/status` reports two
counts alongside it: how many stored stages carry a classification of the
geometry they currently hold, durably, and how many the most recently completed
pass could not classify, which is not — it answers from that one pass alone and
reads zero again after a restart until a pass has run. What is neither count is
simply waiting its turn. Without the second, a stage that fails every pass would
be indistinguishable from one nobody has asked about — both absent from the
first alike. A stage whose
geometry has been re-planned is reclassified, because the cached ranges are
positions in the coordinate array that was replaced.

## OAuth lifecycle

The configuration has one or two target slots. Each begins in
"not_authorized"; automatic sync does not start until every configured slot is
"authorized". These are the values the service serves, so they are spelled as
the wire spells them even where the surrounding prose is not.

Three of the four are stored on the slot. "pending" is not, and is derived when
the status is read, from an OAuth transaction that has neither expired nor been
consumed. It is a state of the flow rather than of the slot: the slot still
holds whatever it held before the browser left, and the flow ends by expiry,
denial, or exchange failure as often as by success — none of which anything
tells the service about, so a stored fourth state would have no transition out
of it. A slot that is already "authorized" stays so while a fresh flow runs,
because its refresh token keeps working until that flow replaces it.

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
   authorisation code and state are thereby removed from the browser URL, and
   the operator arrives back at the page that sent them, where that slot is
   described.

A callback failure returns a generic error response. It never echoes the code,
state, token, Wahoo account identity, or upstream response.

## Manual trigger

A manual trigger is a state change, so it carries the browser-origin
requirement of every state-changing route.

The configured Tailnet user can request `POST /v1/sync` to start an immediate
synchronization of both halves, or `POST /v1/sync/source` and
`POST /v1/sync/targets` to start one. `POST /v1/sync/targets/{target}`
reconciles exactly one configured target slot, catching up or diagnosing that
Wahoo account without touching the source read or any other target; `{target}`
must name a configured slot or the request is refused as not found, exactly as
the OAuth start route refuses one. Each uses the same reconciliation, durable
run record, and Pushover notification path as scheduled work — a single-target
request keeps the same ownership, ordering, update-before-delete, and
deletion-limit rules the target phase always applies to that slot. The service
returns `202` only when no scheduled or manual run is active — a full
synchronization and a single-target one share the same mutual exclusion, so
neither may start while the other is in flight; otherwise it returns `409`
without starting duplicate provider work. A refused trigger changes nothing at
all, the status included: the run already in flight stays the one described, and
no second run state comes into being.

A manual trigger runs its half whether or not the schedule is allowed to start
it. The switches decide what happens unattended; a request is an operator who
has already decided.

## Retrying enrichment

`POST /v1/sync/surface` asks for one immediate classification pass, on the same
terms as a manual trigger: it carries the browser-origin requirement, and the
service returns `202` only when no synchronization or other classification pass
is active, `409` otherwise. It shares that one mutual exclusion with every
other manual trigger above — `POST /v1/sync`, `POST /v1/sync/source`,
`POST /v1/sync/targets`, and `POST /v1/sync/targets/{target}` — none of the
five may run while another is in flight.

Unlike the other four, it never reads VeloPlanner and never writes a Wahoo
target. It reclassifies the stages already stored against the local surface
index and cache alone, the same pass a successful source read runs
automatically. This is what makes it safe to offer on its own: retrying
enrichment can neither create, update, nor delete a route on any target, so it
carries none of the safety gates or notification traffic a synchronization
does.

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
   "needs_reauthorization"; the other target is still attempted.

limits and request-response boundaries before the next target call begins.
All Wahoo calls are serial across configured targets. The client observes
advertised limits and request-response boundaries before the next target call
begins.
rate limits and waits or ends the run safely; it never issues parallel retries.

An advertised quota that reaches zero holds the next request back until it
refills, whether or not the destination said when that will be — a reported
reset of zero means the responding request was not itself limited, not that the
quota is already back. When the wait would exceed what one run holds itself
open for, the run ends and reports the limit rather than sleeping through it.
Each stage is recorded as its own write succeeds, so the next scheduled run
resumes from stored state and the library converges over successive runs
instead of one run stalling for hours.

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
    SourceOn -- yes --> Inventory["read each configured source, independently"]
    Inventory --> Guard{"a source trusted and safe?"}
    Guard -- no --> Block["record that source blocked or failed, keep its last-known stages"]
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
it back rather than fetching a fresh one: the library it reconciles is the last
one validated as whole, which is what lets a lagging target catch up without
asking the source a second question it has already answered.

An inventory that cannot be read back whole fails the target half as a state
failure and deletes nothing. A partial library is indistinguishable from a
library whose missing stages are meant to be deleted, and the difference is not
one a reconciler may guess at.

### Multiple sources

The source half reads every configured source in order, one at a time, and
each source is its own attempt: one source's failure does not stop the others
from being read, and never widens into a deletion of another source's stages.

A source that is read and validated successfully has its own share of the
stored inventory replaced wholesale, exactly as a single source always has. A
source that fails — an unreachable or invalid read, or an empty result blocked
by the gate below — keeps the stages it was last known to have, and those
stages remain part of the merged inventory the target half reconciles from,
authoritative-as-last-known rather than absent. Writing a failing source's
successful half as though it were the whole run is exactly the destructive read
[the safety rules](../../AGENTS.md) forbid, arriving through a second source.

The empty-source deletion gate is evaluated per source, against that source's
own prior stage count: a source that previously had stages and now reports
none is blocked for that source alone unless the operator's empty-source
acknowledgement is set, and every other configured source proceeds
independently of it.

A stage's identity already carries the provider that issued it, so two sources
reporting the same route ID and stage order store as two distinct stages, not
one — there is no cross-source collision to guard against.

The run's result names which sources were read and which failed or were
blocked, each against its own provider, plus the count of stages each
contributed when it was read successfully. The run's own outcome and failure
category are the worst of what its sources reported, in the same vocabulary a
run has always used, and no per-stage detail crosses this boundary: a source
result is a provider name, an outcome, a failure category, and a count.

A source inventory is trusted only when the service has a fresh successful login,
all listing pages complete, every new or changed route detail is valid, and each
unchanged route is backed by a prior trusted revision. Every resulting stage must
have usable geometry. State-loss recovery fetches every route detail afresh. A
malformed route or incomplete pagination invalidates the whole inventory; it
produces no destination mutation.

For each target, Domestique first reads that target's routes once, keyed by
external ID, and answers every question below from that one reading. It
establishes the same fact a per-stage lookup did — what the target actually
holds right now, by external ID — for every stage at once, so a library where
nothing changed costs one request per target rather than one per stage. The
destination's request quota is shared across every configured target, and a
per-stage lookup spent it in proportion to library size.

A route the reading returns without an external ID was not created here, and is
left out entirely: it can therefore never be matched, updated, or deleted.

Working from that reading, Domestique processes desired stages in stable
source-ID and stage-order sequence:

1. Create a missing Wahoo route with its external ID, FIT data, and source
   revision.
2. Update an owned Wahoo route when its source revision changed.
3. Recreate an unchanged desired stage if its recorded Wahoo route vanished —
   a vanished route is one the reading did not return.
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

### Clearing a target

An operator may clear one target: delete every route this service owns there
and forget that slot's stage mappings, leaving it as though it had never been
written to. It exists because a target can end up in a state not worth
reconciling one route at a time, and repairing that by hand is worse than
rebuilding it.

It is the only deletion the per-target deletion limit does not bound. That
limit exists so an unattended run cannot act on a bad inventory; a clear is an
operator saying, about one named slot, that all of it should go. For the same
reason nothing schedules it, and it is reachable only from an explicit manual
request naming that slot.

What it may not do is unchanged:

- it deletes only routes carrying an external ID this service issued, so a
  route created by hand in the same account is invisible to it;
- it touches one slot; another target's routes and mappings are unaffected;
- it leaves the library alone — source stages, their geometry, and the trusted
  inventory are untouched, so the next reconciliation rebuilds the target from
  stored state rather than from a fresh read; and
- it removes the remote routes before forgetting the local record of them, so a
  clear interrupted partway is safe to repeat: a mapping still naming an
  already-deleted route is re-cleared harmlessly, where the reverse would
  strand routes nothing remembers owning.

Unlike a scheduled run, a clear waits out a spent request quota rather than
ending and resuming later. A run the schedule started can afford to stop and
continue on its next pass; a clear was asked for once and is finished only when
the target is empty, so stopping partway would leave an operator pressing the
same destructive control until the count reached zero. On a small quota it may
therefore take many minutes and several refills. The count it reports is real
even when it ends early: those routes are gone, and repeating it continues from
what is left.

It shares the single-flight guard with every other run, so it can neither race
a synchronization nor be started while one is under way — which also means a
long clear holds off the scheduled runs behind it until it finishes. It is
recorded and notified as its own run, so a cleared target appears in history as
the deletion it was rather than as an unexplained drop.

## Deletion gates

A target deletion is permitted only when all conditions hold:

- the source inventory is trusted;
- the stage was previously tracked for that target and is now absent from the
  desired stage set;
- its Wahoo external ID exactly matches the Domestique external-ID format and
  source-stage identity;
- the target has completed all required creates and updates in the run; and
- the deletion plan contains at most five routes for that target.

A previously populated source inventory that becomes empty is blocked while the
empty-source deletion gate denies it, which is how it is seeded and where an
operator leaves it. Opening it is a deliberate act for removing the final source
routes, taken on the settings page and in force from the next run; it does not
bypass the remaining checks, and it stays open until it is closed again.

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
no upstream call of any kind, and it is deliberately indifferent to target
authorisation: an unauthorised slot is a deployment waiting for its one-time
browser visit, not a process that cannot run.

The two probes answer different questions on different sockets. Liveness says the
process is answering HTTP; readiness says it can do its job with what the host
gave it. The readiness listener is never fronted by Tailscale Serve or the
tunnel, which is what keeps it available to Docker and host-local health checking
and unavailable to the authenticated public surface.

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
      "stages":{"current":12,"pending":0},
      "last_run":{"completed_at":"2026-08-16T12:00:04Z","result":"succeeded"}
    },
    {
      "id":"rider-b",
      "authorisation":"authorized",
      "convergence":"lagging",
      "stages":{"current":11,"pending":1},
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
    "source_stages":12,
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
    },
    "wahoo_rate_limit":{
      "remaining":187,
      "resets_at":"2026-08-16T12:05:00Z"
    }
  }
}
~~~

`wahoo_rate_limit` is Wahoo's own most recently advertised request quota — a
live reading of its last response, not a count this service totals itself.
It is absent until a request has actually reached Wahoo and carried a quota
header back, and shared across every configured target rather than reported
per target. `resets_at` is itself absent whenever Wahoo's last response
carried no usable reset, or the last one it did carry has already passed.

Authorisation is one of "not_authorized", "pending", "authorized", or
"needs_reauthorization"; "pending" is derived at read time as the OAuth
lifecycle above describes, and replaces only "not_authorized" or
"needs_reauthorization". Sync state is "not_ready", "idle", "queued",
"running", "delayed", "succeeded", "failed", or "blocked". Timestamps are
RFC 3339 UTC.

Three of those states describe work that has not finished, and each outranks
whatever else `state` would have said. "queued" is a run accepted before its
first half starts, "running" is a half in flight, and "delayed" is a run
deliberately held back — the initial startup delay — rather than one waiting
its turn on the interval. The interval itself is never reported as a delay: it
is this service's cadence, and a state that never settled would leave a reader,
and anything polling on their behalf, with nothing to wait for.

Reporting a finished run's outcome while another is under way would announce a
result for work that has produced none, and nothing in the response would say
which run it described. The recorded fields stay where they are; only `state`
changes, and `active` appears beside it:

~~~json
"active": {
  "phase": "targets",
  "targets": 2,
  "stages": {"current": 11, "pending": 1}
}
~~~

`active` is present only in those three states. `phase` is absent until a half
has started, and `starts_at` appears in "delayed" alone, carrying the instant
the held-back run is due — never beside a phase, because a run under way is not
the run being waited for. `targets` is how many accounts are configured, and
`stages` is the aggregate of the per-target counts above. That is the whole of the progress reported: it is
derived from local state alone, so watching a run costs no provider call, and
it is counts only, so no route is named.

`surface` counts how many stored stages carry a classification measured against
the geometry they hold now; a classification of an earlier shape of a stage does
not count, because it describes a line the map no longer draws.

`incomplete` counts stages the most recently completed classification pass could
not classify, and is what tells those apart from stages simply waiting their
turn — a stage that fails every pass would otherwise look exactly like one
nobody has asked about, since both are equally absent from `classified`. It
answers from the last completed pass alone, whether that pass ran on the
schedule or through `POST /v1/sync/surface`; it is not durable and reads zero
after a restart until a pass has run again.

`generation` and `built_at` name the surface index those classifications were
read from — the build that is live in this process, not merely the last one
recorded, so a service whose index file did not survive a restart says so by
omitting them. Both are absent when no region is configured and until a first
build has landed.

`converged` and the per-target `convergence`, `stages`, and `last_run` answer
whether every stored stage at its current revision has been applied to every
configured target. They are derived from local state alone — the stored source
revision of each stage against the revision each target was last given, plus the
recorded result of each target's last reconciliation. A status request never
contacts Wahoo, so it can be answered while Wahoo is unreachable.

This is convergence of the Wahoo accounts, not a claim about physical device
download: a head unit fetches routes from its account on its own schedule, and
the service cannot see whether it has.

`convergence` is one of:

- "current" — every stored stage is on that account at its stored revision, and
  its last reconciliation succeeded.
- "lagging" — stages remain to be written or removed there.
- "failed" — its last reconciliation did not succeed.
- "unauthorized" — the slot is not authorised, so nothing can be written until
  the one-time browser visit happens. This outranks the others, because a
  lagging count there says nothing an operator can act on differently.

`stages.current` counts stored stages that account holds at the stored revision.
`stages.pending` counts the remaining stored stages plus any stage the account
still holds that has left the library — outstanding removal is outstanding work.
`last_run` is absent until that account has been reconciled once, which is not
the same as a reconciliation that had nothing to do. Neither carries a Wahoo
identifier, route name, or URL.

`converged` is true only when every configured target reads "current". An empty
library converges: there is nothing left to apply.

`schedule` carries the two switches. A phase under `phases` is absent until that
half has finished a run, and carries `last_failure` with the safe failure
category when its last run did not succeed. The fields outside `phases` describe
the most recent run of either half and are kept so an existing reader does not
break.

#### Trusted inventory freshness

`trusted_inventory` is always present: `sync.stale_after` is a runtime setting
carrying a default rather than an optional key, so there is always a bound to
report against. It reports the age of the trusted source inventory — the stored
stages the source phase last replaced wholesale — against that bound, derived
from local state alone: the last source-phase run that recorded a success,
compared to the current instant. Reading it starts no provider work and is
evaluated on every scheduled tick, whether or not the source phase ran on that
tick, because the inventory can go stale while the schedule has that half
switched off.

`last_success_at` is absent until a source phase has ever succeeded: a service
with no trusted inventory yet has nothing to call stale, which is a different
claim from a stale one. `fresh` is `true` in that case. `age_seconds` is
always present, including an age of exactly zero read immediately after a
successful refresh, and is never negative: a recorded success later than the
reporting instant is clamped to zero rather than read as a claim about the
future. `age_seconds` reads `0` before any success too, which `last_success_at`
being absent is what tells apart from a true zero age. `fresh` is
`age_seconds < max_age_seconds`.

A stale reading here never relaxes a deletion gate or implies that any target
holds current state; convergence and the deletion gates are unaffected and are
read exactly as described above.

### PUT /v1/sync/schedule

Sets both switches. The request body names both:

~~~json
{"source":true,"targets":false}
~~~

Returns 200 with the stored state in the same shape. A body naming only one
switch, or carrying an unknown field, is refused with 400: a half-named schedule
would leave the other switch at whatever the caller assumed. It never starts,
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
      "source_stages": 0,
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
position rather than a name, so a page still resolves after the run it was taken
from has been pruned.

`limit` sets the page size, defaulting to 20 and capped at 100 so one request
cannot read the whole retained window. `after` continues from a cursor. A page
size outside that range, or a cursor this service did not issue, is refused with
400 rather than answered with the newest page, which would silently restart a
walk through the history.

A record carries only what the run itself came to. It names no route, carries no
geometry, no Wahoo identity, and nothing a provider said.

### GET /v1/routes

Returns known source metadata in stable source-stage order. Each entry names its
provider. It may include a route title, stage title, source revision, last
synchronised time, and per-target state. It does not include a FIT payload,
unbounded source geometry, OAuth
state, Wahoo IDs, route URLs, or edit controls.

### GET /v1/providers/{provider}/routes/{source-route-id}/stages/{stage}

Returns one route-stage metadata object using the same safe field set. Unknown
stages return 404 with the standard error shape. `GET .../geometry` and
`POST .../reprocess` are addressed the same way.

A request naming a provider this service has never served — anything but
`veloplanner` today — is answered 404 rather than passed to state as a lookup
that could never find anything.

### Legacy two-segment stage routes

`GET /v1/routes/{source-route-id}/stages/{stage}`,
`GET /v1/routes/{source-route-id}/stages/{stage}/geometry`, and
`POST /v1/routes/{source-route-id}/stages/{stage}/reprocess` — the shape
every route had before a second provider existed — redirect with 308 to the
same stage under `veloplanner`, preserving method and body. 308 rather than
301 or 302 is what keeps the reprocess request's `POST` and body intact across
the redirect. The browser address `GET /routes/{source-route-id}/{stage}`
redirects the same way, to `GET /routes/veloplanner/{source-route-id}/{stage}`.

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
`PUT /v1/sync/schedule` switch, the protected
`POST /v1/providers/{provider}/routes/{source-route-id}/stages/{stage}/reprocess`
request, and the protected `PUT /v1/settings/*` section writes are the only
state-changing endpoints. A settings write changes what the service does next and
nothing it has stored about a route; it reaches the runtime settings
[the configuration specification](configuration.md#runtime-settings) defines and
no other configuration. There is no HTTP or CLI endpoint for route deletion,
static configuration or secret mutation, or Wahoo target removal.

Every one of them except the OAuth start and callback is refused with 403 unless
its `Origin` header equals the browser UI's origin. Identity is settled first, so
an unverified caller is answered 401 whatever origin it names. The OAuth callback
stays outside the check because it is a cross-site redirect by design; its
one-time, identity-bound, expiring state is what protects it.

## Notifications

Every terminal run updates the stored run record. The first occurrence of a
failure category in a half sends one failure message; matching failures in that
half are suppressed for six hours. The first following success is the recovery
signal. Suppression is keyed by half and category together: the same category
failing in both halves is two problems, and each is worth one alert.

### The channel switch

`notifications.enabled` is a runtime setting rather than a policy, and it is the
one thing that suppresses every signal below. While it is off nothing is sent: a
failure, a blocked run, a recovery, and a stale inventory are held back exactly
as a routine success is. It is written down here as plainly as it is offered on
the settings page, because an operator reading it as a quieter success policy
would be switching off the alerts notifications exist for.

Nothing is written down as sent while it is off, either. A suppression window is
recorded only by a message that actually went out, so turning the channel back
on cannot find a six-hour window standing behind an alert nobody ever heard.

### Trusted inventory staleness

A source phase can stop producing a trusted inventory without a newly visible
incident once its own failure category is already suppressed — a schedule left
switched off, or a source failing the same way every tick, both look identical
to an operator after the first alert. Trusted-inventory staleness is a second,
independent check for exactly that: every tick compares how long it has been
since the source phase last succeeded against `sync.stale_after`, regardless of
what that tick's phases did, and this check starts no provider work.

The first tick the age crosses `sync.stale_after` sends one message; a further
stale tick is suppressed for six hours, the same window and the same
`notification_state` category an ordinary phase failure uses, kept apart from
any real phase-and-failure pair. A source phase that then succeeds sends the
recovery signal unconditionally — never held back by the success policy — the
same way an ordinary recovery is, and closes the suppression record so the
recovery is a one-shot signal: a later success with no new stale incident in
between sends nothing further. A service whose source phase has never once
succeeded has no trusted inventory to call stale, and is never notified as one.

### The success policy

`notifications.success_policy` decides what a *routine* success sends. A routine
success is one whose half's previous recorded run also succeeded; the recovery
signal is not routine, and neither is a failure or a blocked run. No policy can
suppress those three, which is the point of separating them; only the channel
switch above can.

| Policy | A routine success sends |
| --- | --- |
| `every` | one message carrying the counts that half actually produced |
| `quiet` | nothing |
| `digest` | nothing directly; it is totalled into the next digest |

Whether a success is a recovery is read from the half's own previous recorded
run, so the answer survives a restart and cannot drift from the history an
operator reads back in `GET /v1/sync/runs`. Any recorded outcome other than a
success makes the next one a recovery, not only a failure or a blocked run: a
half left needing onboarding records that it is not ready and notifies nothing,
so the alert that preceded it is still open and the success that follows is what
closes it. A history that cannot be read is treated as a recovery: one message
too many costs an operator a line, and a withheld recovery costs them the end of
an alert they were sent.

### The digest

Under `digest`, one aggregate message covers each `notifications.digest_interval`.
It carries how many runs of each half succeeded in the period and the totals of
the routes they created, updated, and deleted. It names no route, no target, no
run reference, and no failure category — a digest is counts and the period they
cover, and nothing that identifies what was touched.

The digest is sent by the first pass to finish after the interval elapses,
rather than by a timer of its own, so a service that is not running sends no
empty messages. It is considered once per pass, after both halves are recorded,
so a window never closes between the two halves of one pass and leaves the
second reported in no digest at all.

An interval that no run succeeded in sends nothing and still moves the window
on. Leaving it where it was would report two intervals of work under one
interval's heading, and an all-zero message would defeat the policy the operator
chose it for.

The first digest of a newly configured policy sends nothing and starts the
clock. The alternative is an opening message covering however much history the
database happens to hold, which is not the period the operator asked for. The
window it starts is durable and belongs to the digest, not to the policy in
force: switching away from `digest` and back resumes from the last window this
service closed rather than starting a new one, so the runs in between are
reported in the first digest that follows — as far back as the retained run
history reaches. A digest totals recorded runs and can report no run that
pruning has already removed, which is what the upper bound on the period is for
and what a long spell under another policy can still outrun.

That window is bounded by run identity rather than by the clock, because two
runs of one pass are recorded within the same second and a timestamp cannot say
which of them a digest already covered.

Notification content contains only the run result, target count, aggregate
counts, a safe failure category, and — for a message about a single run — that
run's opaque reference. It never contains a route title, source identity, Wahoo
identity, credential, token, secret path, or upstream body. The reference says
which run the message is about without saying anything about it, and resolves to
that run's record in `GET /v1/sync/runs`.

## Required tests

The implementation test suite must cover at least:

- OAuth callback expiry, reuse, wrong Tailnet identity, and duplicate Wahoo user;
- refresh-token replacement persistence and reauthorisation of only one target;
- missing or malformed source inventory causing zero Wahoo deletions;
- an update failure causing zero deletions for its target;
- a source removal of up to five owned stages and a sixth deletion being blocked;
- one configured source failing while another succeeds, without the failing
  source's last-known stages being deleted or the healthy source's read being
  stopped;
- the empty-source deletion gate blocking only the source that emptied out,
  independently of a sibling source that still has stages;
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
- a reprocess request rewriting its stage on every target while keeping the Wahoo
  route it already owns, and being consumed exactly once;
- a classification pass continuing past a stage that failed, and stopping when
  the endpoint reports it has no capacity;
- a status request during a run reporting that run rather than the outcome of
  the last one to finish, and a refused duplicate trigger leaving that report
  unchanged;
- the run history being read a page at a time, refusing a cursor this service
  did not issue, staying bounded as runs accumulate while keeping the newest run
  of each half, and naming runs recorded before references existed; and
- JSON responses and Pushover messages containing no secret or raw upstream
  data;
- each success policy delivering what it states for a routine success, while
  `quiet` and `digest` still deliver failures, blocked runs, and the first
  success that ends one — including across a half left needing onboarding;
- a switched-off channel sending none of those, staleness included, and leaving
  no suppression window behind for the moment it is switched back on; and
- a digest totalling one interval of successful runs, carrying no run reference
  or target identity, starting its clock without reporting prior history, and
  passing over an interval that nothing succeeded in without leaving its runs to
  be counted twice; and
- trusted-inventory staleness: no alert before any successful source run, one
  alert once the age crosses `sync.stale_after`, that alert suppressed for six
  hours, an unconditional recovery message on the next source success, and
  `GET /v1/status` reporting the same age and freshness from local state alone;
  and
- `POST /v1/sync/surface` running a classification pass without reading the
  source or writing a target, refusing to start one alongside a synchronization
  or another such pass in either direction, and `GET /v1/status` reporting
  `incomplete` from the most recently completed pass, reading zero again after
  a restart until a pass has run.
