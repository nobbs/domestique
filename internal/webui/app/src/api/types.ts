/**
 * Domain types for the browser UI.
 *
 * The wire format is the service's snake_case v1 JSON contract. These types are
 * the camelCase shape the application works with; `parse.ts` is the only place
 * that knows about the difference, so a contract change surfaces there as a
 * clear error rather than as `undefined` deep inside a component.
 */

export interface Route {
  provider: string;
  routeId: number;
  stageOrder: number;
  title: string;
  routeName: string;
  stageName: string;
  sourceRevision: string;
  contentHash: string;
  distanceMetres: number;
  ascentMetres: number;
  maxGradientPercent: number;
  pointCount: number;
}

/** West, south, east, north — the order the map's fitBounds expects. */
export type BoundingBox = [number, number, number, number];

export type Position = [number, number] | [number, number, number];

/**
 * The ground classes the service reports, in the wire names it uses.
 *
 * `unknown` is not a failure to parse: it means nobody has surveyed that stretch
 * in OpenStreetMap. It emphatically does not mean smooth.
 */
export const SURFACE_KINDS = [
  "asphalt",
  "paving",
  "compacted",
  "gravel",
  "ground",
  "unknown",
] as const;

export type SurfaceKind = (typeof SURFACE_KINDS)[number];

/**
 * One run of stage points sharing a class.
 *
 * Both indices are inclusive and address `RouteGeometry.coordinates` directly,
 * which is why a classification is only ever served beside the geometry it was
 * measured against.
 */
export interface SurfaceRange {
  kind: SurfaceKind;
  startIndex: number;
  endIndex: number;
}

export interface RouteSurface {
  ranges: SurfaceRange[];
  /**
   * The stage length that snapped to a classified way. The honest denominator
   * for any share: a stage that matched a third of its length must not present
   * its split as though it described the whole ride.
   */
  matchedMetres: number;
}

export interface RouteGeometry {
  stage: Route;
  bbox: BoundingBox;
  coordinates: Position[];
  /**
   * Absent until the enrichment pass has classified this exact geometry, which
   * is a different thing from a classification that matched nothing: that
   * arrives as a present group with no ranges.
   */
  surface?: RouteSurface | undefined;
}

/**
 * What one Wahoo account amounts to, in one word.
 *
 * `current` means the account holds every stored stage at the revision the
 * library holds now. It is a statement about the Wahoo account, not about any
 * head unit: whether a device has since downloaded those routes is not something
 * this service can see.
 */
export const TARGET_CONVERGENCES = ["current", "lagging", "failed", "unauthorized"] as const;

export type TargetConvergence = (typeof TARGET_CONVERGENCES)[number];

/** How much of the stored library one account holds. Counts only, never names. */
export interface TargetStages {
  current: number;
  /**
   * Everything the account still owes the library: a stage never written, a
   * stage written at an older revision, and a stage the account still carries
   * that the library no longer has.
   */
  pending: number;
}

/** One account's own last reconciliation. */
export interface TargetRun {
  completedAt: string;
  result: string;
  /** The safe failure category, present only when that run did not succeed. */
  failure?: string | undefined;
}

export interface TargetStatus {
  id: string;
  authorisation: string;
  convergence: TargetConvergence;
  stages: TargetStages;
  /**
   * Absent until this account has been reconciled once, which is a different
   * state from having been reconciled and failed.
   */
  lastRun?: TargetRun | undefined;
}

/**
 * The two halves of a synchronisation, in the wire names the service uses.
 *
 * `source` reads the VeloPlanner library into the service's own state; `targets`
 * writes what is stored onto the Wahoo accounts. They are switched, triggered,
 * and reported separately because they fail for unrelated reasons.
 */
export const SYNC_PHASES = ["source", "targets"] as const;

export type SyncPhase = (typeof SYNC_PHASES)[number];

/** What the timer is allowed to start. Absent switches are never assumed. */
export type SyncSchedule = Record<SyncPhase, boolean>;

/** The last completed run of one half. */
export interface SyncPhaseRun {
  lastCompletedAt: string;
  lastResult: string;
  /** The safe failure category, present only when the last run did not succeed. */
  lastFailure?: string | undefined;
  sourceStages: number;
  created: number;
  updated: number;
  deleted: number;
}

/**
 * One terminal run in the recorded history.
 *
 * `reference` is the opaque name the service recorded the run under. It means
 * nothing on its own, which is what makes it safe to send in a notification: it
 * says which run without saying anything about it, and it is what an operator
 * matches a Pushover message against this list with.
 */
export interface SyncRun {
  reference: string;
  phase: SyncPhase;
  completedAt: string;
  result: string;
  /** The safe failure category, present only when the run did not succeed. */
  failure?: string | undefined;
  sourceStages: number;
  created: number;
  updated: number;
  deleted: number;
}

/**
 * One page of that history, newest first.
 *
 * `next` is the cursor the page after this one starts at, and is absent when the
 * history ends here. The service prunes old runs, so the history is recent
 * history rather than a permanent record.
 */
export interface SyncRunPage {
  runs: SyncRun[];
  next?: string | undefined;
}

/**
 * How much of the library carries a usable surface classification.
 *
 * Classification cannot fail a synchronisation, by design — which means a stage
 * the endpoint refuses every time looks exactly like one nobody has asked about
 * yet. These two numbers are the difference.
 */
export interface SurfaceCoverage {
  classified: number;
  total: number;
}

/**
 * Work that has not finished: the half in flight, when a run being held back is
 * due to start, and how much of the library the accounts already hold.
 *
 * Present only while the service reports a state that has no result yet, which
 * is what makes its presence the answer to whether anything is happening.
 */
export interface SyncActive {
  /** Absent until a half of the run has started. */
  phase?: SyncPhase | undefined;
  /** Present only while a run is deliberately being held back. */
  startsAt?: string | undefined;
  /** How many accounts are configured, which is what the counts are across. */
  targets: number;
  /** The aggregate of the per-account counts, and the only progress reported. */
  stages: TargetStages;
}

export interface SyncStatus {
  state: string;
  /** Absent whenever nothing is queued, running, or being held back. */
  active?: SyncActive | undefined;
  /** Absent until a run has completed. */
  lastResult?: string | undefined;
  lastCompletedAt?: string | undefined;
  sourceStages: number;
  created: number;
  updated: number;
  deleted: number;
  schedule: SyncSchedule;
  /** A half is absent here until it has finished a run of its own. */
  phases: Partial<Record<SyncPhase, SyncPhaseRun>>;
  surface: SurfaceCoverage;
}

/**
 * Which public source produced the running service.
 *
 * Absent when the service was not built by CI, and then the page says so rather
 * than offering a link to a commit that does not exist.
 */
export interface BuildInfo {
  /** A full commit object name in the public repository. */
  revision: string;
  /** The running image's digest, when the service was told one. */
  imageDigest?: string | undefined;
}

export interface Status {
  ready: boolean;
  /**
   * True only when every stored stage is current on every configured account.
   * Like the per-account word, it says nothing about device downloads.
   */
  converged: boolean;
  build?: BuildInfo | undefined;
  targets: TargetStatus[];
  sync: SyncStatus;
}

/** One cartography the map can be switched to. */
export interface Basemap {
  /** The label the picker shows, and what a remembered choice names. */
  name: string;
  /** The MapLibre style document to load. */
  styleUrl: string;
  /** Loaded in place of styleUrl under a dark system colour scheme. */
  styleUrlDark?: string | undefined;
  /**
   * Whether this entry's ground is dark whatever the system asks for, which is
   * what satellite imagery is. Anything painted over the map reads this rather
   * than the colour scheme.
   */
  darkCartography: boolean;
}

export interface WebUIConfig {
  /**
   * The cartographies on offer, in the configured order. Never empty; the first
   * is what a browser that has not chosen loads.
   */
  basemaps: Basemap[];
  /**
   * Each configured source's own web application, keyed by provider, from
   * which a link back to a stage's source route is built. A provider missing
   * from this map has no such link offered for its stages, whether because the
   * service cannot name one or because it is not configured at all.
   */
  sourceBaseUrls: Record<string, string>;
}

/** A route's stable identity, used for routing and list keys. */
export function routeKey(route: Pick<Route, "provider" | "routeId" | "stageOrder">): string {
  return `${route.provider}/${route.routeId}/${route.stageOrder}`;
}
