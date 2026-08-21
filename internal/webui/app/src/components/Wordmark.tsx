/**
 * The mark, the name, and the way to synchronisation.
 *
 * It replaces the header: the entry page is a map, and a bar across the top of a
 * map is a bar across the map. So the one thing that has to be said everywhere —
 * what this application is — is said by a small panel floating over the corner
 * of the cartography, with one quiet link at the far end of the row for the page
 * where synchronisation can be read and operated.
 *
 * One row, never two. Everything a run has to say about itself is on the sync
 * page; what is left here is whether that page wants the operator, which the
 * link carries as its own colour rather than as a line of prose under the name.
 */

import { useQuery } from "@tanstack/react-query";
import { Link } from "react-router";
import { statusQuery } from "../api/queries";
import type { Status, SyncPhase } from "../api/types";
import { SYNC_PHASES } from "../api/types";
import { formatTimestamp } from "../lib/format";
import { syncGuidance } from "../lib/syncGuidance";
import { Logo } from "./Logo";

/** How urgently the link is painted: quiet unless it is one of the three states. */
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
 * The state of synchronisation, derived from the service's own.
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
 * The state is carried by the link rather than written out beside the name: the
 * map behind this panel is what the reader came for, and a paragraph about
 * synchronisation over the corner of it is a second thing to read before the
 * first one has been looked at. What the tone cannot say in colour, the link
 * carries as its own name, so `Sync, An account is not connected` is what a
 * screen reader and a hovering pointer both get from a mark with no words in
 * it.
 *
 * A state that has not arrived — or never arrives — leaves the link plain. The
 * request failing in the corner of a map is not the reader's problem until they
 * ask about synchronisation.
 */
export function Wordmark() {
  const { data } = useQuery(statusQuery());
  const state = data ? wordmarkState(data) : null;
  const described = state ? `Sync · ${state.label}` : undefined;

  return (
    <div className="panel wordmark">
      <Logo className="wordmark__logo" size={22} />
      <span className="wordmark__name">domestique</span>
      <Link
        className="wordmark__sync"
        to="/sync"
        data-tone={state?.tone}
        /*
         * The mark carries no words of its own, so the name is always spelled
         * out: `Sync` alone while there is no state to report, and the state
         * appended once there is. The tooltip stays with the state — a pointer
         * hovering a link to the sync page learns nothing from being told it
         * goes to the sync page.
         */
        aria-label={described ?? "Sync"}
        title={described}
      >
        {/*
         * Two sliders, taking the colour of the state around them. The word it
         * replaces said the same thing twice — the panel's other two marks are
         * already the name — and a glyph leaves the tone to be the whole of what
         * the corner is saying. The knobs are punched out in the panel's own
         * ground so the mark reads at 16 px rather than filling in.
         */}
        <svg
          viewBox="0 0 16 16"
          width="16"
          height="16"
          fill="none"
          stroke="currentColor"
          strokeWidth="1.6"
          strokeLinecap="round"
          aria-hidden="true"
          focusable="false"
        >
          <line x1="2" y1="5.2" x2="14" y2="5.2" />
          <line x1="2" y1="10.8" x2="14" y2="10.8" />
          <circle cx="6" cy="5.2" r="1.9" fill="var(--panel)" />
          <circle cx="10.6" cy="10.8" r="1.9" fill="var(--panel)" />
        </svg>
      </Link>
    </div>
  );
}
