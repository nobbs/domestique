/**
 * Validation at the edge of the API boundary.
 *
 * Every response is checked before it reaches a component, so a contract drift
 * between the Go DTOs and this client fails loudly and locally instead of
 * producing `undefined` somewhere far away.
 */

import type {
  BoundingBox,
  BuildInfo,
  Position,
  Stage,
  StageGeometry,
  StageSurface,
  Status,
  SurfaceCoverage,
  SurfaceKind,
  SurfaceRange,
  SyncActive,
  SyncPhase,
  SyncPhaseRun,
  SyncRun,
  SyncRunPage,
  SyncSchedule,
  TargetConvergence,
  TargetRun,
  TargetStatus,
  WebUIConfig,
} from "./types";
import { SURFACE_KINDS, SYNC_PHASES, TARGET_CONVERGENCES } from "./types";

/**
 * A response that did not have the shape this client parses.
 *
 * `where` names the field the check failed on, and `endpoint` the request it
 * came back from — set by the caller in `client.ts`, because the parsers know
 * the shape and only the request knows the URL. A drift between the Go views and
 * this client is then reported as the endpoint and the field it happened at,
 * rather than as a value that turned out to be undefined somewhere later.
 */
export class ContractError extends Error {
  readonly where: string;
  readonly endpoint: string | undefined;

  constructor(where: string, endpoint?: string, options?: ErrorOptions) {
    super(
      endpoint === undefined
        ? `unexpected API response: ${where}`
        : `unexpected API response from ${endpoint}: ${where}`,
      options,
    );
    this.name = "ContractError";
    this.where = where;
    this.endpoint = endpoint;
  }

  /**
   * The same failure, attributed to the request it arrived on.
   *
   * The unattributed failure travels along as the cause, because its stack is
   * the one that points at the check that failed. A new error is what carries
   * the endpoint into the message a reader sees, and the message is what the
   * page shows; the trace worth following is one level down.
   */
  at(endpoint: string): ContractError {
    return new ContractError(this.where, endpoint, { cause: this });
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

/**
 * Reads a position in an array.
 *
 * An index is not a measurement: a fractional one addresses nothing, and a
 * negative one is read by `Array.prototype.slice` as a count back from the far
 * end, which would place a stretch of route somewhere it was never measured.
 * Both are rejected here so no consumer has to guess what was meant.
 */
function index(value: unknown, at: string): number {
  const position = count(value, at);
  if (!Number.isInteger(position) || position < 0) {
    throw new ContractError(`${at} is not a non-negative integer`);
  }
  return position;
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

/**
 * Reads one class name, degrading an unfamiliar one to `unknown`.
 *
 * This is the one place the client is deliberately forgiving rather than loud.
 * The service may grow a class this build has never heard of, and the honest
 * rendering of a stretch whose class means nothing here is the same as one
 * nobody surveyed — an unfamiliar name must not take the page down with it.
 */
function surfaceKind(value: unknown, at: string): SurfaceKind {
  const name = text(value, at);

  return (SURFACE_KINDS as readonly string[]).includes(name) ? (name as SurfaceKind) : "unknown";
}

function surfaceRangeFrom(value: unknown, at: string): SurfaceRange {
  const range = record(value, at);
  const startIndex = index(range.start_index, `${at}.start_index`);
  const endIndex = index(range.end_index, `${at}.end_index`);
  if (endIndex < startIndex) {
    throw new ContractError(`${at} ends before it starts`);
  }

  return { kind: surfaceKind(range.kind, `${at}.kind`), startIndex, endIndex };
}

/**
 * Reads the surface group, which is absent until this exact geometry has been
 * classified. Absent is not the same as classified while nothing matched, so it
 * stays `undefined` rather than becoming a surface with no matched length.
 */
function surfaceFrom(value: unknown, at: string): StageSurface | undefined {
  if (value === undefined || value === null) {
    return undefined;
  }
  const surface = record(value, at);

  return {
    ranges: array(surface.ranges, `${at}.ranges`).map((entry, index) =>
      surfaceRangeFrom(entry, `${at}.ranges[${index}]`),
    ),
    matchedMetres: count(surface.matched_metres, `${at}.matched_metres`),
  };
}

export function parseStageGeometry(payload: unknown): StageGeometry {
  const body = record(payload, "body");
  const geometry = record(body.geometry, "body.geometry");
  const properties = record(body.properties, "body.properties");
  const bboxValues = array(body.bbox, "body.bbox").map((entry, index) =>
    count(entry, `body.bbox[${index}]`),
  );
  if (bboxValues.length !== 4) {
    throw new ContractError("body.bbox needs exactly four numbers");
  }

  return {
    stage: stageFrom(properties, "body.properties"),
    bbox: bboxValues as BoundingBox,
    coordinates: array(geometry.coordinates, "body.geometry.coordinates").map((entry, index) =>
      positionFrom(entry, `body.geometry.coordinates[${index}]`),
    ),
    surface: surfaceFrom(properties.surface, "body.properties.surface"),
  };
}

/**
 * Reads one half's last run. A half is absent until it has finished one, which
 * is different from having finished one that failed.
 */
function syncPhaseRunFrom(value: unknown, at: string): SyncPhaseRun | undefined {
  if (value === undefined || value === null) {
    return undefined;
  }
  const run = record(value, at);

  return {
    lastCompletedAt: text(run.last_completed_at, `${at}.last_completed_at`),
    lastResult: text(run.last_result, `${at}.last_result`),
    lastFailure: optionalText(run.last_failure, `${at}.last_failure`),
    sourceStages: count(run.source_stages, `${at}.source_stages`),
    created: count(run.created, `${at}.created`),
    updated: count(run.updated, `${at}.updated`),
    deleted: count(run.deleted, `${at}.deleted`),
  };
}

/**
 * Reads the two schedule switches.
 *
 * Both are required. A missing switch is not an off switch, and rendering a
 * control from an assumed value would show the operator a state the service
 * never reported.
 */
export function parseSyncSchedule(payload: unknown, at = "body"): SyncSchedule {
  const schedule = record(payload, at);

  return {
    source: flag(schedule.source, `${at}.source`),
    targets: flag(schedule.targets, `${at}.targets`),
  };
}

export function parseStatus(payload: unknown): Status {
  const body = record(payload, "body");
  const sync = record(body.sync, "body.sync");

  return {
    ready: flag(body.ready, "body.ready"),
    build: buildInfoFrom(body.build, "body.build"),
    converged: flag(body.converged, "body.converged"),
    targets: array(body.targets, "body.targets").map((entry, index) =>
      targetFrom(record(entry, `targets[${index}]`), `targets[${index}]`),
    ),
    sync: {
      state: text(sync.state, "body.sync.state"),
      active: syncActiveFrom(sync.active, "body.sync.active"),
      lastResult: optionalText(sync.last_result, "body.sync.last_result"),
      lastCompletedAt: optionalText(sync.last_completed_at, "body.sync.last_completed_at"),
      sourceStages: count(sync.source_stages, "body.sync.source_stages"),
      created: count(sync.created, "body.sync.created"),
      updated: count(sync.updated, "body.sync.updated"),
      deleted: count(sync.deleted, "body.sync.deleted"),
      schedule: parseSyncSchedule(sync.schedule, "body.sync.schedule"),
      phases: syncPhasesFrom(sync.phases, "body.sync.phases"),
      surface: surfaceCoverageFrom(sync.surface, "body.sync.surface"),
    },
  };
}

/**
 * Reads one word of convergence, degrading an unfamiliar one to `failed`.
 *
 * The forgiving direction matters here. A state this build has never heard of is
 * a state it cannot claim is fine, so it reads as the one that asks the operator
 * to look — the opposite of `surfaceKind`, where the honest fallback is "nobody
 * surveyed this".
 */
function targetConvergence(value: unknown, at: string): TargetConvergence {
  const name = text(value, at);

  return (TARGET_CONVERGENCES as readonly string[]).includes(name)
    ? (name as TargetConvergence)
    : "failed";
}

/** Reads one account's last reconciliation, absent until it has had one. */
function targetRunFrom(value: unknown, at: string): TargetRun | undefined {
  if (value === undefined || value === null) {
    return undefined;
  }
  const run = record(value, at);

  return {
    completedAt: text(run.completed_at, `${at}.completed_at`),
    result: text(run.result, `${at}.result`),
    failure: optionalText(run.failure, `${at}.failure`),
  };
}

function targetFrom(target: Record<string, unknown>, at: string): TargetStatus {
  const stages = record(target.stages, `${at}.stages`);

  return {
    id: text(target.id, `${at}.id`),
    authorisation: text(target.authorisation, `${at}.authorisation`),
    convergence: targetConvergence(target.convergence, `${at}.convergence`),
    stages: {
      current: count(stages.current, `${at}.stages.current`),
      pending: count(stages.pending, `${at}.stages.pending`),
    },
    lastRun: targetRunFrom(target.last_run, `${at}.last_run`),
  };
}

/**
 * The build group, or undefined when the service did not name one.
 *
 * A revision the service would not stand behind never arrives — it is dropped
 * where it is served — so anything present here is read as given, and a group
 * without a revision is treated as no group rather than as a contract error: a
 * missing build stamp must not cost the operator the status page.
 */
function buildInfoFrom(value: unknown, at: string): BuildInfo | undefined {
  if (value === undefined || value === null) {
    return undefined;
  }

  const build = record(value, at);
  const revision = optionalText(build.revision, `${at}.revision`);
  if (!revision) {
    return undefined;
  }

  return {
    revision,
    imageDigest: optionalText(build.image_digest, `${at}.image_digest`),
  };
}

function surfaceCoverageFrom(value: unknown, at: string): SurfaceCoverage {
  const coverage = record(value, at);

  return {
    classified: count(coverage.classified, `${at}.classified`),
    total: count(coverage.total, `${at}.total`),
  };
}

/**
 * Reads the group describing a run that has not finished. Absent is the answer
 * that nothing is under way, so it stays absent rather than becoming an empty
 * run nobody started.
 *
 * A half this build has never heard of reads as no half at all: the run is
 * still happening, and the page can say that much without inventing a name for
 * what it is doing.
 */
function syncActiveFrom(value: unknown, at: string): SyncActive | undefined {
  if (value === undefined || value === null) {
    return undefined;
  }
  const active = record(value, at);
  const phase = optionalText(active.phase, `${at}.phase`);
  const stages = record(active.stages, `${at}.stages`);

  return {
    phase: (SYNC_PHASES as readonly string[]).includes(phase ?? "")
      ? (phase as SyncPhase)
      : undefined,
    startsAt: optionalText(active.starts_at, `${at}.starts_at`),
    targets: count(active.targets, `${at}.targets`),
    stages: {
      current: count(stages.current, `${at}.stages.current`),
      pending: count(stages.pending, `${at}.stages.pending`),
    },
  };
}

function syncPhasesFrom(value: unknown, at: string): Partial<Record<SyncPhase, SyncPhaseRun>> {
  const phases = record(value, at);
  const runs: Partial<Record<SyncPhase, SyncPhaseRun>> = {};
  for (const phase of SYNC_PHASES) {
    const run = syncPhaseRunFrom(phases[phase], `${at}.${phase}`);
    if (run) {
      runs[phase] = run;
    }
  }

  return runs;
}

/**
 * Reads one page of the recorded run history.
 *
 * A phase word this build does not know is a drift rather than something to
 * degrade past: every row is labelled by its half, and a row that cannot be
 * labelled would be shown as a run of nothing in particular.
 */
export function parseSyncRuns(payload: unknown): SyncRunPage {
  const body = record(payload, "body");

  return {
    runs: array(body.runs, "body.runs").map((entry, index) =>
      syncRunFrom(record(entry, `body.runs[${index}]`), `body.runs[${index}]`),
    ),
    next: optionalText(body.next, "body.next"),
  };
}

function syncRunFrom(run: Record<string, unknown>, at: string): SyncRun {
  const phase = text(run.phase, `${at}.phase`);
  if (!(SYNC_PHASES as readonly string[]).includes(phase)) {
    throw new ContractError(`${at}.phase is not a known synchronisation half`);
  }

  return {
    reference: text(run.reference, `${at}.reference`),
    phase: phase as SyncPhase,
    completedAt: text(run.completed_at, `${at}.completed_at`),
    result: text(run.result, `${at}.result`),
    failure: optionalText(run.failure, `${at}.failure`),
    sourceStages: count(run.source_stages, `${at}.source_stages`),
    created: count(run.created, `${at}.created`),
    updated: count(run.updated, `${at}.updated`),
    deleted: count(run.deleted, `${at}.deleted`),
  };
}

export function parseWebUIConfig(payload: unknown): WebUIConfig {
  const body = record(payload, "body");
  return {
    tileStyleUrl: text(body.tile_style_url, "body.tile_style_url"),
    tileStyleUrlDark: optionalText(body.tile_style_url_dark, "body.tile_style_url_dark"),
    sourceBaseUrl: optionalText(body.source_base_url, "body.source_base_url"),
  };
}
