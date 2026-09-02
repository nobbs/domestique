# Operator recovery runbook

Domestique runs unattended and reports itself in two places. This guide covers
the uncommon moments when it stops and asks for a person: what you will have
seen, what the service has already guaranteed by stopping, and the smallest safe
way forward.

Every scenario below starts from an observable answer rather than a symptom. The
service says what it did in stable words that are safe to log and safe to notify
on. Nothing here asks you to open the database, edit stored state, or delete a
Wahoo route by hand.

## Where the answers are

**The status page.** The browser UI at the public hostname is the status page,
and everything it shows comes from `GET /v1/status`, which reads local state
alone and can be answered while VeloPlanner and Wahoo are both unreachable. The
page says things in plain language: an account reads **Not connected**,
**Behind**, **Last write failed**, or **Up to date**, and a half that did not
succeed reads **Held by a safety gate**, **Did not finish**, or **Did not
start** with a sentence about what to do.

The wire words below — the categories, the authorisation values, the run results
— are what `GET /v1/status` itself returns, which is where to look when the page
does not distinguish two situations that need different answers. Fetching it
needs the same session the page carries, so it is read most easily from the
browser at `/v1/status`.

**Pushover.** What a task can be announced for is a fixed list it declares, and
each entry has its own switch on the settings page: the failure categories, a
routine `succeeded`, the `recovered` that follows anything else, and `stale`
when the task has gone too long without succeeding. An alert nobody has ruled on
is sent, so a new one reaches you before you have heard of it. The switch for
the whole channel is separate and is not a quieter setting: off, a failure goes
unsent too.

A message arrives titled for its task — `Domestique sync` — with a body naming
the task, what it was over, and one stable category:

```text
sync:target failed: deletion_limit run=4f2a9c1d08ab
```

The run reference at the end is what to look up if you need the full record.
The first occurrence of a category is sent; matching ones are then suppressed
for six hours, and the first following success arrives as `recovered`. Silencing
the routine `succeeded` leaves `recovered` to come through, so a quiet
deployment still tells you when an alert is over. **Silence is not health** —
with `succeeded` switched off, silence is also what a healthy service sounds
like, and the status page's timestamps are what tell the two apart.

**The readiness probe**, on the host, over loopback:

```sh
curl --fail http://127.0.0.1:8081/readyz
```

It answers `503` with `state_unreadable` or `state_incomplete` when the process
cannot do its job with what the host gave it. It makes no upstream call and
stays ready while a target waits for its one-time authorisation.

| Category | Task | What it means | Section |
| --- | --- | --- | --- |
| `authorization` | sync:target, sync:clear | A Wahoo account rejected this service's authorisation | [Reconnect a Wahoo account](#reconnect-a-wahoo-account) |
| `empty_source` | sync:source | The library came back empty after previously holding routes | [A deletion was blocked](#a-deletion-was-blocked) |
| `deletion_limit` | sync:target, sync:clear | More owned routes would go than the per-run maximum | [A deletion was blocked](#a-deletion-was-blocked) |
| `source` | sync:source | The library did not arrive complete or valid | [The library is not being read](#the-library-is-not-being-read) |
| `destination` | sync:target, sync:clear | A Wahoo operation did not complete | [A write to Wahoo did not complete](#a-write-to-wahoo-did-not-complete) |
| `course` | sync:target, sync:clear | A route could not be encoded as a course | [A write to Wahoo did not complete](#a-write-to-wahoo-did-not-complete) |
| `state` | sync:source, sync:target, sync:clear | Stored state could not be read or written safely | [State cannot be read](#state-cannot-be-read-or-has-been-lost) |

Two of those are gates rather than faults. A **blocked** run is the service
working correctly: nothing was written, nothing was removed, and the way past it
is a deliberate decision. Re-running a blocked half without making that decision
reaches the same gate again.

## I cannot sign in

**You will have seen** a browser stuck at `/auth/login`, redirected straight
back to it after choosing an identity provider, or shown the service's own 403
page instead of the site. Nothing here is a run outcome, so none of it reaches
Pushover or `GET /v1/status`; the answer is on screen.

**The issuer is unreachable.** `/auth/login` loads but choosing GitHub (or
whichever connection is enabled) hangs or errors before returning. Confirm the
host can reach the configured Auth0 tenant at all, and that `auth.auth0.domain`
in `config.toml` is the tenant host with no scheme and no path. This is a host
problem, not a browser one.

**The subject signed in is not allowed.** The 403 page itself is the
diagnostic: it names the exact `sub` value Auth0 asserted for that sign-in,
and it is the only place this service ever shows that value — it is never
written to a log. Compare it against `allowed_subjects` in `config.toml`. A
value that is not there yet is exactly [first-time setup](auth0.md#first-time-setup):
copy it in and restart. A value that looks like it should already be there
means the list and the running configuration have drifted — confirm the file
that was actually deployed, and that the service restarted after it changed,
since this section is a file setting rather than a runtime one.

**The callback URL does not match.** A sign-in that returns from the identity
provider to an error page, or to the wrong host entirely, usually means the
Auth0 application's Allowed Callback URLs does not hold
`https://<host>/auth/callback` for the host actually in front of the service,
or that `http.browser_origin_url` names a different host than the one the
browser is actually on. The two must agree exactly, including scheme; a
Tailnet URL left over from a previous deployment is a common cause of this
one.

**The certificate is not trusted.** A browser warning about a self-signed
certificate, on a host that has never served a valid one, means Traefik has not
completed issuance rather than that anything about sign-in is wrong — and
because `__Host-` cookies need real TLS, nothing below will work until it has.
TLS-ALPN-01 needs the public DNS record to resolve to this host directly, with
443 open and nothing else terminating TLS in front; a record still proxied by a
CDN fails for that reason. `docker compose logs traefik` carries the ACME
error, and issuance retries on its own once the cause is fixed.

**The cookie is refused.** `__Host-` cookies are a browser rule, not a
service one: they are refused outright over plain HTTP, on any origin that is
not exactly the one that set them, and if the response tries to set a
`Domain` attribute at all. If sign-in appears to succeed and then immediately
looks signed out again, confirm the reverse proxy is actually terminating TLS
for the request that reaches the browser — a proxy silently falling back to
plain HTTP, or a load balancer in front of it that terminates TLS a second
time and forwards plain HTTP, both produce exactly this symptom.

## Reconnect a Wahoo account

**You will have seen** `targets failed: authorization`, and that account reading
**Reconnect needed** on the status page. An account that has never been
connected reads **Not connected** instead. Both are fixed by the same visit, and
the page says which it is. `authorisation` in `GET /v1/status` is
`needs_reauthorization` for the first and `not_authorized` for the second.

**What already held.** Only that slot is affected: the other target is still
attempted in the same run, and a rejected token deletes nothing anywhere. The
refresh token stays encrypted at rest, and access tokens never leave memory.

**In the browser.** The account carries a **Connect** or **Reconnect** link
under **Wahoo accounts** on the status page. It is a link rather than a button:
authorisation is a redirect flow that leaves for Wahoo and comes back. The same
flow is reachable directly at the public hostname, which is what to use when the
page itself will not load:

```text
https://<your-public-hostname>/oauth/wahoo/start/<target-id>
```

That link needs a signed-in Domestique session first: the Wahoo flow's
one-time state is bound to the subject that started it, so opening it from a
browser that has not signed in redirects to `/auth/login` instead. See [I
cannot sign in](#i-cannot-sign-in) if that is what happens.

Sign in as the Wahoo account that slot belongs to — a separate sign-in, at
Wahoo's own login page. Wahoo returns to the callback URL
and the service redirects back to the status page. Confirm that the account has
stopped asking to be connected, then press **Write to Wahoo** rather than
waiting for the hour: the run reconciles from what the account actually holds,
and catches that target up without replaying anything destructive.

An account reading **Connecting** afterwards means the callback never completed.
It offers nothing to press: the transaction is single-use, and starting a second
one invalidates the first. It expires ten minutes after it was started, after
which the account reads as it did before and the link comes back. Start it again
rather than reloading the callback URL.

**On the settings page**, only if reconnecting fails outright: the Wahoo client
ID and client secret must be current, and the callback registered with Wahoo
must be exactly `http.browser_origin_url` plus `/oauth/wahoo/callback`. The
first two are settings and take effect on the next attempt; the origin is on the
host and needs a restart.

## Nothing runs, and nothing failed

**You will have seen** a service that answers both probes and shows a settings
page saying what is still missing, with no run in its history and no
notification of any kind. That is a deployment that has not been configured yet
rather than one that broke, and it is the state every new deployment starts in.
The host's file carries only the listeners, the identity gate and the state, and
everything a run needs — the source libraries and their accounts, the Wahoo
application and its client secret, and at least one target slot — is entered on
the settings page.

**What the service has already guaranteed.** A scheduled run finds nothing
configured and does nothing, rather than reaching an upstream as nobody or
treating an unread library as an empty one. No inventory is stored, so the
deletion gate is never approached. The readiness probe reports ready throughout.

**The way forward** is the settings page, which names each missing setting.
Filling them in takes effect on the next run: there is no restart, and nothing
to edit on the host. A target slot named there is ready for the one-time OAuth
onboarding immediately; see [Reconnect a Wahoo account](#reconnect-a-wahoo-account)
for what that looks like.

## A deletion was blocked

**You will have seen** a `blocked` result and, on the status page, **Held by a
safety gate**. The two gates differ in what you can do about them: the
empty-source gate is a switch on the settings page, behind a confirmation, and
the per-run deletion limit is a constant nothing can raise.

### `empty_source` — the library came back empty

The source half read zero routes from a library that previously held some, and
the empty-source deletion gate denies it. The empty inventory was **not
stored**, so the last inventory validated as whole is still what the targets
reconcile against: both accounts are intact and stay intact.

Treat this as the source being wrong until you know otherwise. Open the library
and confirm what is actually there. If it really is meant to be empty — you are
removing the final routes on purpose — open **Settings → Service settings**,
turn on *Let an empty library delete a target's routes*, confirm the dialog, run
the source half, and turn it off again. It takes effect on that next run, with
no restart, and it stays on until you turn it off; nothing closes it for you.
Nothing else about the gate is bypassed by it.

### `deletion_limit` — more removals than the per-run maximum

One target's reconciliation would have removed more than five owned routes,
which is the per-run limit. It is a constant rather than a setting and cannot be
raised. The gate is checked before any create or update, so **that target was
not written to at all** in that run; it stays behind until the situation is
resolved. The other target is reconciled independently.

Satisfy yourself that the removals are intended. If they are not — routes
vanished from the library by accident — restore them at the source and run the
target half again; nothing was lost. If they are, and the removals are more than
five, the way through is **Delete all routes…** on that account: it deletes
every route this service owns from that slot, which the limit does not bound,
and the next run rebuilds the slot from the stored library. That costs a full
re-upload of the routes that were meant to stay.

## The library is not being read

**You will have seen** `source failed: source`, or a staleness alert once the
trusted inventory has gone longer than `sync.stale_after` — 24 hours by default
— without a successful source run. That alert is sent once, then suppressed for
six hours, and the next successful source run sends an unconditional recovery
message. `sync.phases.source.last_completed_at` on the status page is what says
when the library was last read, `sync.trusted_inventory` reports the same age
against its bound, and the source half's schedule switch says whether the timer
is trying.

**What already held.** An incomplete or malformed read is never treated as
routes going away: a single invalid route invalidates the whole inventory and no
deletion follows from it. The target half keeps reconciling the last inventory
known to be whole, so an unreadable library does not stop a lagging account from
catching up.

**In the browser.** Check that the source switch is on, then press **Read from
VeloPlanner**. A manual trigger runs its half whether the switch is on or off.

**At the source.** A route whose detail will not parse holds up every route:
correct it in VeloPlanner, then read again. If the credentials themselves are
rejected, retype them on the settings page. The page never shows you the stored
one, so a rotated password is entered in full, and the next run uses it.

## A write to Wahoo did not complete

**`destination`** means a Wahoo operation did not finish. The account may hold
part of the change, and every deletion for that target was skipped for the run.
Run the target half again once Wahoo is reachable: the reconciler looks up what
each account actually holds by external ID before it creates anything, so a
retry converges rather than duplicating.

**`course`** means one route could not be encoded as a FIT course. That route
was not written, and the rest of the run continued. Correct the route at the
source, then use **Reprocess** on that route's page: it discards the stored
geometry, the recorded revision, and the surface classification for that route
alone, and asks for a run of both halves. It rewrites the route the service
already owns rather than deleting and recreating it.

## State cannot be read, or has been lost

**You will have seen** `/readyz` answering `503` with `state_unreadable` or
`state_incomplete`, a `state` category on either half, or a status page showing
every account as **Not connected** and each half as **Did not start**. An
unreadable schedule is reported the same way, as a failed source run: switched
off and cannot be read are different answers.

**What already held.** Lost state is never authority to delete. When the service
comes back without state, sync stays disabled until every slot is authorised
again; the first trusted inventory then adopts matching remote routes by their
deterministic external ID, creates what is missing, and removes nothing it does
not recognise.

**On the host.** This is entirely a host scenario; the browser has no control
over any of it.

- The state volume must be present and writable by UID and GID `65532`.
- The state encryption key must be the one this database was written with. If
  the two no longer match, stop the container rather than letting it reconcile
  against half its state.
- Never `docker compose down -v`, and never `docker system prune --volumes`:
  both delete the named state volume.

Losing the geometry or surface caches is harmless: they are rebuilt from the
next run and can never authorise a deletion. Losing targets or their route
mappings is the case above.

**After genuine state loss**, authorise every slot in the browser as in
[Reconnect a Wahoo account](#reconnect-a-wahoo-account), then read the library.
Routes the service can no longer recognise stay on the account. There is no
feature that removes an unmatched remote route, so anything left over is removed
by hand in Wahoo, by you, or left alone.

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
alone. A rollback undoes the code and nothing the code wrote.

## Surface classification is not completing

**You will have seen** the status page saying surface is classified for fewer
routes than the library holds, or `sync.surface` reporting the same. The
classification pass writes one log line per pass carrying counts and whether it
ran to the end, and nothing else.

**What already held.** Classification is enrichment. It belongs to no half,
never changes a run's outcome, never touches a Wahoo account, and cannot
authorise a deletion. A route without it is a route the map draws without
surface colouring.

**Usually, nothing.** The pass runs after a read that stored something new and
skips routes already classified against both their current content hash and the
generation of the current index, so a library that has just been rebuilt against
a new map reclassifies itself over the next run or two.

**When a shortfall persists**, the index is what to look at. `sync.surface` on
the status page names the `generation` and `built_at` of the build the
classifications were read from; both are absent when no index is loaded. Compare
that against the build log: the service writes one line when an index is
rebuilt, one when it finds every region unchanged, and one when a build fails.
A build that fails sends a single notification and then stays quiet for a week,
so the log is where a run of failures is visible.

An empty `sync.surface` generation on a service that has regions configured means
no index is live yet: either the first build has not run — it waits a few minutes
after start — or the last build's file did not survive. Either way the next
scheduled build fills it in.

**When nothing is classified on purpose**, the region list under **Settings →
Service settings** is empty. That is the default, and it switches the whole
feature off: no extract is downloaded and no index is built. Adding a region
there does not build anything by itself; the next rebuild on the configured
schedule does, and routes are classified on the pass after that.

A single route classified wrongly is a **Reprocess** away. Re-planning a route
reclassifies it automatically.

## What this runbook does not cover

There is no state backup, no key rotation, and no remote route cleanup.
There is also no HTTP or CLI path to delete a route. Everything in this guide is
either a browser action the service already offers — including the settings
page, which reaches the deletion gate, the staleness bound, the notification
settings, the basemap list, the surface regions, the source libraries, the Wahoo
application and its target slots, the ride model, and every credential those
reach their upstreams with — or a change to the host's configuration file, which
holds only the listeners, the identity gate and the state, followed by a
restart.

A credential can be replaced from that page but not read from it, and removing
one is not offered at all. A blank field means keep rather than clear.
