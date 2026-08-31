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
current     what it covers was already up to date, so it did nothing at all
~~~

## Run history

Every attempt is recorded, with two exceptions. One that found its work already
`current` did nothing worth remembering, and recording every such attempt is
what a sweep over a whole library would otherwise write on every tick. One that
shutdown `cancelled` cannot write during the shutdown that ended it.

A refusal is recorded, and says which kind of busy stopped it: this service
working on the very same thing, or working on something else that held what the
attempt needed. They are the answer to why something did not run, and that is
only answerable afterwards if it was written down.

A chain link is the exception. One asking for work already under way is dropped
rather than refused, because the work is happening — which is what the link
wanted — and the rest of the chain counts it as run.

`unchanged` and `current` are deliberately separate. A rebuild that reached its
upstream and found the published data identical did work — it checked — and the
next delay counts from that check. A stage whose fingerprint already matched
reached nothing.

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
| `sync` | phase, or none for both | `inventory` exclusive | every hour |
| `sync:target` | target slot | `inventory` exclusive | none |
| `sync:clear` | target slot | `inventory` exclusive | none |
| `surface:annotate` | none | `inventory` exclusive | none |
| `surface:index` | none | `surface-index` exclusive | the configured rebuild interval |

A scheduled `sync` honours the schedule switches for each half. An operator
asking for one overrides them, because asking is the point.

## Out of scope

Notification policy, delayed retry after repeated failure, and a bound on how
long one attempt may take are not part of this layer yet. Nothing here queues
work or retries an attempt.
