/**
 * What is happening now, and the controls over each half.
 *
 * A sync has two halves — reading the VeloPlanner library, and
 * writing what was read onto the Wahoo targets — and each has its own switch
 * and its own button. They are one row per half rather than a settings list,
 * because the question an operator actually has is about a half: is it on, when
 * did it last run, and can I run it now.
 *
 * A switch governs the timer only. The button beside it runs that half whether
 * the switch is on or off, which is why the button never disables itself for a
 * switched-off half: turning the schedule off is a statement about unattended
 * runs, not a lock.
 */

import { useQuery, useQueryClient } from "@tanstack/react-query";
import { useRunTask, useSetTaskSchedule } from "../../api/generated";
import { statusQuery, tasksQuery, webUIConfigQuery } from "../../api/queries";
import { SYNC_PHASE_TASKS, TASKS } from "../../api/tasks";
import type { Status, SyncActive, SyncPhase } from "../../api/types";
import { SYNC_PHASES } from "../../api/types";
import { Button } from "../../components/Button";
import { Skeleton } from "../../components/ui/skeleton";
import { Spinner } from "../../components/ui/spinner";
import { formatCadence, formatTimestamp } from "../../lib/format";
import { syncGuidance } from "../../lib/syncGuidance";
import { phaseLabels, runningPhaseLabels } from "../../lib/syncLabels";
import { SyncPhaseRow } from "./SyncPhaseRow";

function activeHeadline(
  state: string,
  active: SyncActive,
  runningLabels: Record<SyncPhase, string>,
): string {
  if (state === "delayed") {
    return active.startsAt
      ? `First run at ${formatTimestamp(active.startsAt)}`
      : "Waiting to start";
  }
  if (active.phase) {
    return runningLabels[active.phase];
  }

  // Accepted, and not yet in either half. It is a moment long, and it is the
  // moment an operator who has just pressed the button is looking at.
  return "Starting";
}

/**
 * What is happening right now, in a sentence.
 *
 * The counts are the aggregate the service reports and nothing beyond it: how
 * much of what the targets are owed they already hold. A run cannot say which
 * route it is on without naming one, so it does not try, and an empty library
 * is left to the headline alone rather than told it is nought of nought.
 */
export function activeSummary(
  state: string,
  active: SyncActive,
  runningLabels: Record<SyncPhase, string> = runningPhaseLabels([]),
): string {
  const headline = activeHeadline(state, active, runningLabels);
  const total = active.routes.current + active.routes.pending;
  if (total === 0) {
    return headline;
  }
  const targets = active.targets === 1 ? "target" : "targets";

  return `${headline} · ${active.routes.current} of ${total} routes across ${active.targets} ${targets}`;
}

/**
 * What the page says when nothing is running.
 *
 * Two clauses: that nothing is running, and when each half last got somewhere.
 * A half that was held says so instead of saying when it finished, because a
 * gate that held is the one thing on this card an operator has to act on.
 */
export function idleSummary(status: Status): string {
  const held = SYNC_PHASES.map((phase) => {
    const run = status.sync.phases[phase];
    const guidance = run ? syncGuidance(phase, run.lastResult, run.lastFailure) : undefined;

    return guidance?.kind === "blocked" ? { phase, run } : null;
  }).find(Boolean);
  if (held?.run) {
    const half = held.phase === "source" ? "read" : "write";

    return `Nothing is running. The last ${half} was held at ${formatTimestamp(held.run.lastCompletedAt)}.`;
  }

  const read = status.sync.phases.source?.lastCompletedAt;
  const write = status.sync.phases.targets?.lastCompletedAt;
  if (!read && !write) {
    return "Nothing is running, and nothing has run yet.";
  }

  return `Nothing is running. Last read ${formatTimestamp(read)}, last write ${formatTimestamp(write)}.`;
}

/** The body of the "Now" card: one line, then one row per half. */
export function SyncControls() {
  const queryClient = useQueryClient();
  const { data, isPending, isError } = useQuery(statusQuery());
  const tasks = useQuery(tasksQuery());
  const config = useQuery(webUIConfigQuery());
  // The sources this build can name, which is every provider a base URL is
  // configured for. Unresolved while the config is still loading, which reads
  // as the generic phrase below rather than blocking on a second query.
  const sourceProviders = Object.keys(config.data?.sourceBaseUrls ?? {});
  const labels = phaseLabels(sourceProviders);
  const runningLabels = runningPhaseLabels(sourceProviders);

  const invalidateStatus = () =>
    Promise.all([
      queryClient.invalidateQueries({ queryKey: statusQuery().queryKey }),
      queryClient.invalidateQueries({ queryKey: tasksQuery().queryKey }),
    ]);
  const schedule = useSetTaskSchedule({ mutation: { onSuccess: invalidateStatus } });
  const sourceRun = useRunTask({ mutation: { onSuccess: invalidateStatus } });
  const targetsRun = useRunTask({ mutation: { onSuccess: invalidateStatus } });
  // Each phase now has its own mutation, so each keeps its own terminal state
  // and a failure would otherwise outlive the successful run of the other half.
  // The banner belongs to whichever phase was asked for last.
  const lastRun = targetsRun.submittedAt >= sourceRun.submittedAt ? targetsRun : sourceRun;
  const retryClassification = useRunTask({
    mutation: { onSuccess: invalidateStatus },
  });

  // The task list is required, not decorative: a switch drawn before it arrives
  // would show a state nobody reported, and pressing it would send the opposite
  // of a guess.
  if (isPending || tasks.isPending) {
    return <Skeleton className="h-28 w-full" role="status" aria-label="Loading sync controls" />;
  }
  if (isError || tasks.isError) {
    return <p className="text-sm text-[var(--alert)]">The service did not say what it is doing.</p>;
  }

  // Each half is its own task now, so a switch travels alone: nothing here has
  // to carry a value for the other half, and cannot get it wrong. A task the
  // service does not list is one nobody has ruled on, which runs.
  const enabledFor = (phase: SyncPhase) =>
    tasks.data?.tasks?.find((task) => task.name === SYNC_PHASE_TASKS[phase])?.enabled ?? true;
  const cadenceFor = (phase: SyncPhase) =>
    formatCadence(
      tasks.data?.tasks?.find((task) => task.name === SYNC_PHASE_TASKS[phase])?.intervalSeconds,
    );
  const toggle = (phase: SyncPhase) =>
    schedule.mutate({ name: SYNC_PHASE_TASKS[phase], data: { enabled: !enabledFor(phase) } });

  return (
    <>
      {/*
       * The one line here that changes while it is being read. It is announced
       * politely rather than assertively: the operator asked for this run, so
       * its progress is something to look at when they choose to, and the text
       * only changes when the run does.
       */}
      <p className="text-sm text-[var(--ink-2)]" aria-live="polite">
        {data.sync.active
          ? activeSummary(data.sync.state, data.sync.active, runningLabels)
          : idleSummary(data)}
      </p>
      {/*
       * Classification is enrichment: it never fails a run, so a route the
       * endpoint keeps refusing is otherwise indistinguishable from one that has
       * not come up yet. incomplete is the count that draws that difference,
       * and the retry beside it touches only the local surface index and
       * cache — it never reads VeloPlanner or writes a Wahoo target, which is
       * why it is offered on its own rather than folded into "Run now".
       */}
      {data.sync.surface.total > 0 && data.sync.surface.classified < data.sync.surface.total ? (
        <p className="text-sm text-[var(--ink-2)]">
          Surface classified for {data.sync.surface.classified} of {data.sync.surface.total}{" "}
          {data.sync.surface.total === 1 ? "route" : "routes"}. Each unclassified route is tried
          again after every read.
          {data.sync.surface.incomplete > 0 ? (
            <>
              {" "}
              {data.sync.surface.incomplete} could not be classified last time.{" "}
              <Button
                variant="outline"
                disabled={retryClassification.isPending}
                onClick={() => retryClassification.mutate({ name: TASKS.surfaceAnnotate })}
              >
                {retryClassification.isPending ? <Spinner aria-label="Requesting retry" /> : null}
                {retryClassification.isPending ? "Requesting…" : "Retry now"}
              </Button>
            </>
          ) : null}
        </p>
      ) : null}
      {/*
       * enrichmentFailures is the surface and duration passes together, and
       * durable rather than reset by the next pass: a stage classification
       * keeps retrying can still be timed successfully, and vice versa, so
       * this is the count that would otherwise be write-only.
       */}
      {data.sync.surface.enrichmentFailures > 0 ? (
        <p className="text-sm text-[var(--ink-2)]">
          {data.sync.surface.enrichmentFailures}{" "}
          {data.sync.surface.enrichmentFailures === 1 ? "stage has" : "stages have"} an unfinished
          enrichment pass.
        </p>
      ) : null}
      {retryClassification.isError ? (
        <p className="text-sm text-[var(--alert)]" role="alert">
          {retryClassification.error instanceof Error && retryClassification.error.message
            ? retryClassification.error.message
            : "That retry could not be started."}
        </p>
      ) : null}
      <ul className="grid gap-3">
        {SYNC_PHASES.map((phase) => {
          const run = phase === "source" ? sourceRun : targetsRun;

          return (
            <SyncPhaseRow
              key={phase}
              phase={phase}
              label={labels[phase]}
              lastRun={data.sync.phases[phase]}
              enabled={enabledFor(phase)}
              cadence={cadenceFor(phase)}
              scheduleDisabled={schedule.isPending}
              onToggle={() => toggle(phase)}
              running={run.isPending}
              onRun={() => run.mutate({ name: SYNC_PHASE_TASKS[phase] })}
            />
          );
        })}
      </ul>
      {/*
       * Announced rather than waited for: the operator has just pressed
       * something and nothing happened, which is the case a polite live region
       * is least suited to.
       */}
      {schedule.isError ? (
        <p className="text-sm text-[var(--alert)]" role="alert">
          The schedule was not changed. It is still what it was.
        </p>
      ) : null}
      {lastRun.isError ? (
        <p className="text-sm text-[var(--alert)]" role="alert">
          {lastRun.error instanceof Error && lastRun.error.message
            ? lastRun.error.message
            : "That run could not be started."}
        </p>
      ) : null}
    </>
  );
}
