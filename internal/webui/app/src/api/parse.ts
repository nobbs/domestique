/**
 * Validation at the edge of the API boundary.
 *
 * Every response is checked before it reaches a component, so a contract drift
 * between the Go DTOs and this client fails loudly and locally instead of
 * producing `undefined` somewhere far away.
 */

import type { BoundingBox, Position, Stage, StageGeometry, Status, WebUIConfig } from "./types";

export class ContractError extends Error {
  constructor(message: string) {
    super(`unexpected API response: ${message}`);
    this.name = "ContractError";
  }
}

function record(value: unknown, at: string): Record<string, unknown> {
  if (typeof value !== "object" || value === null || Array.isArray(value)) {
    throw new ContractError(`${at} is not an object`);
  }
  return value as Record<string, unknown>;
}

function array(value: unknown, at: string): unknown[] {
  if (!Array.isArray(value)) {
    throw new ContractError(`${at} is not an array`);
  }
  return value;
}

function text(value: unknown, at: string): string {
  if (typeof value !== "string") {
    throw new ContractError(`${at} is not a string`);
  }
  return value;
}

function optionalText(value: unknown, at: string): string | undefined {
  return value === undefined ? undefined : text(value, at);
}

function count(value: unknown, at: string): number {
  if (typeof value !== "number" || !Number.isFinite(value)) {
    throw new ContractError(`${at} is not a finite number`);
  }
  return value;
}

function flag(value: unknown, at: string): boolean {
  if (typeof value !== "boolean") {
    throw new ContractError(`${at} is not a boolean`);
  }
  return value;
}

function stageFrom(source: Record<string, unknown>, at: string): Stage {
  return {
    routeId: count(source.route_id, `${at}.route_id`),
    stageOrder: count(source.stage, `${at}.stage`),
    title: text(source.title, `${at}.title`),
    routeName: text(source.route_name, `${at}.route_name`),
    stageName: text(source.stage_name, `${at}.stage_name`),
    sourceRevision: text(source.source_revision ?? "", `${at}.source_revision`),
    contentHash: text(source.content_hash ?? "", `${at}.content_hash`),
    distanceMetres: count(source.distance_metres, `${at}.distance_metres`),
    ascentMetres: count(source.ascent_metres ?? 0, `${at}.ascent_metres`),
    maxGradientPercent: count(source.max_gradient_percent ?? 0, `${at}.max_gradient_percent`),
    pointCount: count(source.point_count, `${at}.point_count`),
  };
}

export function parseStages(payload: unknown): Stage[] {
  const body = record(payload, "body");
  return array(body.stages, "body.stages").map((entry, index) =>
    stageFrom(record(entry, `stages[${index}]`), `stages[${index}]`),
  );
}

export function parseStage(payload: unknown): Stage {
  return stageFrom(record(payload, "body"), "body");
}

function positionFrom(value: unknown, at: string): Position {
  const entries = array(value, at).map((entry, index) => count(entry, `${at}[${index}]`));
  const [longitude, latitude, elevation] = entries;
  if (longitude === undefined || latitude === undefined) {
    throw new ContractError(`${at} needs at least a longitude and a latitude`);
  }
  return elevation === undefined ? [longitude, latitude] : [longitude, latitude, elevation];
}

export function parseStageGeometry(payload: unknown): StageGeometry {
  const body = record(payload, "body");
  const geometry = record(body.geometry, "body.geometry");
  const bboxValues = array(body.bbox, "body.bbox").map((entry, index) =>
    count(entry, `body.bbox[${index}]`),
  );
  if (bboxValues.length !== 4) {
    throw new ContractError("body.bbox needs exactly four numbers");
  }

  return {
    stage: stageFrom(record(body.properties, "body.properties"), "body.properties"),
    bbox: bboxValues as BoundingBox,
    coordinates: array(geometry.coordinates, "body.geometry.coordinates").map((entry, index) =>
      positionFrom(entry, `body.geometry.coordinates[${index}]`),
    ),
  };
}

export function parseStatus(payload: unknown): Status {
  const body = record(payload, "body");
  const sync = record(body.sync, "body.sync");

  return {
    ready: flag(body.ready, "body.ready"),
    targets: array(body.targets, "body.targets").map((entry, index) => {
      const target = record(entry, `targets[${index}]`);
      return {
        id: text(target.id, `targets[${index}].id`),
        authorisation: text(target.authorisation, `targets[${index}].authorisation`),
      };
    }),
    sync: {
      state: text(sync.state, "body.sync.state"),
      lastResult: optionalText(sync.last_result, "body.sync.last_result"),
      lastCompletedAt: optionalText(sync.last_completed_at, "body.sync.last_completed_at"),
      sourceStages: count(sync.source_stages, "body.sync.source_stages"),
      created: count(sync.created, "body.sync.created"),
      updated: count(sync.updated, "body.sync.updated"),
      deleted: count(sync.deleted, "body.sync.deleted"),
    },
  };
}

export function parseWebUIConfig(payload: unknown): WebUIConfig {
  const body = record(payload, "body");
  return { tileStyleUrl: text(body.tile_style_url, "body.tile_style_url") };
}
