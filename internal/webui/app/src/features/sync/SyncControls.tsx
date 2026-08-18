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
import type { SyncPhase, SyncPhaseRun, SyncSchedule } from "../../api/types";
import { SYNC_PHASES } from "../../api/types";
import { ErrorMessage } from "../../components/StatusMessage";
import { formatTimestamp } from "../../lib/format";

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

/** What one half's last run amounts to, in a sentence. */
export function runSummary(phase: SyncPhase, run: SyncPhaseRun | undefined): string {
  if (!run) {
    return "Has not run yet.";
  }
  const when = formatTimestamp(run.lastCompletedAt);
  if (run.lastResult !== "succeeded") {
    return `${run.lastResult}${run.lastFailure ? ` (${run.lastFailure})` : ""} · ${when}`;
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

          return (
            <li className="sync-controls__phase" key={phase}>
              <div className="sync-controls__text">
                <span className="sync-controls__title">{PHASE_LABELS[phase].title}</span>
                <span className="sync-controls__detail">{PHASE_LABELS[phase].detail}</span>
                <span className="sync-controls__run">
                  {runSummary(phase, data.sync.phases[phase])}
                </span>
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
                <button
                  type="button"
                  className="sync-controls__button"
                  disabled={run.isPending}
                  onClick={() => run.mutate(phase)}
                  aria-label={`Run now: ${PHASE_LABELS[phase].title}`}
                >
                  Run now
                </button>
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
