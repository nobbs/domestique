/**
 * The geometry behind drawing a forecast on the map: where a point on the
 * ground sits along the route, how strongly the route's conditions still speak
 * for it out there, and what the readings are at that distance.
 *
 * A corridor hugging the route rather than a grid over the map. The forecast is
 * one-dimensional — a couple of dozen samples strung along the ride, from
 * `forecastSamples.ts` — so a map point's reading is the route's reading at its
 * nearest approach, faded out by how far off the route it lies. Inverse-distance
 * weighting between sample positions would instead invent a field across the
 * whole map, filling the ground between two arms of a loop with weather nobody
 * asked about.
 *
 * The nearest approach is onto the polyline, not onto the nearest vertex. These
 * routes carry long straight segments, and a vertex-only answer is wrong by up
 * to half of one — enough to bend the corridor away from the road it is meant
 * to hug.
 *
 * How wide the corridor is comes from the forecast's own grid, so the drawing
 * cannot claim more precision than the model has: `forecastResolution.ts` says
 * how big a cell is, and a coarse far-out forecast is drawn broad and vague
 * where a sharp one is drawn tight.
 *
 * Wind is interpolated as a vector, never as a bearing. Averaging 350° and 10°
 * arithmetically gives 180° — arrows pointing exactly backwards — so directions
 * go through east/north components, the same treatment `wind.ts` gives its
 * window of bearings.
 *
 * Pure: no DOM, no fetching, no colour. `distances` is taken from the caller —
 * it already has `cumulativeMetres(coordinates)` from `profile.ts` — rather
 * than walking the geometry a second time here.
 */

import type { Position } from "../api/types";
import { haversineMetres } from "./profile";

/**
 * One degree of latitude on the same spherical model `haversineMetres` uses, so
 * an offset measured here is in the units the route's own distances are in.
 */
const METRES_PER_DEGREE_LATITUDE = haversineMetres([0, 0], [0, 1]);

/**
 * Where a point on the ground meets the route: how far along the route its
 * nearest approach is, and how far off the route the point itself lies.
 */
export interface RouteProjection {
  alongMetres: number;
  offsetMetres: number;
}

/** A point in metres east and north of wherever the caller put the origin. */
interface PlanarPoint {
  east: number;
  north: number;
}

/**
 * A coordinate as metres from `origin`, flat-earth around that latitude.
 *
 * Good to a fraction of a percent over the few kilometres a corridor spans,
 * which is well inside the grid cell the reading came from anyway.
 */
function planar(origin: Position, point: Position, longitudeScale: number): PlanarPoint {
  return {
    east: (point[0] - origin[0]) * METRES_PER_DEGREE_LATITUDE * longitudeScale,
    north: (point[1] - origin[1]) * METRES_PER_DEGREE_LATITUDE,
  };
}

/**
 * The point's nearest approach to the route, or null when there is no route to
 * approach — fewer than two coordinates, a `distances` array that does not
 * match them, or a point that is not a finite position.
 *
 * Perpendicular onto each segment rather than to the nearest vertex, and
 * clamped to the endpoints beyond either end: a point past the finish reads as
 * the finish, at its true distance away.
 *
 * `distances` is `cumulativeMetres(coordinates)`, supplied by the caller rather
 * than recomputed here.
 */
export function projectToRoute(
  coordinates: Position[],
  distances: number[],
  point: Position,
): RouteProjection | null {
  if (coordinates.length < 2 || coordinates.length !== distances.length) {
    return null;
  }
  if (!Number.isFinite(point[0]) || !Number.isFinite(point[1])) {
    return null;
  }
  const longitudeScale = Math.cos((point[1] * Math.PI) / 180);

  let bestSquared = Number.POSITIVE_INFINITY;
  let alongMetres = 0;
  for (let index = 0; index < coordinates.length - 1; index++) {
    const from = coordinates[index];
    const to = coordinates[index + 1];
    if (!from || !to) {
      continue;
    }
    // Relative to the query point, so the point itself sits at the origin and
    // the nearest approach is the shortest vector from it to the segment.
    const start = planar(point, from, longitudeScale);
    const end = planar(point, to, longitudeScale);
    const runEast = end.east - start.east;
    const runNorth = end.north - start.north;
    const lengthSquared = runEast * runEast + runNorth * runNorth;
    const ratio =
      lengthSquared > 0
        ? Math.min(Math.max(-(start.east * runEast + start.north * runNorth) / lengthSquared, 0), 1)
        : 0;
    const east = start.east + ratio * runEast;
    const north = start.north + ratio * runNorth;
    const squared = east * east + north * north;
    if (squared < bestSquared) {
      bestSquared = squared;
      const startMetres = distances[index] ?? 0;
      alongMetres = startMetres + ratio * ((distances[index + 1] ?? startMetres) - startMetres);
    }
  }

  return Number.isFinite(bestSquared)
    ? { alongMetres, offsetMetres: Math.sqrt(bestSquared) }
    : null;
}

/**
 * How far either side of the route a reading is drawn at full strength, as a
 * share of the forecast's grid cell: 1500 m for ICON-D2's 2 km cell.
 *
 * Under a cell, because a reading is a point sample of that cell rather than a
 * measurement of all of it, and over-claiming is the thing the corridor exists
 * to avoid.
 */
export const CORRIDOR_CORE_CELLS = 0.75;

/** Where it has faded to nothing: 4000 m for that same 2 km cell. */
export const CORRIDOR_EDGE_CELLS = 2;

/** The full-strength and vanishing radii for one forecast grid size. */
export interface CorridorRadii {
  coreMetres: number;
  edgeMetres: number;
}

/**
 * How wide the corridor is for a forecast resolved at `metresPerCell`.
 *
 * Both radii come from the cell, which is what makes a three-day-out forecast
 * read as visibly broader and vaguer than tomorrow morning's.
 */
export function corridorRadii(metresPerCell: number): CorridorRadii {
  const cell = Number.isFinite(metresPerCell) ? Math.max(metresPerCell, 0) : 0;

  return { coreMetres: cell * CORRIDOR_CORE_CELLS, edgeMetres: cell * CORRIDOR_EDGE_CELLS };
}

/**
 * How much of the route's reading still applies `offsetMetres` off it, from 1
 * on the road itself to 0 at the corridor's edge and beyond.
 *
 * Smoothstepped between the two radii rather than cut off: a hard boundary
 * draws a line the weather does not have, and the eye reads that line as a
 * front.
 */
export function corridorWeight(offsetMetres: number, metresPerCell: number): number {
  const { coreMetres, edgeMetres } = corridorRadii(metresPerCell);
  const offset = Math.abs(offsetMetres);
  if (!Number.isFinite(offset) || offset >= edgeMetres) {
    return 0;
  }
  if (offset <= coreMetres) {
    return 1;
  }
  const fade = (offset - coreMetres) / (edgeMetres - coreMetres);

  return 1 - fade * fade * (3 - 2 * fade);
}

/** One reading of a single number, at its distance along the route. */
export interface ScalarSample {
  distanceMetres: number;
  value: number;
}

/** Which two samples a distance falls between, and how far between them it is. */
interface Bracket<T> {
  from: T;
  to: T;
  ratio: number;
}

/**
 * The two samples bracketing `alongMetres`, assuming ascending distances — the
 * order `forecastSamples` produces them in.
 *
 * Outside their range both ends are the nearest sample, which is what clamps
 * every interpolation here: the forecast says nothing about ground it has no
 * sample for, and the nearest reading is the honest stand-in for the metres
 * between the first sample and the start line.
 */
function bracket<T extends { distanceMetres: number }>(
  samples: T[],
  alongMetres: number,
): Bracket<T> | null {
  if (samples.length === 0 || !Number.isFinite(alongMetres)) {
    return null;
  }
  const first = samples[0] as T;
  const last = samples[samples.length - 1] as T;
  if (alongMetres <= first.distanceMetres) {
    return { from: first, to: first, ratio: 0 };
  }
  if (alongMetres >= last.distanceMetres) {
    return { from: last, to: last, ratio: 0 };
  }

  let index = 0;
  while (index < samples.length - 2 && (samples[index + 1]?.distanceMetres ?? 0) <= alongMetres) {
    index++;
  }
  const from = samples[index] as T;
  const to = samples[index + 1] as T;
  const span = to.distanceMetres - from.distanceMetres;

  return { from, to, ratio: span > 0 ? (alongMetres - from.distanceMetres) / span : 0 };
}

/**
 * A scalar reading — temperature, rain, cloud — at one distance along the
 * route, interpolated between the samples either side of it and clamped to the
 * nearest sample outside their range.
 *
 * Null when there is no sample to read, rather than a plausible zero.
 */
export function sampleScalarAt(samples: ScalarSample[], alongMetres: number): number | null {
  const found = bracket(samples, alongMetres);

  return found ? found.from.value + found.ratio * (found.to.value - found.from.value) : null;
}

/**
 * A wind reading. `directionDegrees` follows the meteorological convention
 * `WeatherPoint.windDirectionDegrees` uses: the direction the wind blows
 * *from*, 0° north, clockwise.
 */
export interface WindVector {
  speedKmh: number;
  directionDegrees: number;
}

/** One wind reading, at its distance along the route. */
export interface WindSample extends WindVector {
  distanceMetres: number;
}

/**
 * A wind as east/north components, the form it can be averaged in.
 *
 * They point back along the direction the wind comes from, keeping the
 * meteorological convention rather than flipping it halfway through.
 */
export interface WindComponents {
  eastwardKmh: number;
  northwardKmh: number;
}

/** A wind reading split into components. */
export function windComponents(vector: WindVector): WindComponents {
  const radians = (vector.directionDegrees * Math.PI) / 180;

  return {
    eastwardKmh: vector.speedKmh * Math.sin(radians),
    northwardKmh: vector.speedKmh * Math.cos(radians),
  };
}

/**
 * Components back into a speed and a direction, the exact inverse of
 * `windComponents`. At a speed of nought the direction is arbitrary rather than
 * meaningful, and reads as 0°.
 */
export function windVector(components: WindComponents): WindVector {
  const { eastwardKmh, northwardKmh } = components;
  const degrees = (Math.atan2(eastwardKmh, northwardKmh) * 180) / Math.PI;

  return {
    speedKmh: Math.sqrt(eastwardKmh * eastwardKmh + northwardKmh * northwardKmh),
    directionDegrees: ((degrees % 360) + 360) % 360,
  };
}

/**
 * The wind at one distance along the route, interpolated as a vector and
 * clamped to the nearest sample outside the samples' range. Null when there is
 * nothing to read.
 *
 * Through components, never as a bearing: 350° and 10° average arithmetically
 * to 180°, which is a wind blowing the opposite way. Two opposing winds cancel
 * to a near-nought speed here instead, which is the honest reading of a front
 * passing between two samples.
 */
export function sampleVectorAt(samples: WindSample[], alongMetres: number): WindVector | null {
  const found = bracket(samples, alongMetres);
  if (!found) {
    return null;
  }
  const from = windComponents(found.from);
  const to = windComponents(found.to);

  return windVector({
    eastwardKmh: from.eastwardKmh + found.ratio * (to.eastwardKmh - from.eastwardKmh),
    northwardKmh: from.northwardKmh + found.ratio * (to.northwardKmh - from.northwardKmh),
  });
}
