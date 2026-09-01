/**
 * How a recorded task attempt reads on the page: what its outcome means, and
 * what a `skipped` one's detail says was busy.
 *
 * Mirrors `Outcome` and `Detail` in `internal/task/task.go`. Every label here
 * is a constant; nothing from a run is interpolated into it.
 */

export const TASK_OUTCOMES = [
  "succeeded",
  "failed",
  "blocked",
  "not_ready",
  "skipped",
  "cancelled",
  "unchanged",
] as const;

export type TaskOutcome = (typeof TASK_OUTCOMES)[number];

export const OUTCOME_LABELS: Record<TaskOutcome, string> = {
  succeeded: "Succeeded",
  failed: "Failed",
  blocked: "Held by a safety gate",
  not_ready: "Not ready",
  skipped: "Skipped",
  cancelled: "Cancelled",
  unchanged: "Unchanged",
};

function isTaskOutcome(value: string): value is TaskOutcome {
  return (TASK_OUTCOMES as readonly string[]).includes(value);
}

/** A recorded attempt's outcome, or the wire value itself for one this page does not recognise. */
export function outcomeLabel(outcome: string): string {
  return isTaskOutcome(outcome) ? OUTCOME_LABELS[outcome] : outcome;
}

/** What a `skipped` attempt's detail says was busy: this exact work, or something it needed. */
export const TASK_DETAIL_LABELS: Record<string, string> = {
  already_working: "This work was already happening.",
  resource_held: "Something it needs was held by another run.",
};

/** A `skipped` attempt's detail, or the wire value itself for one this page does not recognise. */
export function taskDetailLabel(detail: string | undefined): string | undefined {
  if (detail === undefined) {
    return undefined;
  }

  return TASK_DETAIL_LABELS[detail] ?? detail;
}
