# Operator recovery runbook

Domestique runs unattended and reports itself in two places. This guide covers
the uncommon moments when it stops and asks for a person: what you will have
seen, what the service has already guaranteed by stopping, and the smallest safe
way forward.

Every scenario below starts from an observable answer rather than a symptom,
because the service says what it did in stable words that are safe to log and
safe to notify on. Nothing here asks you to open the database, edit stored
state, or delete a Wahoo route by hand.

## Where the answers are

**The status page.** The browser UI at the public hostname is the status page,
and everything it shows comes from `GET /v1/status`, which reads local state
alone — it can be answered while VeloPlanner and Wahoo are both unreachable. The
page says things in plain language: an account reads **Not connected**,
**Behind**, **Last write failed**, or **Up to date**, and a half that did not
succeed reads **Held by a safety gate**, **Did not finish**, or **Did not
start** with a sentence about what to do.

The wire words below — the categories, the authorisation values, the run results
— are what `GET /v1/status` itself returns, which is where to look when the page
does not distinguish two situations that need different answers. Fetching it
needs the same Cloudflare Access assertion the page carries, so it is read most
easily from the browser at `/v1/status`.

**Pushover.** Every terminal run notifies. A success carries its counts. A
failure arrives titled `Domestique sync failed` with a body naming the half and
one stable category:

```text
targets failed: deletion_limit
```

The first occurrence of a category in a half is sent; matching failures in that
half are then suppressed for six hours, and the first following success is the
recovery signal. **Silence is not health** — after a first alert, the status
page's timestamps are the only thing that tells you whether anything has run.

**The readiness probe**, on the host, over loopback:

```sh
curl --fail http://127.0.0.1:8081/readyz
```

It answers `503` with `state_unreadable` or `state_incomplete` when the process
cannot do its job with what the host gave it. It makes no upstream call and
stays ready while a target waits for its one-time authorisation.

| Category | Half | What it means | Section |
| --- | --- | --- | --- |
| `authorization` | targets | A Wahoo account rejected this service's authorisation | [Reconnect a Wahoo account](#reconnect-a-wahoo-account) |
| `empty_source` | source | The library came back empty after previously holding stages | [A deletion was blocked](#a-deletion-was-blocked) |
| `deletion_limit` | targets | More owned routes would go than the per-run maximum | [A deletion was blocked](#a-deletion-was-blocked) |
| `source` | source | The library did not arrive complete or valid | [The library is not being read](#the-library-is-not-being-read) |
| `destination` | targets | A Wahoo operation did not complete | [A write to Wahoo did not complete](#a-write-to-wahoo-did-not-complete) |
| `course` | targets | A stage could not be encoded as a course | [A write to Wahoo did not complete](#a-write-to-wahoo-did-not-complete) |
| `state` | either | Stored state could not be read or written safely | [State cannot be read](#state-cannot-be-read-or-has-been-lost) |

Two of those are gates rather than faults. A **blocked** run is the service
working correctly: nothing was written, nothing was removed, and the way past it
is a deliberate decision. Re-running a blocked half without making that decision
just reaches the same gate again, which is the point of it.

## Reconnect a Wahoo account

**You will have seen** `targets failed: authorization`, and that account reading
**Reconnect needed** on the status page. An account that has never been
connected reads **Not connected** instead; both are fixed by the same visit, but
they are different situations and the page says which it is. `authorisation` in
`GET /v1/status` is `needs_reauthorization` for the first and `not_authorized`
for the second.

**What already held.** Only that slot is affected: the other target is still
attempted in the same run, and a rejected token deletes nothing anywhere. The
refresh token stays encrypted at rest, and access tokens never leave memory.

**In the browser.** The account carries a **Connect** or **Reconnect** link
under **Wahoo accounts** on the status page. It is a link rather than a button
because authorisation is a redirect flow: following it leaves for Wahoo and
comes back. The same flow is reachable directly at the public hostname, which is
what to use when the page itself will not load:

```text
https://<your-public-hostname>/oauth/wahoo/start/<target-id>
```

Sign in as the account that slot belongs to. Wahoo returns to the callback URL
and the service redirects back to the status page. Confirm that the account has
stopped asking to be connected, then press **Write to Wahoo** rather than
waiting for the hour: the run reconciles from what the account actually holds,
so it will catch that target up without replaying anything destructive.

An account reading **Connecting** afterwards means the callback never completed.
That is also why it offers nothing to press: the transaction is single-use, and
starting a second one invalidates the first. It expires ten minutes after it was
started, after which the account reads as it did before and the link comes back
— start it again rather than reloading the callback URL.

**On the host**, only if reconnecting fails outright: `wahoo.redirect_url` must
be exactly the public hostname plus `/oauth/wahoo/callback` and must match the
callback registered with Wahoo, and `wahoo_client_secret` must be current. Both
are static configuration and need a restart.

## A deletion was blocked

**You will have seen** a `blocked` result and, on the status page, **Held by a
safety gate**. Neither gate can be opened from the browser, deliberately: the
UI can show you the gate and run the half again, and the decision to widen a
deletion lives in static configuration on the host.

### `empty_source` — the library came back empty

The source half read zero stages from a library that previously held some, and
`sync.empty_source_deletion` is `"deny"`. The empty inventory was **not
stored**, so the last inventory validated as whole is still what the targets
reconcile against: both accounts are intact and stay intact.

Treat this as the source being wrong until you know otherwise. Open the library
and confirm what is actually there. If it really is meant to be empty — you are
removing the final routes on purpose — set `empty_source_deletion = "allow"` on
the host, restart, run the source half, then set it back to `"deny"`. Nothing
else about the gate is bypassed by that setting.

### `deletion_limit` — more removals than the per-run maximum

One target's reconciliation would have removed more owned routes than
`sync.max_deletions_per_target`. The gate is checked before any create or
update, so **that target was not written to at all** in that run; it stays
behind until the situation is resolved. The other target is reconciled
independently.

Satisfy yourself that the removals are intended. If they are not — routes
vanished from the library by accident — restore them at the source and run the
target half again; no host change is needed and nothing was lost. If they are,
raise `max_deletions_per_target` on the host, restart, run the target half, then
put the value back to what it was.

## The library is not being read

**You will have seen** `source failed: source`, or nothing at all — this is the
scenario where you have to read a timestamp. There is no staleness alert today:
after the first notification, six hours of quiet look exactly like six hours of
success. `sync.phases.source.last_completed_at` on the status page is what says
when the library was last read, and the source half's schedule switch is what
says whether the timer is even trying.

**What already held.** An incomplete or malformed read is never treated as
routes going away: a single invalid route invalidates the whole inventory and no
deletion follows from it. Meanwhile the target half keeps reconciling the last
inventory known to be whole, so an unreadable library does not stop a lagging
account from catching up.

**In the browser.** Check that the source switch is on, then press **Read from
VeloPlanner**. A manual trigger runs its half whether the switch is on or off.

**At the source.** A route whose detail will not parse holds up every route:
correct it in VeloPlanner, then read again. If the credentials themselves are
rejected, they are secret files on the host and need a restart after they
change.

## A write to Wahoo did not complete

**`destination`** means a Wahoo operation did not finish. The account may hold
part of the change, and every deletion for that target was skipped for the run.
Run the target half again once Wahoo is reachable: the reconciler looks up what
each account actually holds by external ID before it creates anything, so a
retry converges rather than duplicating.

**`course`** means one stage could not be encoded as a FIT course. That stage
was not written, and the rest of the run continued. Correct the stage at the
source, then use **Reprocess** on that stage's page: it discards the stored
geometry, the recorded revision, and the surface classification for that stage
alone, and asks for a run of both halves. It rewrites the route the service
already owns rather than deleting and recreating it.

## State cannot be read, or has been lost

**You will have seen** `/readyz` answering `503` with `state_unreadable` or
`state_incomplete`, a `state` category on either half, or a status page showing
every account as **Not connected** and each half as **Did not start**. An
unreadable schedule is reported the same way — as a failed source run — because
"switched off" and "cannot be read" are different answers and a timer must not
act on the second as the first.

**What already held.** Lost state is never authority to delete. When the service
comes back without state, sync stays disabled until every slot is authorised
again; the first trusted inventory then adopts matching remote routes by their
deterministic external ID, creates what is missing, and removes nothing it does
not recognise.

**On the host.** This is entirely a host scenario — the browser has no control
over any of it.

- The state volume must be present and writable by UID and GID `65532`.
- The state encryption key must be the one this database was written with. If
  the two no longer match, stop the container rather than letting it reconcile
  against half its state.
- Never `docker compose down -v`, and never `docker system prune --volumes`:
  both delete the named state volume.

Losing the geometry or surface caches is harmless — they are rebuilt from the
next run and can never authorise a deletion. Losing targets or their stage
mappings is the case above.

**After genuine state loss**, authorise every slot in the browser as in
[Reconnect a Wahoo account](#reconnect-a-wahoo-account), then read the library.
Routes the service can no longer recognise stay on the account: there is
deliberately no feature that removes an unmatched remote route, so anything left
over is removed by hand in Wahoo, by you, or left alone.

## A deployment did not come up

**You will have seen** the deploy script's own Pushover alert and a failed CI
job. The script gates on both probes: if the new image does not answer
`/healthz` and then `/readyz` within a minute, cannot read its state, or
publishes anything other than a loopback port, **it has already restored the
previous digest and restarted**. Confirm that on the host before doing anything
else:

```sh
curl --fail http://127.0.0.1:8080/healthz
curl --fail http://127.0.0.1:8081/readyz
```

To go back deliberately, on a host that has the script:

```sh
sudo /usr/local/lib/domestique/domestique-deploy.sh --rollback
```

That returns to the digest the host ran before the last change. Passing an
explicit `sha256:` digest instead goes to any published image. On a host without
the script, or when the script is what broke, pin `DOMESTIQUE_IMAGE` to a known
digest and run `docker compose --env-file .env up -d` from the deployment
directory.

**No rollback restores old state.** Every path leaves the named state volume
alone, which is what makes going back safe and also what means a rollback
undoes the code and nothing the code wrote.

## Surface classification is not completing

**You will have seen** the status page saying surface is classified for fewer
stages than the library holds, or `sync.surface` reporting the same. The
classification pass writes one log line per pass carrying counts and whether it
ran to the end, and nothing else.

**What already held.** Classification is enrichment. It belongs to no half,
never changes a run's outcome, never touches a Wahoo account, and cannot
authorise a deletion. A stage without it is a stage the map draws without
surface colouring.

**Usually, nothing.** The pass runs after a read that stored something new and
skips stages already classified against both their current content hash and the
generation of the current index, so a library that has just been rebuilt against
a new map reclassifies itself over the next run or two.

**When a shortfall persists**, the index is what to look at. `sync.surface` on
the status page names the `generation` and `built_at` of the build the
classifications were read from; both are absent when no index is loaded. Compare
that against the build log — the service writes one line when an index is
rebuilt, one when it finds every region unchanged, and one when a build fails.
A build that fails sends a single notification and then stays quiet for a week,
so the log is where a run of failures is visible.

An empty `sync.surface` generation on a service that has regions configured means
no index is live yet: either the first build has not run (it waits a few minutes
after start), or the last build's file did not survive. Either way the next
scheduled build fills it in.

**When nothing is classified on purpose**, `regions` in the host's `[surface]`
configuration is empty. That is the default, and it switches the whole feature
off: no extract is downloaded and no index is built.

A single stage classified wrongly is a **Reprocess** away; re-planning a stage
reclassifies it automatically, because the cached ranges describe coordinates
that were replaced.

## What this runbook does not cover

There is no state backup, no key rotation, and no remote route cleanup.
There is also no HTTP or CLI path to delete a route, mutate configuration, or
remove a target: everything in this guide is either a browser action the service
already offers or a change to static configuration on the host followed by a
restart.
