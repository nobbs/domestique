/**
 * The state of a sync, in as few words as a line allows.
 *
 * It lives beside `syncGuidance` rather than with the component that shows it,
 * because it is a reading of the service's status and not a property of any one
 * place that reading is printed. The menu bar prints it; the sync page draws the
 * same conclusion at length from the same status.
 */

import type { Status, SyncPhase } from "../api/types";
import { SYNC_PHASES } from "../api/types";
import { formatTimestamp } from "./format";
import { syncGuidance } from "./syncGuidance";

/** How urgently the state is painted: quiet unless it is one of the three. */
export type StateTone = "good" | "hold" | "alert" | undefined;

export interface SyncState {
  label: string;
  tone: StateTone;
}

/** What a half is doing while it is doing it. */
const RUNNING_LABELS: Record<SyncPhase, string> = {
  source: "Reading the library",
  targets: "Writing to Wahoo",
};

/**
 * The order is the order an operator would want it in: what is happening now
 * outranks what happened last, a run that needs them outranks a run that does
 * not, and a target that cannot be written to at all outranks how far behind
 * the targets are. Only the last of those is a state worth painting green.
 */
export function syncState(status: Status): SyncState {
  const { sync } = status;
  if (sync.active) {
    const label = sync.active.phase ? RUNNING_LABELS[sync.active.phase] : "Starting";

    return { label: sync.state === "delayed" ? "Waiting to start" : label, tone: undefined };
  }

  for (const phase of SYNC_PHASES) {
    const run = sync.phases[phase];
    const guidance = run ? syncGuidance(phase, run.lastResult, run.lastFailure) : undefined;
    if (guidance?.kind === "blocked") {
      return { label: `Held by a gate · ${formatTimestamp(run?.lastCompletedAt)}`, tone: "hold" };
    }
    // Only a run that went wrong. `syncGuidance` also speaks for a run that never
    // started — a half skipped because the other was going, or one held back until
    // a target is connected — and neither is a fault: the second is the unconnected
    // target, which the check below names for what it is.
    if (guidance?.kind === "failed") {
      return { label: `Did not finish · ${formatTimestamp(run?.lastCompletedAt)}`, tone: "alert" };
    }
  }

  if (status.targets.some((target) => target.convergence === "unauthorized")) {
    return { label: "A target is not connected", tone: "alert" };
  }
  if (!sync.lastCompletedAt) {
    return { label: "Has not run yet", tone: undefined };
  }
  const when = formatTimestamp(sync.lastCompletedAt);

  return status.converged
    ? { label: `In sync · ${when}`, tone: "good" }
    : { label: `Behind · ${when}`, tone: undefined };
}
