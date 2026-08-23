/**
 * What is happening now, and the controls over each half.
 *
 * A synchronisation has two halves — reading the VeloPlanner library, and
 * writing what was read onto the Wahoo accounts — and each has its own switch
 * and its own button. They are one row per half rather than a settings list,
 * because the question an operator actually has is about a half: is it on, when
 * did it last run, and can I run it now.
 *
 * A switch governs the timer only. The button beside it runs that half whether
 * the switch is on or off, which is why the button never disables itself for a
 * switched-off half: turning the schedule off is a statement about unattended
 * runs, not a lock.
 */

import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { retrySurfaceEnrichment, setSyncSchedule, triggerSync } from "../../api/client";
import { statusQuery, webUIConfigQuery } from "../../api/queries";
import type { Status, SyncActive, SyncPhase, SyncPhaseRun, SyncSchedule } from "../../api/types";
import { SYNC_PHASES } from "../../api/types";
import { Button } from "../../components/Button";
import { formatTimestamp } from "../../lib/format";
import { syncGuidance } from "../../lib/syncGuidance";
import { phaseLabels, runningPhaseLabels } from "../../lib/syncLabels";

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
 * much of what the accounts are owed they already hold. A run cannot say which
 * route it is on without naming one, so it does not try, and an empty library
 * is left to the headline alone rather than told it is nought of nought.
 */
export function activeSummary(
  state: string,
  active: SyncActive,
  runningLabels: Record<SyncPhase, string> = runningPhaseLabels([]),
): string {
  const headline = activeHeadline(state, active, runningLabels);
  const total = active.stages.current + active.stages.pending;
  if (total === 0) {
    return headline;
  }
  const accounts = active.targets === 1 ? "account" : "accounts";

  return `${headline} · ${active.stages.current} of ${total} routes across ${active.targets} ${accounts}`;
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

/**
 * What one half's last run amounts to, in a line.
 *
 * A run that did not succeed reduces to how it ended and when. What it means
 * and what to do about it is the guidance line beneath it, because "held" and
 * "failed" ask opposite things of an operator and neither fits in a count.
 */
export function runSummary(phase: SyncPhase, run: SyncPhaseRun | undefined): string {
  if (!run) {
    return "Has not run yet";
  }
  const when = formatTimestamp(run.lastCompletedAt);
  const guidance = syncGuidance(phase, run.lastResult, run.lastFailure);
  if (guidance) {
    return `${when} · ${guidance.kind === "blocked" ? "held by a gate" : "did not finish"}`;
  }
  const counts =
    phase === "source"
      ? `${run.sourceStages} routes`
      : [
          `${run.created} created`,
          `${run.updated} updated`,
          ...(run.deleted > 0 ? [`${run.deleted} deleted`] : []),
        ].join(", ");

  return `${when} · ${counts}`;
}

/** The body of the "Now" card: one line, then one row per half. */
export function SyncControls() {
  const queryClient = useQueryClient();
  const { data, isPending, isError } = useQuery(statusQuery());
  const config = useQuery(webUIConfigQuery());
  // The sources this build can name, which is every provider a base URL is
  // configured for. Unresolved while the config is still loading, which reads
  // as the generic phrase below rather than blocking on a second query.
  const sourceProviders = Object.keys(config.data?.sourceBaseUrls ?? {});
  const labels = phaseLabels(sourceProviders);
  const runningLabels = runningPhaseLabels(sourceProviders);

  const invalidateStatus = () =>
    queryClient.invalidateQueries({ queryKey: statusQuery().queryKey });
  const schedule = useMutation({
    mutationFn: (next: SyncSchedule) => setSyncSchedule(next),
    onSuccess: invalidateStatus,
  });
  const run = useMutation({
    mutationFn: (phase: SyncPhase) => triggerSync(phase),
    onSuccess: invalidateStatus,
  });
  const retryClassification = useMutation({
    mutationFn: retrySurfaceEnrichment,
    onSuccess: invalidateStatus,
  });

  if (isPending) {
    return null;
  }
  if (isError) {
    return <p className="sync-card__error">The service did not say what it is doing.</p>;
  }

  // Both switches travel on every change: the service refuses a half-named
  // schedule, and sending the pair means the operator's own view of the other
  // half is what gets written rather than a value assumed here.
  const toggle = (phase: SyncPhase) =>
    schedule.mutate({ ...data.sync.schedule, [phase]: !data.sync.schedule[phase] });

  return (
    <>
      {/*
       * The one line here that changes while it is being read. It is announced
       * politely rather than assertively: the operator asked for this run, so
       * its progress is something to look at when they choose to, and the text
       * only changes when the run does.
       */}
      <p className="sync-card__line" aria-live="polite">
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
        <p className="sync-card__line">
          Surface classified for {data.sync.surface.classified} of {data.sync.surface.total}{" "}
          {data.sync.surface.total === 1 ? "route" : "routes"}. Each unclassified route is tried
          again after every read.
          {data.sync.surface.incomplete > 0 ? (
            <>
              {" "}
              {data.sync.surface.incomplete} could not be classified last time.{" "}
              <Button
                variant="standard"
                disabled={retryClassification.isPending}
                onClick={() => retryClassification.mutate()}
              >
                {retryClassification.isPending ? "Requesting…" : "Retry now"}
              </Button>
            </>
          ) : null}
        </p>
      ) : null}
      {retryClassification.isError ? (
        <p className="sync-card__error" role="alert">
          {retryClassification.error instanceof Error && retryClassification.error.message
            ? retryClassification.error.message
            : "That retry could not be started."}
        </p>
      ) : null}
      <ul className="sync-card__list">
        {SYNC_PHASES.map((phase) => {
          const enabled = data.sync.schedule[phase];
          const phaseRun = data.sync.phases[phase];
          const guidance = phaseRun
            ? syncGuidance(phase, phaseRun.lastResult, phaseRun.lastFailure)
            : undefined;

          return (
            <li className="sync-row" key={phase}>
              <div className="sync-row__text">
                <span className="sync-row__title">{labels[phase]}</span>
                <span className="sync-row__detail">{runSummary(phase, phaseRun)}</span>
                {/*
                 * A gate that held is not an error the operator caused, so it is
                 * stated rather than announced: the page is being read, not
                 * interrupted, and the run it describes finished some time ago.
                 */}
                {guidance ? (
                  <span className="sync-guidance" data-kind={guidance.kind}>
                    <strong>{guidance.headline}</strong> {guidance.remediation}
                  </span>
                ) : null}
              </div>
              <div className="sync-row__actions">
                {/*
                 * Both rows carry the same two words, so the visible text alone
                 * names neither half. The accessible name says which one, since
                 * a reader arriving at the second checkbox has no row above it
                 * to tell them apart. The interval is the service's own and is
                 * fixed at an hour, so the switch can say what it schedules.
                 */}
                <label className="sync-row__switch">
                  <input
                    type="checkbox"
                    checked={enabled}
                    disabled={schedule.isPending}
                    onChange={() => toggle(phase)}
                    aria-label={`Hourly: ${labels[phase]}`}
                  />
                  <span>Hourly</span>
                </label>
                <Button
                  disabled={run.isPending}
                  onClick={() => run.mutate(phase)}
                  aria-label={`Run now: ${labels[phase]}`}
                >
                  Run now
                </Button>
              </div>
            </li>
          );
        })}
      </ul>
      {/*
       * Announced rather than waited for: the operator has just pressed
       * something and nothing happened, which is the case a polite live region
       * is least suited to.
       */}
      {schedule.isError ? (
        <p className="sync-card__error" role="alert">
          The schedule was not changed. It is still what it was.
        </p>
      ) : null}
      {run.isError ? (
        <p className="sync-card__error" role="alert">
          {run.error instanceof Error && run.error.message
            ? run.error.message
            : "That run could not be started."}
        </p>
      ) : null}
    </>
  );
}
