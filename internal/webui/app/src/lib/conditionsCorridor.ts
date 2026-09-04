/**
 * The forecast corridor as ground to fill, not a line to stroke.
 *
 * A wide translucent line cannot draw this. Wherever the route bends tighter
 * than the corridor's own half-width the band folds over itself, and the two
 * translucent fragments blend into a dark streak that reads as weather nobody
 * forecast. So the corridor is built here as filled polygons that never
 * overlap: every shape is differenced against everything already emitted
 * before it is emitted itself, which makes painting the same ground twice
 * impossible rather than unlikely.
 *
 * Buffering is done here rather than taken from a library. The centreline is
 * walked up one side and back down the other at the radius asked for, rounding
 * the outside of each bend, which closes one ring that crosses itself wherever
 * the route bends tighter than the radius; `polygon-clipping` resolves those
 * crossings, and does the differences the rings are cut with. A general
 * geometry engine ships a buffer of its own, at an order of magnitude more
 * bundle for one polyline's worth of it.
 *
 * Concentric rings rather than a gradient: the core out to `coreMetres` at full
 * strength, then annuli stepping down toward nothing at `edgeMetres`, so the
 * corridor still fades away instead of ending on a drawn boundary — a boundary
 * reads as a front that is not there.
 *
 * Pure: no DOM, no colour, no MapLibre. A band is a number here; what it looks
 * like belongs to `measures.ts` and to the layer.
 */

import polygonClipping from "polygon-clipping";
import type { Position } from "../api/types";
import { corridorRadii, corridorWeight } from "./conditionsField";
import { haversineMetres } from "./profile";

/**
 * `polygon-clipping` declares its geometry types but does not export them at
 * runtime, so the three this needs are named here: a point, a closed ring of
 * them, and a set of polygons that may have holes — GeoJSON `MultiPolygon`
 * coordinates, in metres rather than degrees until the last step.
 */
type Pair = [number, number];
type Ring = Pair[];
type MultiPolygon = Ring[][];

const { difference, union } = polygonClipping;

/**
 * One degree of latitude on the same spherical model `haversineMetres` uses, so
 * a radius in metres here is in the units the route's own distances are in.
 */
const METRES_PER_DEGREE_LATITUDE = haversineMetres([0, 0], [0, 1]);

/**
 * How finely a rounded bend or end cap is drawn. A sixteenth of a turn cuts the
 * corner by two percent of the radius — thirty metres on a kilometre and a
 * half, well inside the grid cell the reading came from.
 *
 * It also fixes how much smaller one ring is than the next it has to nest
 * inside: a chord at this step reaches 98% of the radius, and every radius here
 * is comfortably further apart than that.
 */
const ARC_STEP_RADIANS = Math.PI / 8;

/** How many stepped rings carry the fade from the core radius out to the edge. */
export const FADE_RINGS = 5;

/**
 * How far apart the centreline is resampled, as a share of the core radius.
 *
 * The corridor is kilometres wide, so a vertex every few metres moves no
 * boundary that can be seen while multiplying the cost of every boolean
 * operation the rings are built from.
 */
const RESAMPLE_CORE_FRACTION = 0.25;

/**
 * The most points one corridor is built from, which widens the interval on a
 * very long route rather than letting the cost grow with the ride. It bites
 * past about 160 km; below that the interval is the core radius' quarter.
 */
const MAX_RESAMPLE_POINTS = 400;

/** Two distances closer than this along the route are the same place. */
const EPSILON_METRES = 1e-6;

/** One stretch of route over which the reading stays inside one band. */
export interface BandRun {
  /** The band, indexing the measure's own table. */
  band: number;
  fromMetres: number;
  toMetres: number;
}

/** One filled ring of one run's corridor, ready to become a map feature. */
export interface CorridorRing {
  band: number;
  /** How strongly to paint it: 1 in the core, stepping toward 0 at the edge. */
  strength: number;
  /** Longitude/latitude rings, as a GeoJSON `MultiPolygon`'s coordinates. */
  polygon: Position[][][];
}

/** A flat-earth frame: metres east and north of one origin on the route. */
interface Frame {
  origin: Position;
  longitudeScale: number;
}

/**
 * The frame for a route, scaled at its middle latitude.
 *
 * Mercator's cosine is the only thing that varies, and over the degree a ride
 * spans it moves the corridor's width by a fraction of a percent — far less
 * than the vagueness the corridor is drawn to admit to in the first place.
 */
function frameFor(coordinates: Position[]): Frame {
  const origin = coordinates[0] ?? [0, 0];
  const middle = coordinates[Math.floor(coordinates.length / 2)] ?? origin;
  // Clamped the way routeCues.ts's offsetPosition() clamps the same cosine:
  // near a pole it runs to nought, and toGeographic() divides by it.
  const longitudeScale = Math.max(Math.cos((middle[1] * Math.PI) / 180), 1e-6);

  return { origin, longitudeScale };
}

function toPlanar(frame: Frame, position: Position): Pair {
  return [
    (position[0] - frame.origin[0]) * METRES_PER_DEGREE_LATITUDE * frame.longitudeScale,
    (position[1] - frame.origin[1]) * METRES_PER_DEGREE_LATITUDE,
  ];
}

function toGeographic(frame: Frame, point: Pair): Position {
  return [
    frame.origin[0] + point[0] / (METRES_PER_DEGREE_LATITUDE * frame.longitudeScale),
    frame.origin[1] + point[1] / METRES_PER_DEGREE_LATITUDE,
  ];
}

/**
 * A point on the millimetre grid.
 *
 * Sines and cosines land a hair off the round number the geometry means — a
 * quarter turn comes back as 1.8e-13 rather than nought — and a vertex that
 * far off a line it should be on is exactly the input a sweep line cannot
 * order. Rounding to a millimetre makes coincidences exact, and a millimetre
 * is nothing against a corridor measured in kilometres.
 */
function snap(east: number, north: number): Pair {
  return [Math.round(east * 1000) / 1000, Math.round(north * 1000) / 1000];
}

/** Points along an arc of `sweep` radians, signed, from `fromAngle`. */
function arcPoints(centre: Pair, radius: number, fromAngle: number, sweep: number): Pair[] {
  const steps = Math.max(1, Math.ceil(Math.abs(sweep) / ARC_STEP_RADIANS));
  const points: Pair[] = [];
  for (let step = 0; step <= steps; step++) {
    const angle = fromAngle + (sweep * step) / steps;
    points.push(snap(centre[0] + radius * Math.cos(angle), centre[1] + radius * Math.sin(angle)));
  }

  return points;
}

function heading(from: Pair, to: Pair): number {
  return Math.atan2(to[1] - from[1], to[0] - from[0]);
}

/** An angle brought back into (-π, π], so a turn reads as left or right. */
function normalise(angle: number): number {
  return Math.atan2(Math.sin(angle), Math.cos(angle));
}

/**
 * One closed ring `radius` out from the path: up the left of it, round the far
 * end, and back down what was the right.
 *
 * Only the outside of a bend is rounded. On the inside the two offsets cross,
 * and on a bend tighter than the radius the ring crosses itself outright —
 * both are left for the boolean to resolve, which is what a fold has to be
 * turned into ground covered once rather than ground painted twice.
 */
function offsetRing(points: Pair[], radius: number, roundEnd: boolean): Ring {
  const ring: Ring = [];
  // A straight (or nearly straight) stretch offsets two segments to the same
  // point — the next segment's `from` is this one's `to` — so every push here
  // goes through this to keep the ring free of the zero-length edges a sweep
  // line reads as ambiguous.
  const push = (point: Pair) => {
    const last = ring[ring.length - 1];
    if (!last || last[0] !== point[0] || last[1] !== point[1]) {
      ring.push(point);
    }
  };
  const walk = (sequence: Pair[], cap: boolean) => {
    for (let index = 0; index + 1 < sequence.length; index++) {
      const from = sequence[index] as Pair;
      const to = sequence[index + 1] as Pair;
      const left = heading(from, to) + Math.PI / 2;
      const east = radius * Math.cos(left);
      const north = radius * Math.sin(left);
      push(snap(from[0] + east, from[1] + north));
      push(snap(to[0] + east, to[1] + north));
      const next = sequence[index + 2];
      if (next) {
        const turn = normalise(heading(to, next) - heading(from, to));
        if (turn < 0) {
          for (const point of arcPoints(to, radius, left, turn)) {
            push(point);
          }
        }
      } else if (cap) {
        for (const point of arcPoints(to, radius, left, -Math.PI)) {
          push(point);
        }
      }
    }
  };
  walk(points, roundEnd);
  walk([...points].reverse(), true);

  return ring;
}

/**
 * How many segments of the centreline go into one ribbon before the next one
 * starts.
 *
 * A ribbon over the whole run would cross itself once per bend tighter than the
 * radius, and resolving fifty crossings at once is both slow and the input
 * these libraries are least reliable on. A short ribbon folds over itself
 * rarely, and unioning a handful of them is the shape of problem they are good
 * at.
 */
const RIBBON_SEGMENTS = 6;

/** The union of a set of polygons, merged in pairs rather than all at once. */
function mergeAll(parts: MultiPolygon[]): MultiPolygon {
  let level = parts.filter((part) => part.length > 0);
  while (level.length > 1) {
    const merged: MultiPolygon[] = [];
    for (let index = 0; index < level.length; index += 2) {
      const one = level[index] as MultiPolygon;
      const other = level[index + 1];
      merged.push(other ? union(one, other) : one);
    }
    level = merged;
  }

  return level[0] ?? [];
}

/**
 * Everything within `radius` of the path, folds resolved, as one polygon.
 *
 * `roundEnd` is false where the next run takes over: the ground beyond that
 * point belongs to the next band, and its own round start cap — clipped
 * against this run — fills the wedge a turn leaves outside the corner. Every
 * join between two ribbons of the same run is capped round on both sides, and
 * the union takes the overlap back out.
 */
function bufferPath(points: Pair[], radius: number, roundEnd: boolean): MultiPolygon {
  const only = points[0];
  if (!only) {
    return [];
  }
  if (points.length === 1) {
    return union([arcPoints(only, radius, 0, 2 * Math.PI).slice(0, -1)]);
  }
  const ribbons: MultiPolygon[] = [];
  for (let start = 0; start + 1 < points.length; start += RIBBON_SEGMENTS) {
    const last = start + RIBBON_SEGMENTS + 1 >= points.length;
    ribbons.push(
      union([
        offsetRing(points.slice(start, start + RIBBON_SEGMENTS + 1), radius, !last || roundEnd),
      ]),
    );
  }

  return mergeAll(ribbons);
}

/**
 * The run's centreline, resampled at `intervalMetres` and clipped to the run,
 * with both of its ends kept exactly.
 */
function runPoints(
  coordinates: Position[],
  distances: number[],
  run: BandRun,
  intervalMetres: number,
  frame: Frame,
): Pair[] {
  const span = Math.max(run.toMetres - run.fromMetres, 0);
  const steps = Math.max(1, Math.ceil(span / intervalMetres));
  const points: Pair[] = [];
  let index = 0;
  for (let step = 0; step <= steps; step++) {
    const target = run.fromMetres + (span * step) / steps;
    while (index < coordinates.length - 2 && (distances[index + 1] ?? 0) < target) {
      index++;
    }
    const from = coordinates[index];
    const to = coordinates[index + 1] ?? from;
    if (!from || !to) {
      break;
    }
    const start = distances[index] ?? 0;
    const end = distances[index + 1] ?? start;
    const ratio = end > start ? Math.min(Math.max((target - start) / (end - start), 0), 1) : 0;
    const point = toPlanar(frame, [
      from[0] + ratio * (to[0] - from[0]),
      from[1] + ratio * (to[1] - from[1]),
    ]);
    const previous = points[points.length - 1];
    // A repeated point has no heading to offset from, and would put a
    // zero-length segment into the ring for the boolean to trip over.
    if (!previous || previous[0] !== point[0] || previous[1] !== point[1]) {
      points.push(point);
    }
  }

  return points;
}

/** The core radius, then one radius per fade ring, the last of them the edge. */
function ringRadii(coreMetres: number, edgeMetres: number): number[] {
  const radii = [coreMetres];
  for (let ring = 1; ring <= FADE_RINGS; ring++) {
    radii.push(coreMetres + ((edgeMetres - coreMetres) * ring) / FADE_RINGS);
  }

  return radii;
}

/**
 * How strongly each ring paints, read off `corridorWeight` at the middle of the
 * ground that ring covers rather than stepped evenly.
 *
 * The falloff is already defined once, in `conditionsField`; sampling it here
 * keeps the drawing and the definition from drifting apart, and because it
 * eases out rather than descending in equal steps, the outermost ring — the one
 * boundary that meets bare ground, and so the one the eye finds — arrives
 * almost transparent.
 */
function ringStrengths(radii: number[], metresPerCell: number): number[] {
  return radii.map((radius, level) => {
    const inner = level === 0 ? 0 : (radii[level - 1] ?? 0);

    return level === 0 ? 1 : corridorWeight((inner + radius) / 2, metresPerCell);
  });
}

/**
 * A boolean operation, or null where the library could not finish it.
 *
 * `polygon-clipping` gives up on some arrangements of coincident edges — a
 * sweep line is a fussy thing — and one shape it cannot cut is a gap in the
 * wash, where an exception would be the route page. Everything emitted was
 * still differenced against everything before it, so what survives a refusal
 * is a smaller corridor, not one painted twice anywhere.
 */
function attempt(operation: () => MultiPolygon): MultiPolygon | null {
  try {
    return operation();
  } catch {
    return null;
  }
}

/**
 * The corridor for a banded route: concentric rings around each run, none of
 * which shares any ground with any other.
 *
 * Nearest claim first — every run's core before any run's fade, and along the
 * route within each level — and each shape differenced against everything
 * already painted. That order is what keeps a neighbouring band's faint outer
 * ring from taking ground its own core is entitled to.
 *
 * `distances` is `cumulativeMetres(coordinates)`, supplied by the caller rather
 * than walked again here. Runs are expected in route order, and a run whose
 * band paints nothing is expected to have been left out already: this knows
 * about bands as numbers, not about what they are worth.
 */
export function corridorRings(
  coordinates: Position[],
  distances: number[],
  runs: BandRun[],
  metresPerCell: number,
): CorridorRing[] {
  const { coreMetres, edgeMetres } = corridorRadii(metresPerCell);
  if (
    coordinates.length < 2 ||
    coordinates.length !== distances.length ||
    runs.length === 0 ||
    coreMetres <= 0 ||
    edgeMetres <= coreMetres
  ) {
    return [];
  }
  const frame = frameFor(coordinates);
  const totalMetres = distances[distances.length - 1] ?? 0;
  const intervalMetres = Math.max(
    coreMetres * RESAMPLE_CORE_FRACTION,
    totalMetres / MAX_RESAMPLE_POINTS,
  );
  const radii = ringRadii(coreMetres, edgeMetres);
  const strengths = ringStrengths(radii, metresPerCell);
  // One resampling per run, buffered at every radius: the rings are
  // differences between these, so the same points have to underlie all of them
  // or one ring would not nest inside the next.
  const buffers = runs.map((run, index) => {
    const points = runPoints(coordinates, distances, run, intervalMetres, frame);
    const next = runs[index + 1];
    const roundEnd = !next || next.fromMetres > run.toMetres + EPSILON_METRES;

    return radii.map((radius) => attempt(() => bufferPath(points, radius, roundEnd)));
  });

  const rings: CorridorRing[] = [];
  let painted: MultiPolygon = [];
  for (let level = 0; level < radii.length; level++) {
    for (const [index, run] of runs.entries()) {
      const buffer = buffers[index]?.[level];
      if (!buffer || buffer.length === 0) {
        continue;
      }
      // Claimed before it is cut: a shape whose claim could not be recorded is
      // dropped rather than painted, because ground painted twice is the one
      // thing this must not do.
      const claimed = attempt(() => (painted.length > 0 ? union(painted, buffer) : buffer));
      if (!claimed) {
        continue;
      }
      const shape = attempt(() => (painted.length > 0 ? difference(buffer, painted) : buffer));
      painted = claimed;
      if (shape && shape.length > 0) {
        rings.push({
          band: run.band,
          strength: strengths[level] ?? 0,
          polygon: shape.map((polygon) =>
            polygon.map((ring) => ring.map((point) => toGeographic(frame, point))),
          ),
        });
      }
    }
  }

  return rings;
}
