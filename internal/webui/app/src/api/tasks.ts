/**
 * The background activities this page can ask for, by the names the service
 * registers them under.
 *
 * The page asks for a task rather than for an endpoint, so an activity added to
 * the layer needs no route of its own. What each one is over — a phase, a
 * target slot — travels as the argument.
 */

/** TASKS are the registered names. `GET /v1/tasks` lists what a build has. */
export const TASKS = {
  /** Reading every configured source library into the stored inventory. */
  syncSource: "sync:source",
  /**
   * Writing the stored inventory to the targets: every configured slot, or the
   * one the argument names.
   */
  syncTarget: "sync:target",
  /** Deleting every owned route from one target slot. Destructive. */
  syncClear: "sync:clear",
  /** One surface-classification pass over the stored library. */
  surfaceAnnotate: "surface:annotate",
} as const;

/** SYNC_PHASE_TASKS is which task does each half of a synchronization. */
export const SYNC_PHASE_TASKS = {
  source: TASKS.syncSource,
  targets: TASKS.syncTarget,
} as const;

/**
 * How often each half runs unasked, in the words the switch shows. The read is
 * the timely one; a read that stored a library asks for the targets at once, so
 * the target schedule is a backstop behind it rather than a second cadence.
 */
export const SYNC_PHASE_CADENCE = {
  source: "Hourly",
  targets: "Every six hours",
} as const;
