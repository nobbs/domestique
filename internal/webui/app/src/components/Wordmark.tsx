/**
 * The mark, the name, and one line about synchronisation.
 *
 * It replaces the header: the entry page is a map, and a bar across the top of a
 * map is a bar across the map. So the one thing that has to be said everywhere —
 * what this application is, and whether it is doing its job — is said by a small
 * panel floating over the corner of the cartography, and the whole panel is the
 * link to the page where synchronisation can actually be read and operated.
 *
 * One line, never two. Everything a run has to say about itself is on the sync
 * page; here there is room for whether it wants the operator, and when it last
 * finished.
 */

import { useQuery } from "@tanstack/react-query";
import { Link } from "react-router";
import { statusQuery } from "../api/queries";
import type { Status, SyncPhase } from "../api/types";
import { SYNC_PHASES } from "../api/types";
import { formatTimestamp } from "../lib/format";
import { syncGuidance } from "../lib/syncGuidance";
import { Logo } from "./Logo";

/** How urgently the line is painted: quiet unless it is one of the three states. */
export type StateTone = "good" | "hold" | "alert" | undefined;

export interface WordmarkState {
  label: string;
  tone: StateTone;
}

/** What a half is doing while it is doing it, in as few words as a line allows. */
const RUNNING_LABELS: Record<SyncPhase, string> = {
  source: "Reading the library",
  targets: "Writing to Wahoo",
};

/**
 * The one line, derived from the service's own state.
 *
 * The order is the order an operator would want it in: what is happening now
 * outranks what happened last, a run that needs them outranks a run that does
 * not, and an account that cannot be written to at all outranks how far behind
 * the accounts are. Only the last of those is a state worth painting green.
 */
export function wordmarkState(status: Status): WordmarkState {
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
    if (guidance) {
      return { label: `Did not finish · ${formatTimestamp(run?.lastCompletedAt)}`, tone: "alert" };
    }
  }

  if (status.targets.some((target) => target.convergence === "unauthorized")) {
    return { label: "An account is not connected", tone: "alert" };
  }
  if (!sync.lastCompletedAt) {
    return { label: "Has not run yet", tone: undefined };
  }
  const when = formatTimestamp(sync.lastCompletedAt);

  return status.converged
    ? { label: `In sync · ${when}`, tone: "good" }
    : { label: `Behind · ${when}`, tone: undefined };
}

/**
 * The wordmark panel.
 *
 * The state line is absent rather than guessed at while the status is still on
 * its way, and absent rather than alarming if it never arrives: the map behind
 * this panel is what the reader came for, and a request that failed in the
 * corner of it is not their problem until they ask about synchronisation.
 */
export function Wordmark() {
  const { data } = useQuery(statusQuery());
  const state = data ? wordmarkState(data) : null;

  return (
    <Link className="panel wordmark" to="/sync">
      <Logo size={26} />
      <span className="wordmark__text">
        <span className="wordmark__name">domestique</span>
        {state ? (
          <span className="wordmark__state" data-tone={state.tone}>
            {state.label}
          </span>
        ) : null}
      </span>
    </Link>
  );
}
