/**
 * Domain types for the browser UI.
 *
 * The wire format is the service's snake_case v1 JSON contract. These types are
 * the camelCase shape the application works with; `parse.ts` is the only place
 * that knows about the difference, so a contract change surfaces there as a
 * clear error rather than as `undefined` deep inside a component.
 */

export interface Stage {
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
 * Both indices are inclusive and address `StageGeometry.coordinates` directly,
 * which is why a classification is only ever served beside the geometry it was
 * measured against.
 */
export interface SurfaceRange {
  kind: SurfaceKind;
  startIndex: number;
  endIndex: number;
}

export interface StageSurface {
  ranges: SurfaceRange[];
  /**
   * The stage length that snapped to a classified way. The honest denominator
   * for any share: a stage that matched a third of its length must not present
   * its split as though it described the whole ride.
   */
  matchedMetres: number;
}

export interface StageGeometry {
  stage: Stage;
  bbox: BoundingBox;
  coordinates: Position[];
  /**
   * Absent until the enrichment pass has classified this exact geometry, which
   * is a different thing from a classification that matched nothing: that
   * arrives as a present group with no ranges.
   */
  surface?: StageSurface | undefined;
}

export interface TargetStatus {
  id: string;
  authorisation: string;
}

export interface SyncStatus {
  state: string;
  /** Absent until a run has completed. */
  lastResult?: string | undefined;
  lastCompletedAt?: string | undefined;
  sourceStages: number;
  created: number;
  updated: number;
  deleted: number;
}

export interface Status {
  ready: boolean;
  targets: TargetStatus[];
  sync: SyncStatus;
}

export interface WebUIConfig {
  tileStyleUrl: string;
}

/** A stage's stable identity, used for routing and list keys. */
export function stageKey(stage: Pick<Stage, "routeId" | "stageOrder">): string {
  return `${stage.routeId}/${stage.stageOrder}`;
}
