/**
 * The name each half of a sync goes by, on the sync page and in its
 * history.
 *
 * Writing is always to Wahoo, so that half is fixed. Reading is from whichever
 * sources are configured, so that half's name follows them: exactly one
 * configured source is named — even one this build does not recognise,
 * `providerLabel` falls back to the wire value itself — because that stays the
 * friendlier answer while it is simple. Zero configured sources, or more than
 * one, reads as the library itself rather than naming any single source.
 */

import type { SyncPhase } from "../api/types";
import { providerLabel } from "./provider";

const TARGETS_LABEL = "Write to Wahoo";
const TARGETS_RUNNING_LABEL = "Writing to Wahoo";

function sourceLabel(sourceProviders: string[], verb: "Read" | "Reading"): string {
  const [only, ...rest] = sourceProviders;
  if (only !== undefined && rest.length === 0) {
    return `${verb} from ${providerLabel(only)}`;
  }

  return `${verb} from the source library`;
}

/** The name each half goes by at rest, keyed the way the page reads a phase. */
export function phaseLabels(sourceProviders: string[]): Record<SyncPhase, string> {
  return { source: sourceLabel(sourceProviders, "Read"), targets: TARGETS_LABEL };
}

/** What each half is doing while it is doing it, rather than what to press. */
export function runningPhaseLabels(sourceProviders: string[]): Record<SyncPhase, string> {
  return { source: sourceLabel(sourceProviders, "Reading"), targets: TARGETS_RUNNING_LABEL };
}
