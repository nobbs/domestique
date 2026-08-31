# Domestique task layer specification

**Status:** accepted

This specification is subordinate to [the service contract](service.md). It
defines what the service runs in the background, what those activities may not
run beside, and when they run.

## Purpose

Every background activity in the service is a task. One layer owns registering
them, keeping them off each other's state, starting them on a schedule, and
waiting for them at shutdown. No package starts a goroutine of its own to do
recurring work.

## Vocabulary

A **task** is a named activity. An **argument** narrows one attempt to a part of
what the task covers — a target slot, for instance — and a task that covers only
one thing takes an empty argument. An **invocation** is one attempt: its task,
its argument, and what started it. An **outcome** is what an attempt came to; a
**detail** is a stable, safe-to-display reason for it, and never carries provider
response text, a route name, or an upstream identifier.

The outcomes are:

~~~text
succeeded   the attempt did what it set out to do
failed      a safe failure stopped it
blocked     a safety gate refused the work
not_ready   a setting is unset, or a target still awaits onboarding
skipped     it did no work: see its detail for which kind of busy stopped it
cancelled   shutdown ended it; never a fault
unchanged   it ran, checked, and found nothing new
~~~

## Run history

Every attempt is recorded, with one exception: one that shutdown `cancelled`
cannot write during the shutdown that ended it.

A refusal is recorded, and says which kind of busy stopped it: this service
working on the very same thing, or working on something else that held what the
attempt needed. They are the answer to why something did not run, and that is
only answerable afterwards if it was written down.

A chain link is the exception. One asking for work already under way is dropped
rather than refused, because the work is happening — which is what the link
wanted — and the rest of the chain counts it as run.

History is bounded per task, so a task running every few minutes cannot evict
the history of one running weekly. The most recent attempt over each argument is
kept whatever its age: it is what that argument last came to.

A history that cannot be written is logged and does not change what the attempt
came to. Losing a row costs a stale line on a status page.

## Mutual exclusion

A task declares the **resources** an attempt needs, each held either shared or
exclusively. Two attempts wanting the same resource run together only when both
want it shared. An attempt takes its whole set or none of it, so no attempt ever
waits while holding part of what another one needs.

A resource that is held refuses the attempt rather than queueing it. Every task
here would rather run again later than pile attempts on the same state, and a
refused attempt reports `skipped`.

Resources describe state, not tasks. Everything that reads or writes the trusted
inventory takes `inventory`, whichever task asked, so no two of those overlap. A
surface index build takes `surface-index`, touches neither, and runs beside them.

## Concurrency

A task's concurrency limit is how many of its attempts may run at once. It
defaults to one, so registering a task never introduces parallelism by accident.
The limit and the resource set are taken together or not at all.

## Chains

An attempt reports the invocations its own result made necessary. What follows
what is decided by whoever knows the outcome, rather than declared where nobody
can see it.

An attempt releases its resources before its chain starts. A link wanting what
its parent held would otherwise be refused by its own parent, which is the usual
case rather than the exception.

A link asking for work already under way is dropped, not refused: the work is
happening, which is what the link wanted. A link losing a resource to something
unrelated is a refusal, and is recorded as one.

Because links are chosen while a task runs, nothing can reject a cycle when a
task is registered. One chain carries one set of what it has already run and
refuses to run any of it again — a chain runs in order, so branches share that
set rather than each starting from a copy. A depth limit sits behind it for a
chain whose arguments keep changing.

These chains are registered:

~~~text
sync            stored an inventory  ->  surface:annotate
surface:index   installed a new map  ->  surface:annotate
~~~

A rebuilt index makes every stored classification stale, and nothing else
notices that.

## Scheduling

A task with a schedule runs unasked. The schedule answers when a task last due
at one instant is due again, and is read again before each wait, so an operator
editing a cadence changes the next gap rather than needing a restart.

The first run of a process waits out an initial delay. A gap that has already
elapsed when the previous run finished starts the next run at once rather than
queueing the ones it missed, and the cadence carries on from there.

A task with no schedule runs only when something asks for it.

### Fixed gaps and calendars

A fixed gap has no time of its own to wait for, so it runs as soon as its
initial delay is out and counts from there. A calendar schedule names a
wall-clock time — a time of day, or a weekday and a time of day — and waits for
it: restarting the service at three on a Wednesday afternoon is not "every
Monday at two". A schedule that says nothing about which it is is treated as a
calendar one, which is the safer of the two to be wrong about.

A calendar schedule reads the service's own zone rather than the host's, and
reads it again before each wait. A run that was missed because the service was
not running is not made up: the next occurrence is the next one, not the one
that has already gone.

Where the wall clock skips the hour a schedule names, the run rolls forward into
the hour that does exist. Being an hour of wall clock late costs less than being
skipped until the clocks go back. Where an hour happens twice, the run happens
in the first of them.

### Backoff

A task may hold itself back from its own schedule while it keeps faulting. The
wait doubles with each consecutive fault, from a base to a cap; a base without a
cap is refused where the task is registered, because uncapped doubling reaches
days within a morning, which is a task that has stopped rather than one waiting
longer.

The wait is a floor under the next attempt, never a ceiling. A schedule due after
the backoff has expired still waits for its own time.

What counts as a fault is a failed or blocked attempt. A success ends the
streak. Anything else — a refusal because something else held what the task
needed, a run that found nothing to do — is passed over: the task was busy, not
broken.

The streak is read from the recorded history rather than kept in memory, so a
restart neither forgets a backoff nor has to rebuild one, and it is counted per
argument: one target slot failing does not hold back another. A history that
cannot be read holds nothing back, because not running is the more expensive of
the two ways to be wrong.

Nothing is recorded about an attempt that was held back. The task is already in
its history as failing, and a row per suppressed tick would bury that under the
waiting.

A backoff never refuses an operator. Asking is a decision already made, and the
attempt they ask for is also the way out: a success ends the streak.

## The HTTP surface

`GET /v1/tasks` lists every background activity this build registers, in
registration order, with what is known about each right now: whether it runs
unasked, how many attempts are in flight, and when the first scheduled run is
due. A task nothing schedules reports no due time, which reads as absent rather
than as the zero instant.

`POST /v1/tasks/{name}/run`, and `POST /v1/tasks/{name}/run/{argument}` for one
over an argument, start a single attempt on exactly the terms a schedule starts
one. `202` means the attempt was accepted and continues independently of the
request. `409` means it was refused, which is not a fault: either this exact
work is already happening, or something it needs is held by another run.

A name this build does not register is refused as not found, so a page built
against a different build asks for nothing that silently does nothing.

## The service timezone

One zone for the whole service, not one per reader. A run happens once, and it
has to happen at a time somebody chose; a forecast hour has to describe where
the rider reads it. It is an IANA zone name, defaulting to `Europe/Berlin`.

A zone this binary cannot load is refused where it is entered and at startup,
because a calendar schedule reading it would have no answer to when it is next
due. The zone database travels inside the binary rather than depending on what
the runtime image carries.

## Shutdown

An attempt is bounded by shutdown, and by whatever bounds the work it does.
Shutdown cancels every scheduled task, waits for each to stop, and then waits for
whatever a manual trigger started. An attempt that shutdown ended reports
`cancelled` whatever it had managed to return, because it did not finish, and
that is never a fault.

Once shutdown has begun nothing new starts. A trigger is refused, and a schedule
whose wait ended at the same instant as the cancellation does not run: waiting
watches the clock and the context together and may report either, so the answer
is checked rather than inferred.

## The registered tasks

| Task | Argument | Resources | Schedule |
| --- | --- | --- | --- |
| `sync:source` | library, or none for every one | `inventory` exclusive | every hour |
| `sync:target` | target slot, or none for every one | `inventory` exclusive | every six hours |
| `sync:clear` | target slot | `inventory` exclusive | none |
| `surface:annotate` | none | `inventory` exclusive | none |
| `surface:index` | none | `surface-index` exclusive | the configured rebuild interval |

The read takes a library the same way the targets take a slot: none is every
configured one, a name is that one alone. One task rather than one per library,
for the reason the targets are one task — the configured set is a runtime
setting and tasks are registered at startup, so a library added through the
settings page would otherwise wait for a restart.

`sync:target` follows the read. `surface:annotate` follows both the read and the
index rebuild, and runs after each: either alone leaves stages wanting it.
`sync:target`'s own schedule is a backstop behind its edge — what it catches is a
slot that failed on its own, and an operator who has the read switched off.

`sync:target` takes a slot name or nothing. Nothing is every configured slot,
which is what both the schedule and a source read ask for; a name is that slot
alone. It is one task rather than one per slot because slots are a runtime
setting and tasks are registered at startup — per-slot tasks would leave a newly
onboarded slot doing nothing until a restart.

## What follows what

A task declares the tasks it follows, and that declaration is the whole graph.
An edge naming a task this build does not register, or one that closes a cycle,
is refused when the graph is resolved rather than found by a depth cap at four
in the morning. The cap and the set of what one chain has run stay behind that,
as belt and braces.

What follows an attempt follows a successful one. A read that failed stored
nothing to write or classify, and a rebuild that found nothing new left every
stored classification standing.

Each edge fires on its own, so a task following two predecessors runs after
each. That is what classification wants: a read leaves stages nobody has
classified, and a rebuild leaves the stored classifications stale, and neither
is waiting on the other.

An edge carries no argument. What a successor is over is its own business:
`sync:target` reconciles every configured slot when nothing names one.

## Switching a task off

Each task carries its own switch, read at each tick. A task nobody has ruled on
runs, so one added to a build reaches its schedule without anybody turning it on.

Switching one off pauses its schedule rather than ending it: the loop waits out
its tick and nothing is recorded, so switching it back on needs no restart. An
operator who turned it off is not waiting to be told it did not run.

The switch governs unattended runs only. An operator asking for a task has
already decided, which is also how they run something they keep switched off.

## Alerts

A task declares what an alert about it is titled and how long one silences the
next, or declares nothing and has nothing announced about it. Declaring one
without the other is refused where the task is registered: a window left at zero
would either silence the task or repeat itself every tick, and neither is what
leaving it out meant.

A fault is announced: a run that failed, or one a safety gate blocked. So is a
success, as one of two alerts — the routine kind, and the kind that follows
anything else and so ends an incident. They are separate because an operator who
silences every routine pass still wants to hear that the thing came back.

An attempt that did no work because something else held what it needed is
announced as nothing at all: this service was busy, which is not the same as
broken. Nor is a run that found its work already done.

A task may also be announced for how long it has gone without succeeding. One
that stopped succeeding raises no new fault once its first one is suppressed, so
without this it goes quiet exactly when it matters. The bound is the task's own,
read when the question is asked, and a task that names none is not expected on a
clock. Nothing is stale before it has ever been fresh: a task nobody has run yet
is waiting, not overdue.

A success is what freshness is, so it ends the staleness incident rather than
waiting out the window an earlier alert opened.

An operator rules on each alert separately, and a task declares which alerts it
can raise so that ruling on one is possible before being woken by it. An alert
nobody has ruled on is announced: a fault nobody has heard of is the one worth
hearing about. An alert switched off is not sent and opens no window, so
switching it back on does not find one it never heard the alert behind.

The suppression window is keyed by the reason as well as the task. A library
that cannot be read and a target that needs reauthorising are separate problems,
and one must not silence the other. A failing task is worth one message; the
same message every tick afterwards is noise an operator learns to ignore, which
is how the message that mattered gets missed.

Nothing is written down as sent until it has been, so a channel that was down
does not silence the alert it failed to carry. While the channel is switched off
nothing is sent and nothing is recorded as sent: turning it back on must not
find a suppression window it never heard the alert behind.

Every message names its run. The reference is random and means nothing on its
own, which is what makes it safe to send.

## The alert matrix

What an operator is offered a decision about is what the registered tasks
declare, read once at start because it is a fact about the build rather than
about a run. An alert nothing raises cannot be decided: storing one would leave
a row nobody reads behind a switch that appears to do something.

A decision about an alert nobody has ruled on is what creates its record, so an
alert left out of an edit keeps whatever it had rather than being switched off
by omission.

A decision reaches the running service, not only the database. One that had not
been read back would keep sending the alert an operator had just switched off,
which is the failure the switch exists to prevent. The decisions are read at
start too, and a service that cannot read them refuses to start rather than
announcing what was deliberately silenced.

## Enrichment failures

Classifying the ground under a stage and timing it are passes over the whole
stored inventory, not tasks of their own. A stage either pass could not finish
is named, with a stable reason for what stopped it, and the record is replaced
when the pass tries again and removed when it succeeds. What is there is what is
wrong now, rather than a log of everything that ever went wrong.

Storing what a pass produced is what clears its failure, in the same
transaction, so a stage cannot be enriched and listed as failing at once.

A failure goes when the stage it names leaves the library, and when the pass it
names stops being configured at all: a stage cannot be failing something nothing
is asking for. A pass whose inputs merely changed keeps its failures until the
next attempt replaces or clears them.

A pass a shutdown ended records nothing about the stage it was on. Whatever
reached it was the cancellation rather than anything about that stage, and a row
blaming the map for a service that was stopping would outlast the shutdown.

## Out of scope

Delayed retry after repeated failure and a bound on how long one attempt may
take are not part of this layer yet. Nothing here queues work or retries an
attempt.
