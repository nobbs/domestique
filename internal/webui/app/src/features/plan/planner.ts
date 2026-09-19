import type { PlanAvoid } from "../../api/generated";
import {
  PLAN_PROFILES,
  type Plan,
  type PlanProfile,
  type PlanWaypoint,
  type Position,
} from "../../api/types";
import { cumulativeMetres } from "../../lib/profile";

export interface PlannerState {
  name: string;
  profile: PlanProfile;
  cues: boolean;
  waypoints: PlannerWaypoint[];
  nextWaypointID: number;
  avoid: PlannerAvoid[];
  nextAvoidID: number;
  past: PlannerSnapshot[];
  future: PlannerSnapshot[];
}

interface PlannerSnapshot {
  name: string;
  profile: PlanProfile;
  cues: boolean;
  waypoints: PlannerWaypoint[];
  nextWaypointID: number;
  avoid: PlannerAvoid[];
  nextAvoidID: number;
}

export interface PlannerWaypoint extends PlanWaypoint {
  id: number;
}

export interface PlannerAvoid extends PlanAvoid {
  id: number;
}

/** A library route a copy is traced along, and the vertex of it each waypoint sits on. */
export interface PlannerTrace {
  route: Position[];
  indices: number[];
}

/** A new local plan prefilled from the route currently open in the library. */
export interface PlannerSeed {
  name: string;
  profile: PlanProfile;
  waypoints: PlanWaypoint[];
  trace?: PlannerTrace;
}

/** The service refuses a plan with more waypoints than this. */
export const MAX_PLAN_WAYPOINTS = 200;

/** How far a copy's first waypoints may cut a corner; tracing adds what routing gets wrong. */
const SEED_TOLERANCE_METRES = 1500;

/** How far a traced route may stray from the library route before a waypoint is added. */
const TRACE_TOLERANCE_METRES = 50;

const METRES_PER_DEGREE = (Math.PI / 180) * 6_371_000;

/** Flat metres around one latitude: exact enough across a single leg of a route. */
function projector(latitude: number): (position: Position) => [number, number] {
  const east = METRES_PER_DEGREE * Math.cos((latitude * Math.PI) / 180);
  return ([longitude, lat]) => [longitude * east, lat * METRES_PER_DEGREE];
}

function segmentDistance(
  [x, y]: [number, number],
  [ax, ay]: [number, number],
  [bx, by]: [number, number],
): number {
  const dx = bx - ax;
  const dy = by - ay;
  const length = dx * dx + dy * dy;
  const along =
    length === 0 ? 0 : Math.max(0, Math.min(1, ((x - ax) * dx + (y - ay) * dy) / length));
  return Math.hypot(x - ax - along * dx, y - ay - along * dy);
}

function isPosition(value: unknown): value is Position {
  if (!Array.isArray(value)) {
    return false;
  }
  const [longitude, latitude] = value;
  return (
    Number.isFinite(longitude) &&
    Number.isFinite(latitude) &&
    longitude >= -180 &&
    longitude <= 180 &&
    latitude >= -90 &&
    latitude <= 90
  );
}

/** The vertices Douglas–Peucker keeps at toleranceMetres, both ends always among them. */
function simplifiedIndices(line: Position[], toleranceMetres: number): number[] {
  const project = projector(line[0]?.[1] ?? 0);
  const points = line.map(project);
  const kept = new Set([0, line.length - 1]);
  const spans: Array<[number, number]> = [[0, line.length - 1]];
  for (let span = spans.pop(); span; span = spans.pop()) {
    const [start, end] = span;
    const from = points[start];
    const to = points[end];
    let farthest = -1;
    let distance = toleranceMetres;
    for (let index = start + 1; index < end && from && to; index++) {
      const point = points[index];
      const away = point ? segmentDistance(point, from, to) : 0;
      if (away > distance) {
        farthest = index;
        distance = away;
      }
    }
    if (farthest !== -1) {
      kept.add(farthest);
      spans.push([start, farthest], [farthest, end]);
    }
  }

  return [...kept].sort((a, b) => a - b);
}

/** Which of points lies farthest from line, past toleranceMetres; -1 when none does. */
function farthestFrom(
  points: Array<[number, number]>,
  line: Array<[number, number]>,
  toleranceMetres: number,
): number {
  let farthest = -1;
  let distance = toleranceMetres;
  const segments = line.length - 1;
  // Where the previous point found a close segment: a point further along the line
  // finds one at once, and wrapping round still reaches every segment.
  let cursor = 0;
  points.forEach((point, index) => {
    let nearest = Number.POSITIVE_INFINITY;
    for (let step = 0; step < segments && nearest > distance; step++) {
      const at = (cursor + step) % segments;
      const from = line[at];
      const to = line[at + 1];
      if (from && to) {
        nearest = Math.min(nearest, segmentDistance(point, from, to));
        if (nearest <= distance) {
          cursor = at;
        }
      }
    }
    if (nearest > distance) {
      farthest = index;
      distance = nearest;
    }
  });

  return farthest;
}

/**
 * Where the routed legs still stray from the route they trace, either way: a
 * leg that misses part of the route, or one that wanders off it and back. For
 * each such leg, the route vertex to add, with the waypoint index it goes before.
 */
function traceAdditions(
  trace: PlannerTrace,
  legs: Position[][],
  toleranceMetres = TRACE_TOLERANCE_METRES,
): Array<{ index: number; routeIndex: number }> {
  const project = projector(trace.route[0]?.[1] ?? 0);
  const additions: Array<{ index: number; routeIndex: number }> = [];
  legs.forEach((leg, index) => {
    const start = trace.indices[index];
    const end = trace.indices[index + 1];
    const routed = leg.map(project);
    if (start === undefined || end === undefined || routed.length < 2) {
      return;
    }
    const stretch = trace.route.slice(start, end + 1).map(project);
    const interior = stretch.slice(1, -1);
    const missed = farthestFrom(interior, routed, toleranceMetres);
    if (missed !== -1) {
      additions.push({ index: index + 1, routeIndex: start + 1 + missed });
      return;
    }
    // A leg's own ends are left out: its cut may run a vertex past a waypoint, and a
    // waypoint off the road starts the leg at the point the engine snapped it to.
    const between = routed.slice(1, -1);
    const wandered = between[farthestFrom(between, stretch, toleranceMetres)];
    if (!wandered || interior.length === 0) {
      return;
    }
    let nearest = 0;
    interior.forEach((vertex, at) => {
      const best = interior[nearest] ?? vertex;
      if (
        Math.hypot(vertex[0] - wandered[0], vertex[1] - wandered[1]) <
        Math.hypot(best[0] - wandered[0], best[1] - wandered[1])
      ) {
        nearest = at;
      }
    });
    additions.push({ index: index + 1, routeIndex: start + 1 + nearest });
  });

  return additions;
}

/** Plan segments grouped under one bounding box, so a far point skips a run at a time. */
const DEVIATION_CHUNK = 64;

/**
 * The stretches of a copied route farther than toleranceMetres from every part
 * of the plan's routed line, each padded by one vertex so it meets the route it
 * interrupts. Order is not weighed: a lap the plan skips on road it rides
 * elsewhere is not a stretch.
 */
export function deviationStretches(
  route: Position[],
  plan: Position[],
  toleranceMetres = TRACE_TOLERANCE_METRES,
): Position[][] {
  if (plan.length < 2 || route.length === 0) {
    return [];
  }
  const project = projector(route[0]?.[1] ?? 0);
  const line = plan.map(project);
  const segments = line.length - 1;
  const boxes: Array<[number, number, number, number]> = [];
  for (let first = 0; first < segments; first += DEVIATION_CHUNK) {
    const run = line.slice(first, Math.min(first + DEVIATION_CHUNK, segments) + 1);
    const xs = run.map(([x]) => x);
    const ys = run.map(([, y]) => y);
    boxes.push([
      Math.min(...xs) - toleranceMetres,
      Math.min(...ys) - toleranceMetres,
      Math.max(...xs) + toleranceMetres,
      Math.max(...ys) + toleranceMetres,
    ]);
  }
  // The run the previous point matched in is tried first: the next point is usually beside it.
  let cursor = 0;
  const far = route.map((position) => {
    const point = project(position);
    for (let step = 0; step < boxes.length; step++) {
      const chunk = (cursor + step) % boxes.length;
      const box = boxes[chunk];
      if (
        !box ||
        point[0] < box[0] ||
        point[0] > box[2] ||
        point[1] < box[1] ||
        point[1] > box[3]
      ) {
        continue;
      }
      const end = Math.min((chunk + 1) * DEVIATION_CHUNK, segments);
      for (let at = chunk * DEVIATION_CHUNK; at < end; at++) {
        const from = line[at];
        const to = line[at + 1];
        if (from && to && segmentDistance(point, from, to) <= toleranceMetres) {
          cursor = chunk;
          return false;
        }
      }
    }
    return true;
  });

  const stretches: Position[][] = [];
  let start = -1;
  far.forEach((isFar, index) => {
    if (isFar && start === -1) {
      start = index;
    } else if (!isFar && start !== -1) {
      stretches.push(route.slice(Math.max(0, start - 1), index + 1));
      start = -1;
    }
  });
  if (start !== -1) {
    stretches.push(route.slice(Math.max(0, start - 1)));
  }

  return stretches;
}

/** How a finished trace read: its waypoint counts, timing, and how closely the plan follows what it copied. */
export interface TraceSummary {
  waypoints: { seed: number; peak: number; final: number };
  rounds: number;
  seconds: number;
  /** Share (0..1) of the copied route outside the strayed stretches, each counted with its one padding vertex a side. */
  followedShare: number;
  strayedStretches: number;
  planKm: number;
  copiedKm: number;
  /** stoppedShort = the waypoint cap or round limit ended it, not a full match. */
  outcome: "complete" | "stoppedShort";
}

function lengthMetres(coordinates: Position[]): number {
  return cumulativeMetres(coordinates).at(-1) ?? 0;
}

export function summariseTrace({
  route,
  stretches,
  seed,
  peak,
  final,
  rounds,
  seconds,
  planMetres,
  incomplete,
}: {
  route: Position[];
  stretches: Position[][];
  seed: number;
  peak: number;
  final: number;
  rounds: number;
  seconds: number;
  planMetres: number;
  incomplete: boolean;
}): TraceSummary {
  const copiedMetres = lengthMetres(route);
  const strayedMetres = stretches.reduce((total, stretch) => total + lengthMetres(stretch), 0);

  return {
    waypoints: { seed, peak, final },
    rounds,
    seconds,
    followedShare: copiedMetres > 0 ? 1 - strayedMetres / copiedMetres : 1,
    strayedStretches: stretches.length,
    planKm: planMetres / 1000,
    copiedKm: copiedMetres / 1000,
    outcome: incomplete ? "stoppedShort" : "complete",
  };
}

/** Rounds of adding waypoints a trace may spend before it only prunes what it has. */
const MAX_ADD_ROUNDS = 12;

/**
 * A trace under way: it adds waypoints where the routed legs stray, then tests
 * removing them. `removed` are the ones taken out for the round being routed;
 * `settled` are the ones such a round proved necessary. `incomplete` is set when
 * the waypoint cap or the round limit stopped it adding while legs still strayed.
 */
export interface TraceProgress extends PlannerTrace {
  phase: "add" | "prune" | "done";
  settled: number[];
  removed: number[];
  rounds: number;
  incomplete: boolean;
}

export function startTrace(trace: PlannerTrace): TraceProgress {
  return { ...trace, phase: "add", settled: [], removed: [], rounds: 0, incomplete: false };
}

function union(indices: number[], more: number[]): number[] {
  return [...new Set([...indices, ...more])].sort((a, b) => a - b);
}

/** Takes out every other untested interior waypoint, so no two removals share a leg. */
function withRemovals(progress: TraceProgress): TraceProgress | null {
  const indices: number[] = [];
  const removed: number[] = [];
  progress.indices.forEach((index, at) => {
    const interior = at > 0 && at < progress.indices.length - 1;
    const afterRemoval = removed.at(-1) === progress.indices[at - 1];
    if (interior && !afterRemoval && !progress.settled.includes(index)) {
      removed.push(index);
    } else {
      indices.push(index);
    }
  });

  return removed.length === 0
    ? null
    : { ...progress, indices, removed, rounds: progress.rounds + 1 };
}

/**
 * The trace's next waypoints, given the legs routed for its current ones; phase
 * "done", with the waypoints unchanged, once every interior waypoint has been
 * tested and none needs restoring.
 */
export function nextTraceStep(progress: TraceProgress, legs: Position[][]): TraceProgress {
  const straying = traceAdditions(progress, legs);
  if (progress.phase === "add") {
    const indices = union(
      progress.indices,
      straying.map((addition) => addition.routeIndex),
    );
    if (
      straying.length > 0 &&
      progress.rounds < MAX_ADD_ROUNDS &&
      indices.length <= MAX_PLAN_WAYPOINTS
    ) {
      return { ...progress, indices, rounds: progress.rounds + 1 };
    }
    const pruning: TraceProgress = { ...progress, phase: "prune", incomplete: straying.length > 0 };
    return withRemovals(pruning) ?? { ...pruning, phase: "done" };
  }
  const strayingLegs = new Set(straying.map((addition) => addition.index - 1));
  const failed = progress.removed.filter((index) =>
    strayingLegs.has(progress.indices.filter((kept) => kept < index).length - 1),
  );
  const restored: TraceProgress = {
    ...progress,
    indices: union(progress.indices, failed),
    settled: [...progress.settled, ...failed],
    removed: [],
  };

  return withRemovals(restored) ?? (failed.length > 0 ? restored : { ...restored, phase: "done" });
}

/** A copy's first, coarse waypoints over the valid part of a library route. */
function seedTrace(coordinates: Position[]): PlannerTrace | null {
  const route = coordinates.filter(isPosition);
  if (route.length < 2) {
    return null;
  }
  const kept = simplifiedIndices(route, SEED_TOLERANCE_METRES);
  const count = Math.min(kept.length, MAX_PLAN_WAYPOINTS);
  const indices = Array.from(
    { length: count },
    (_, index) => kept[Math.floor((index * (kept.length - 1)) / (count - 1))] ?? 0,
  );

  return { route, indices };
}

/** The service refuses longer plan names, so a copied title is cut to fit. */
export const MAX_PLAN_NAME_LENGTH = 120;

/** An unsaved draft copied from another provider's route; null when it has no name or too short a line. */
export function plannerSeedFrom(title: string, coordinates: Position[]): PlannerSeed | null {
  const trace = seedTrace(coordinates);
  const name = Array.from(title.trim()).slice(0, MAX_PLAN_NAME_LENGTH).join("");

  return trace === null || name === ""
    ? null
    : {
        name,
        profile: "trekking",
        waypoints: trace.indices.map((index) => {
          const [longitude, latitude] = trace.route[index] ?? [0, 0];
          return { longitude, latitude };
        }),
        trace,
      };
}

function isPlannerTrace(value: unknown, waypoints: number): value is PlannerTrace {
  if (!value || typeof value !== "object") {
    return false;
  }
  const { route, indices } = value as Partial<PlannerTrace>;

  return (
    Array.isArray(route) &&
    route.every(isPosition) &&
    Array.isArray(indices) &&
    indices.length === waypoints &&
    indices.every(
      (index, at) =>
        Number.isInteger(index) && index < route.length && index > (indices[at - 1] ?? -1),
    )
  );
}

export function isPlannerSeed(value: unknown): value is PlannerSeed {
  if (!value || typeof value !== "object") {
    return false;
  }
  const candidate = value as Partial<PlannerSeed>;

  return (
    typeof candidate.name === "string" &&
    candidate.name.trim().length > 0 &&
    Array.from(candidate.name).length <= MAX_PLAN_NAME_LENGTH &&
    typeof candidate.profile === "string" &&
    PLAN_PROFILES.includes(candidate.profile as PlanProfile) &&
    Array.isArray(candidate.waypoints) &&
    candidate.waypoints.length >= 2 &&
    candidate.waypoints.length <= MAX_PLAN_WAYPOINTS &&
    candidate.waypoints.every(
      (waypoint) =>
        waypoint !== null &&
        typeof waypoint === "object" &&
        Number.isFinite(waypoint.longitude) &&
        Number.isFinite(waypoint.latitude) &&
        waypoint.longitude >= -180 &&
        waypoint.longitude <= 180 &&
        waypoint.latitude >= -90 &&
        waypoint.latitude <= 90,
    ) &&
    (candidate.trace === undefined || isPlannerTrace(candidate.trace, candidate.waypoints.length))
  );
}

export type PlannerAction =
  | { type: "setName"; name: string }
  | { type: "setProfile"; profile: PlanProfile }
  | { type: "setCues"; cues: boolean }
  | { type: "append"; waypoint: PlanWaypoint }
  | { type: "insert"; index: number; waypoint: PlanWaypoint }
  | { type: "insertMany"; waypoints: PlanWaypoint[] }
  | { type: "move"; index: number; waypoint: PlanWaypoint }
  | { type: "snap"; id: number; from: PlanWaypoint; waypoint: PlanWaypoint }
  | { type: "delete"; index: number }
  | { type: "reverse" }
  | { type: "reorder"; index: number; direction: "up" | "down" }
  | { type: "reorder"; order: number[] }
  | { type: "setStraight"; id: number; straight: boolean }
  | { type: "trace"; waypoints: Array<PlanWaypoint & { id?: number }> }
  | { type: "addAvoid"; longitude: number; latitude: number }
  | { type: "moveAvoid"; id: number; longitude: number; latitude: number }
  | { type: "setAvoidRadius"; id: number; radiusMetres: number }
  | { type: "deleteAvoid"; id: number }
  | { type: "undo" }
  | { type: "redo" }
  | { type: "reset" }
  | {
      type: "load";
      plan: Pick<Plan, "name" | "profile" | "waypoints"> & { cues?: boolean; avoid?: PlanAvoid[] };
    };

/**
 * The same place, read within one turn of the globe. A click or a marker drag
 * on a wrapped copy of the world answers a longitude beyond ±180, which the
 * service refuses as out of range.
 */
export function unwrapped(waypoint: PlanWaypoint): PlanWaypoint {
  if (waypoint.longitude >= -180 && waypoint.longitude <= 180) {
    return waypoint;
  }
  const longitude = ((((waypoint.longitude + 180) % 360) + 360) % 360) - 180;

  return { ...waypoint, longitude };
}

/** The first waypoint can never be straight; every waypoint mutation runs through here. */
function normaliseWaypoints(waypoints: PlannerWaypoint[]): PlannerWaypoint[] {
  const first = waypoints[0];
  if (!first?.straight) {
    return waypoints;
  }
  const { straight: _straight, ...rest } = first;
  return [rest as PlannerWaypoint, ...waypoints.slice(1)];
}

function snapshot({
  name,
  profile,
  cues,
  waypoints,
  nextWaypointID,
  avoid,
  nextAvoidID,
}: PlannerState): PlannerSnapshot {
  return { name, profile, cues, waypoints, nextWaypointID, avoid, nextAvoidID };
}

function apply(state: PlannerState, next: PlannerSnapshot): PlannerState {
  return {
    ...next,
    waypoints: normaliseWaypoints(next.waypoints),
    past: [...state.past, snapshot(state)],
    future: [],
  };
}

/** Where a point falls against a route: the leg it is nearest, and how far along it. */
interface LegPosition {
  /** The index the point would be inserted at: the end of its leg, or past the finish. */
  index: number;
  /** The point's projection onto that leg, unclamped: under 0 is before it, over 1 past it. */
  along: number;
}

function legPosition(
  waypoints: ReadonlyArray<{ longitude: number; latitude: number }>,
  waypoint: { longitude: number; latitude: number },
): LegPosition {
  let nearest: LegPosition = { index: 1, along: 0 };
  let nearestDistance = Number.POSITIVE_INFINITY;

  for (let index = 0; index < waypoints.length - 1; index++) {
    const start = waypoints[index];
    const end = waypoints[index + 1];
    if (!start || !end) {
      continue;
    }
    const longitudeScale = Math.max(
      0.01,
      Math.cos(((start.latitude + end.latitude + waypoint.latitude) / 3) * (Math.PI / 180)),
    );
    const endX = (end.longitude - start.longitude) * longitudeScale;
    const endY = end.latitude - start.latitude;
    const pointX = (waypoint.longitude - start.longitude) * longitudeScale;
    const pointY = waypoint.latitude - start.latitude;
    const lengthSquared = endX * endX + endY * endY;
    const projection = lengthSquared === 0 ? 0 : (pointX * endX + pointY * endY) / lengthSquared;
    const fraction = Math.max(0, Math.min(1, projection));
    const distance = (pointX - endX * fraction) ** 2 + (pointY - endY * fraction) ** 2;

    if (distance < nearestDistance) {
      nearestDistance = distance;
      const pastFinish = index === waypoints.length - 2 && projection >= 1;
      nearest = { index: pastFinish ? waypoints.length : index + 1, along: projection };
    }
  }

  return nearest;
}

/** Where one new waypoint joins a route of two or more: into its nearest leg, or past the finish. */
export function insertionIndex(
  waypoints: ReadonlyArray<{ longitude: number; latitude: number }>,
  waypoint: { longitude: number; latitude: number },
): number {
  return legPosition(waypoints, waypoint).index;
}

/**
 * Several new waypoints at once, each onto its nearest leg of the route as it
 * stood and in order along it, so the result is the same whatever order they
 * were chosen in. Without a route to order along, they follow in that order.
 */
export function insertAll<T extends { longitude: number; latitude: number }>(
  waypoints: readonly T[],
  added: readonly T[],
): T[] {
  if (waypoints.length < 2) {
    return [...waypoints, ...added];
  }
  const placed = added
    .map((waypoint) => ({ waypoint, ...legPosition(waypoints, waypoint) }))
    .sort((a, b) => a.index - b.index || a.along - b.along);
  const result = [...waypoints];
  // Back to front, so every index still names its slot in the route as it stood.
  for (const { waypoint, index } of placed.reverse()) {
    result.splice(index, 0, waypoint);
  }

  return result;
}

export const initialPlannerState: PlannerState = {
  name: "",
  profile: "trekking",
  cues: false,
  waypoints: [],
  nextWaypointID: 0,
  avoid: [],
  nextAvoidID: 0,
  past: [],
  future: [],
};

export function plannerReducer(state: PlannerState, action: PlannerAction): PlannerState {
  switch (action.type) {
    case "setName":
      return action.name === state.name
        ? state
        : apply(state, { ...snapshot(state), name: action.name });
    case "setProfile":
      return action.profile === state.profile
        ? state
        : apply(state, { ...snapshot(state), profile: action.profile });
    case "setCues":
      return action.cues === state.cues
        ? state
        : apply(state, { ...snapshot(state), cues: action.cues });
    case "append":
      return state.waypoints.length === MAX_PLAN_WAYPOINTS
        ? state
        : apply(state, {
            ...snapshot(state),
            waypoints: [
              ...state.waypoints,
              { ...unwrapped(action.waypoint), id: state.nextWaypointID },
            ],
            nextWaypointID: state.nextWaypointID + 1,
          });
    case "insert": {
      if (state.waypoints.length === MAX_PLAN_WAYPOINTS) {
        return state;
      }
      const index = Math.max(0, Math.min(action.index, state.waypoints.length));

      return apply(state, {
        ...snapshot(state),
        waypoints: [
          ...state.waypoints.slice(0, index),
          { ...unwrapped(action.waypoint), id: state.nextWaypointID },
          ...state.waypoints.slice(index),
        ],
        nextWaypointID: state.nextWaypointID + 1,
      });
    }
    case "insertMany": {
      const room = MAX_PLAN_WAYPOINTS - state.waypoints.length;
      if (room <= 0 || action.waypoints.length === 0) {
        return state;
      }
      const added = action.waypoints
        .slice(0, room)
        .map((waypoint, offset) => ({ ...unwrapped(waypoint), id: state.nextWaypointID + offset }));

      return apply(state, {
        ...snapshot(state),
        waypoints: insertAll(state.waypoints, added),
        nextWaypointID: state.nextWaypointID + added.length,
      });
    }
    case "move": {
      if (!state.waypoints[action.index]) {
        return state;
      }
      const waypoints = [...state.waypoints];
      const current = waypoints[action.index];
      if (!current) {
        return state;
      }
      waypoints[action.index] = { ...current, ...unwrapped(action.waypoint), id: current.id };

      return apply(state, { ...snapshot(state), waypoints });
    }
    case "snap": {
      // Settles a waypoint just placed onto the road beside it. It belongs to the
      // placing, so it rewrites the present rather than adding a step to undo.
      // A reply for a waypoint moved, or an id reused, since it was asked is stale.
      const at = state.waypoints.findIndex(
        (waypoint) =>
          waypoint.id === action.id &&
          waypoint.longitude === action.from.longitude &&
          waypoint.latitude === action.from.latitude,
      );
      if (at < 0) {
        return state;
      }
      const waypoints = [...state.waypoints];
      waypoints[at] = { ...waypoints[at], ...unwrapped(action.waypoint), id: action.id };

      return { ...state, waypoints };
    }
    case "delete":
      return state.waypoints[action.index]
        ? apply(state, {
            ...snapshot(state),
            waypoints: state.waypoints.filter((_, index) => index !== action.index),
          })
        : state;
    case "reverse": {
      if (state.waypoints.length < 2) {
        return state;
      }
      // A straight flag marks the leg arriving at its waypoint, so each moves one along.
      const reversed = [...state.waypoints].reverse();
      const waypoints = reversed.map((waypoint, index) => {
        const { straight: _straight, ...rest } = waypoint;
        return reversed[index - 1]?.straight ? { ...rest, straight: true } : rest;
      });

      return apply(state, { ...snapshot(state), waypoints });
    }
    case "reorder": {
      if ("order" in action) {
        if (
          action.order.length !== state.waypoints.length ||
          new Set(action.order).size !== state.waypoints.length
        ) {
          return state;
        }
        const byID = new Map(state.waypoints.map((waypoint) => [waypoint.id, waypoint]));
        const waypoints = action.order.map((id) => byID.get(id));
        if (waypoints.some((waypoint) => !waypoint)) {
          return state;
        }
        const ordered = waypoints as PlannerWaypoint[];
        return ordered.every((waypoint, index) => waypoint === state.waypoints[index])
          ? state
          : apply(state, { ...snapshot(state), waypoints: ordered });
      }
      const nextIndex = action.index + (action.direction === "up" ? -1 : 1);
      if (!state.waypoints[action.index] || !state.waypoints[nextIndex]) {
        return state;
      }
      const waypoints = [...state.waypoints];
      const [waypoint] = waypoints.splice(action.index, 1);
      if (!waypoint) {
        return state;
      }
      waypoints.splice(nextIndex, 0, waypoint);

      return apply(state, { ...snapshot(state), waypoints });
    }
    case "setStraight": {
      const index = state.waypoints.findIndex((waypoint) => waypoint.id === action.id);
      const current = state.waypoints[index];
      if (index <= 0 || !current || (current.straight ?? false) === action.straight) {
        return state;
      }
      const waypoints = [...state.waypoints];
      waypoints[index] = { ...current, straight: action.straight };

      return apply(state, { ...snapshot(state), waypoints });
    }
    case "addAvoid":
      return state.avoid.length === 20
        ? state
        : apply(state, {
            ...snapshot(state),
            avoid: [
              ...state.avoid,
              {
                id: state.nextAvoidID,
                longitude: action.longitude,
                latitude: action.latitude,
                radiusMetres: 250,
              },
            ],
            nextAvoidID: state.nextAvoidID + 1,
          });
    case "moveAvoid": {
      const index = state.avoid.findIndex((area) => area.id === action.id);
      const current = state.avoid[index];
      if (!current) {
        return state;
      }
      const avoid = [...state.avoid];
      avoid[index] = { ...current, longitude: action.longitude, latitude: action.latitude };

      return apply(state, { ...snapshot(state), avoid });
    }
    case "setAvoidRadius": {
      const index = state.avoid.findIndex((area) => area.id === action.id);
      const current = state.avoid[index];
      if (!current || current.radiusMetres === action.radiusMetres) {
        return state;
      }
      const avoid = [...state.avoid];
      avoid[index] = { ...current, radiusMetres: action.radiusMetres };

      return apply(state, { ...snapshot(state), avoid });
    }
    case "deleteAvoid":
      return state.avoid.some((area) => area.id === action.id)
        ? apply(state, {
            ...snapshot(state),
            avoid: state.avoid.filter((area) => area.id !== action.id),
          })
        : state;
    case "undo": {
      const previous = state.past.at(-1);
      if (!previous) {
        return state;
      }

      return {
        ...previous,
        past: state.past.slice(0, -1),
        future: [snapshot(state), ...state.future],
      };
    }
    case "redo": {
      const next = state.future[0];
      if (!next) {
        return state;
      }

      return { ...next, past: [...state.past, snapshot(state)], future: state.future.slice(1) };
    }
    case "reset":
      return initialPlannerState;
    case "trace": {
      // Part of loading a copy, so it leaves no step to undo.
      if (action.waypoints.length < 2 || action.waypoints.length > MAX_PLAN_WAYPOINTS) {
        return state;
      }
      let nextWaypointID = state.nextWaypointID;
      const waypoints = action.waypoints.map(({ id, ...waypoint }) => ({
        ...unwrapped(waypoint),
        id: id ?? nextWaypointID++,
      }));

      return { ...state, waypoints: normaliseWaypoints(waypoints), nextWaypointID };
    }
    case "load": {
      const avoid = (action.plan.avoid ?? []).map((area, id) => ({ ...area, id }));

      return {
        ...action.plan,
        // A copy seeded from a library route carries no switch of its own.
        cues: action.plan.cues ?? false,
        waypoints: normaliseWaypoints(
          action.plan.waypoints.map((waypoint, id) => ({ ...waypoint, id })),
        ),
        nextWaypointID: action.plan.waypoints.length,
        avoid,
        nextAvoidID: avoid.length,
        past: [],
        future: [],
      };
    }
  }
}
