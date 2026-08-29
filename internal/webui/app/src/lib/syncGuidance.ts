/**
 * What a run that did not succeed means, and what the operator should do next.
 *
 * The service reports a run as a result word and a stable failure category, and
 * both are deliberately terse: they are safe to log, safe to notify on, and say
 * nothing about routes, payloads, or upstream text. That safety is exactly what
 * makes them unhelpful on their own — "blocked (deletion_limit)" is the shape of
 * an answer without being one.
 *
 * The distinction this module exists to draw is blocked from failed. A blocked
 * run is the service working correctly: a deletion gate held, nothing was
 * removed, and the operator has a deliberate decision to make. A failed run is
 * something that went wrong and will usually clear on its own or on a retry.
 * Presenting the two the same way teaches an operator to retry a gate, which is
 * the one response that must stay a considered act.
 *
 * Every string here is a constant. Nothing from a run, a route, a target slot,
 * or an upstream response is interpolated into guidance, so this module cannot
 * become a way for any of that to reach the page.
 */

import type { SyncPhase } from "../api/types";

/**
 * The failure categories the service emits, in its own wire names.
 *
 * Mirrors `FailureCategory` in `internal/sync/service.go`. The empty category is
 * not listed: it means "no failure", which is the absence of a reason rather
 * than one of them.
 */
export const SYNC_FAILURE_CATEGORIES = [
  "state",
  "source",
  "authorization",
  "destination",
  "course",
  "empty_source",
  "deletion_limit",
] as const;

export type SyncFailureCategory = (typeof SYNC_FAILURE_CATEGORIES)[number];

/**
 * How a run that did not succeed should read.
 *
 * `blocked` is a safety gate holding on purpose and nothing was written;
 * `failed` is a fault; `waiting` is neither, and covers the two results that
 * report a run which never got to do anything.
 */
export type SyncGuidanceKind = "blocked" | "failed" | "waiting";

export interface SyncGuidance {
  kind: SyncGuidanceKind;
  /** What happened, in one sentence, naming the half it happened in. */
  headline: string;
  /** The safe next action, or what to wait for when there is nothing to do. */
  remediation: string;
}

/** How each half reads inside a sentence about it. */
const PHASE_NAMES: Record<SyncPhase, string> = {
  source: "Reading the library",
  targets: "Writing to Wahoo",
};

interface CategoryGuidance {
  kind: SyncGuidanceKind;
  what: string;
  remediation: string;
}

/**
 * One entry per category in `SYNC_FAILURE_CATEGORIES`, keyed by the category
 * itself, so a category added to that list without an entry here fails to
 * compile. The list is kept in step with the service by hand: `failure` arrives
 * as a plain string and nothing across the wire can enforce that. A category
 * the list does not know falls through to `UNRECOGNISED` rather than reaching
 * the page as itself.
 *
 * `empty_source` and `deletion_limit` are the two deletion gates, and both are
 * `blocked`: the library is intact on both targets, and the way past either is
 * a deliberate configuration change rather than another run.
 */
const CATEGORY_GUIDANCE: Record<SyncFailureCategory, CategoryGuidance> = {
  state: {
    kind: "failed",
    what: "stored state could not be read or written safely",
    remediation:
      "Check that the state volume is writable and the state encryption key is the one this state was written with, then run this half again: it reconciles from what is actually there rather than repeating what it tried.",
  },
  source: {
    kind: "failed",
    what: "the library did not arrive complete or valid",
    remediation:
      "Nothing was deleted: an incomplete read is never treated as routes going away. Wait for the source to recover, then run this half again.",
  },
  authorization: {
    kind: "failed",
    what: "a Wahoo account would not accept this service's authorisation",
    remediation:
      "That target has to be connected again before anything can be written to it. Reconnect it, then run this half again.",
  },
  destination: {
    kind: "failed",
    what: "a Wahoo operation did not complete",
    remediation:
      "The target may hold part of the change. Run this half again once Wahoo is reachable; it reconciles from what is actually there rather than repeating what it tried.",
  },
  course: {
    kind: "failed",
    what: "a stage could not be encoded as a course",
    remediation:
      "The stage was not written and the rest of the run continued. Read the library again once the stage has been corrected at the source.",
  },
  empty_source: {
    kind: "blocked",
    what: "the library came back empty after previously holding stages",
    remediation:
      "Nothing was deleted. If the library really is meant to be empty, set the empty-source deletion acknowledgement deliberately and run this half again; if it is not, treat this as the source being wrong and leave the gate closed.",
  },
  deletion_limit: {
    kind: "blocked",
    what: "the run would have removed more owned routes than the configured maximum",
    remediation:
      "Nothing was deleted. Satisfy yourself that the removals are intended, then raise the per-run deletion maximum deliberately, or reduce what is being removed and run this half again.",
  },
};

/**
 * A category this build has not heard of.
 *
 * It degrades toward asking the operator to look, the same direction
 * `targetConvergence` degrades in: a reason this page cannot explain is not a
 * reason to imply there is nothing to explain.
 */
const UNRECOGNISED: CategoryGuidance = {
  kind: "failed",
  what: "the run reported a reason this page does not recognise",
  remediation:
    "This page may be older than the service it is talking to. Check the service's own status output for the reason it gave.",
};

/** Results that report a run which never got to do anything. */
const WAITING_GUIDANCE: Record<string, CategoryGuidance> = {
  not_ready: {
    kind: "waiting",
    what: "did not start, because a target still has to be connected",
    remediation:
      "Connect every configured target. Nothing is written while one of them is unconnected.",
  },
  skipped: {
    kind: "waiting",
    what: "did not start, because a sync was already running",
    remediation: "Nothing is wrong. This half runs on its next turn.",
  },
};

function isRecognised(value: string): value is SyncFailureCategory {
  return (SYNC_FAILURE_CATEGORIES as readonly string[]).includes(value);
}

/**
 * What one half's last run should tell the operator, or `undefined` when it
 * succeeded and there is nothing to act on.
 *
 * `result` and `failure` are passed as the service gave them. Neither reaches
 * the returned strings: they select an entry, and the entry is a constant.
 */
export function syncGuidance(
  phase: SyncPhase,
  result: string,
  failure: string | undefined,
): SyncGuidance | undefined {
  if (result === "succeeded") {
    return undefined;
  }

  const waiting = WAITING_GUIDANCE[result];
  if (waiting) {
    return {
      kind: waiting.kind,
      headline: `${PHASE_NAMES[phase]} ${waiting.what}.`,
      remediation: waiting.remediation,
    };
  }

  const entry = failure && isRecognised(failure) ? CATEGORY_GUIDANCE[failure] : UNRECOGNISED;
  // The service's own word wins on blocked, because it is the half that knows
  // whether a gate held. A category that is only ever reported as blocked still
  // reads as blocked when a run reports it any other way, since the target is
  // untouched either way.
  const kind: SyncGuidanceKind = result === "blocked" ? "blocked" : entry.kind;
  const verb = kind === "blocked" ? "stopped" : "could not finish";

  return {
    kind,
    headline: `${PHASE_NAMES[phase]} ${verb}: ${entry.what}.`,
    remediation: entry.remediation,
  };
}

/** How a guidance kind is labelled where one word has to stand for it. */
export const GUIDANCE_LABELS: Record<SyncGuidanceKind, string> = {
  blocked: "Held by a safety gate",
  failed: "Did not finish",
  waiting: "Did not start",
};
