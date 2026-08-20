/**
 * The operator's controls over synchronisation.
 *
 * A synchronisation has two halves — reading the VeloPlanner library, and
 * writing what was read onto the Wahoo accounts — and each has its own switch
 * and its own button. They are presented as one row per half rather than as a
 * settings list, because the question an operator actually has is about a half:
 * is it on, when did it last run, and can I run it now.
 *
 * A switch governs the timer only. The button beside it runs that half whether
 * the switch is on or off, which is why the button never disables itself for a
 * switched-off half: turning the schedule off is a statement about unattended
 * runs, not a lock.
 */

import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { setSyncSchedule, triggerSync } from "../../api/client";
import { statusQuery } from "../../api/queries";
import type { SyncActive, SyncPhase, SyncPhaseRun, SyncSchedule } from "../../api/types";
import { SYNC_PHASES } from "../../api/types";
import { Button } from "../../components/Button";
import { ErrorMessage } from "../../components/StatusMessage";
import { formatTimestamp } from "../../lib/format";
import { GUIDANCE_LABELS, syncGuidance } from "../../lib/syncGuidance";

const PHASE_LABELS: Record<SyncPhase, { title: string; detail: string }> = {
  source: {
    title: "Read from VeloPlanner",
    detail: "Refreshes the stored library and the map. Touches no device.",
  },
  targets: {
    title: "Write to Wahoo",
    detail: "Reconciles the stored library onto every target account.",
  },
};

/** What each half is doing while it is doing it, rather than what to press. */
const RUNNING_LABELS: Record<SyncPhase, string> = {
  source: "Reading from VeloPlanner",
  targets: "Writing to Wahoo",
};

function activeHeadline(state: string, active: SyncActive): string {
  if (state === "delayed") {
    return active.startsAt
      ? `First run at ${formatTimestamp(active.startsAt)}`
      : "Waiting to start";
  }
  if (active.phase) {
    return RUNNING_LABELS[active.phase];
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
 * stage it is on without naming one, so it does not try, and an empty library
 * is left to the headline alone rather than told it is nought of nought.
 */
export function activeSummary(state: string, active: SyncActive): string {
  const headline = activeHeadline(state, active);
  const total = active.stages.current + active.stages.pending;
  if (total === 0) {
    return headline;
  }
  const accounts = active.targets === 1 ? "account" : "accounts";

  return `${headline} · ${active.stages.current} of ${total} stages across ${active.targets} ${accounts}`;
}

/**
 * What one half's last run amounts to, in a sentence.
 *
 * A run that did not succeed reduces to how it ended and when. What it means and
 * what to do about it is the guidance line beside this one, because "blocked"
 * and "failed" ask opposite things of an operator and neither fits in a count.
 */
export function runSummary(phase: SyncPhase, run: SyncPhaseRun | undefined): string {
  if (!run) {
    return "Has not run yet.";
  }
  const when = formatTimestamp(run.lastCompletedAt);
  const guidance = syncGuidance(phase, run.lastResult, run.lastFailure);
  if (guidance) {
    return `${GUIDANCE_LABELS[guidance.kind]} · ${when}`;
  }
  const counts =
    phase === "source"
      ? `${run.sourceStages} stages`
      : `${run.created} created · ${run.updated} updated · ${run.deleted} deleted`;

  return `${counts} · ${when}`;
}

export function SyncControls() {
  const queryClient = useQueryClient();
  const { data, isPending, isError, error } = useQuery(statusQuery());

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

  if (isPending) {
    return null;
  }
  if (isError) {
    return <ErrorMessage what="the synchronisation controls" error={error} />;
  }

  // Both switches travel on every change: the service refuses a half-named
  // schedule, and sending the pair means the operator's own view of the other
  // half is what gets written rather than a value assumed here.
  const toggle = (phase: SyncPhase) =>
    schedule.mutate({ ...data.sync.schedule, [phase]: !data.sync.schedule[phase] });

  return (
    <section className="sync-controls" aria-labelledby="sync-controls-heading">
      <h2 className="sync-controls__heading" id="sync-controls-heading">
        Synchronisation
      </h2>
      {/*
       * The one line here that changes while it is being read. It is announced
       * politely rather than assertively: the operator asked for this run, so
       * its progress is something to look at when they choose to, and the text
       * only changes when the run does.
       */}
      {data.sync.active ? (
        <p className="sync-controls__active" aria-live="polite">
          {activeSummary(data.sync.state, data.sync.active)}
        </p>
      ) : null}
      {/*
       * Classification is enrichment: it never fails a run, so a stage the
       * endpoint keeps refusing is otherwise indistinguishable from one that has
       * not come up yet. The count is the only place that difference shows.
       */}
      {data.sync.surface.total > 0 && data.sync.surface.classified < data.sync.surface.total ? (
        <p className="sync-controls__coverage">
          Surface classified for {data.sync.surface.classified} of {data.sync.surface.total}{" "}
          {data.sync.surface.total === 1 ? "stage" : "stages"}. Each unclassified stage is tried
          again after every read.
        </p>
      ) : null}
      <ul className="sync-controls__phases">
        {SYNC_PHASES.map((phase) => {
          const enabled = data.sync.schedule[phase];
          const phaseRun = data.sync.phases[phase];
          const guidance = phaseRun
            ? syncGuidance(phase, phaseRun.lastResult, phaseRun.lastFailure)
            : undefined;

          return (
            <li className="sync-controls__phase" key={phase}>
              <div className="sync-controls__text">
                <span className="sync-controls__title">{PHASE_LABELS[phase].title}</span>
                <span className="sync-controls__detail">{PHASE_LABELS[phase].detail}</span>
                <span className="sync-controls__run">{runSummary(phase, phaseRun)}</span>
                {/*
                 * A gate that held is not an error the operator caused, so it is
                 * stated rather than announced: the page is being read, not
                 * interrupted, and the run it describes finished some time ago.
                 */}
                {guidance ? (
                  <span className="sync-controls__guidance" data-kind={guidance.kind}>
                    <span className="sync-controls__guidance-headline">{guidance.headline}</span>{" "}
                    {guidance.remediation}
                  </span>
                ) : null}
              </div>
              <div className="sync-controls__actions">
                {/*
                 * Both rows carry the same two words, so the visible text alone
                 * names neither half. The accessible name says which one, since
                 * a reader arriving at the second checkbox has no row above it
                 * to tell them apart.
                 */}
                <label className="sync-controls__switch">
                  <input
                    type="checkbox"
                    checked={enabled}
                    disabled={schedule.isPending}
                    onChange={() => toggle(phase)}
                    aria-label={`Schedule: ${PHASE_LABELS[phase].title}`}
                  />
                  <span>{enabled ? "Scheduled" : "Paused"}</span>
                </label>
                <Button
                  disabled={run.isPending}
                  onClick={() => run.mutate(phase)}
                  aria-label={`Run now: ${PHASE_LABELS[phase].title}`}
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
        <p className="sync-controls__error" role="alert">
          The schedule was not changed. It is still what it was.
        </p>
      ) : null}
      {run.isError ? (
        <p className="sync-controls__error" role="alert">
          {run.error instanceof Error && run.error.message
            ? run.error.message
            : "That run could not be started."}
        </p>
      ) : null}
    </section>
  );
}
