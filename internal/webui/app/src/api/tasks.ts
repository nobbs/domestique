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
  /** One synchronization. The argument names a half, or none for both. */
  sync: "sync",
  /** Reconciling exactly one target slot, named by the argument. */
  syncTarget: "sync:target",
  /** Deleting every owned route from one target slot. Destructive. */
  syncClear: "sync:clear",
  /** One surface-classification pass over the stored library. */
  surfaceAnnotate: "surface:annotate",
} as const;
