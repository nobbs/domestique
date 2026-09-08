import type { Status } from "../api/types";

/**
 * A service with nothing to report, for a page whose subject is not the sync.
 *
 * `MenuBar` reads the status on every page, so a page test that says nothing
 * about it still has to seed it or the query goes to the network.
 */
export const IDLE_STATUS: Status = {
  ready: true,
  converged: true,
  targets: [],
  sync: {
    state: "idle",
    sourceRoutes: 0,
    created: 0,
    updated: 0,
    deleted: 0,
    phases: {},
    surface: { classified: 0, total: 0, incomplete: 0, enrichmentFailures: 0 },
  },
};
