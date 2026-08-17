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

export interface StageGeometry {
  stage: Stage;
  bbox: BoundingBox;
  coordinates: Position[];
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
