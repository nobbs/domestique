/** The background activities this page can ask for, by the names the service registers them under. */

/** TASKS are the registered names. `GET /v1/tasks` lists what a build has. */
export const TASKS = {
  /** Reading every configured source library into the stored inventory. */
  syncSource: "sync:source",
  /** Writing the stored inventory to every configured target, or the one the argument names. */
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
