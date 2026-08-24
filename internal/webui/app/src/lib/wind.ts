/**
 * Turning a forecast's wind reading into what a cyclist actually wants to
 * know: is it pushing me along, or into my face?
 *
 * A bare "18 km/h from 240°" makes a rider do trigonometry at the roadside.
 * The answer that matters is the wind's component along the direction of
 * travel, which needs a direction of travel to project against — and that
 * direction is measured over a window rather than between two adjacent
 * coordinates, for the same reason `profile.ts` windows its gradients: source
 * points can sit metres apart, and a bearing across two of them is mostly
 * noise from GPS jitter rather than a fact about the road.
 *
 * The window can also disagree with itself. A switchback is honestly heading
 * two directions within a few hundred metres, and averaging that into one
 * confident bearing would report a crosswind that is not what either leg of
 * the hairpin actually feels. So the window's spread is measured too, and a
 * window that does not agree on a direction is reported as `"mixed"` rather
 * than smoothed into a number that looks more certain than the ground is.
 *
 * Pure: no DOM, no fetching. `distances` is taken from the caller — it
 * already has `cumulativeMetres(coordinates)` from `profile.ts` — rather than
 * walking the geometry a second time here.
 */

import type { Position } from "../api/types";

/**
 * How much of the route either side of a point its bearing is measured over.
 *
 * Wide enough that a couple of GPS-jittery points do not flip the reported
 * direction, narrow enough that the bearing still describes the road under
 * the forecast point rather than the shape of the whole stage.
 */
export const BEARING_WINDOW_METRES = 600;

/**
 * How far a window's sub-bearings may disagree, in the same units
 * `spreadDegrees` reports, before the window is read as `"mixed"` rather than
 * as one direction.
 *
 * A smooth bend can easily swing sub-bearings 60–70° apart across a window
 * this wide without the road actually turning back on itself; a genuine
 * switchback cancels its own sub-bearings toward zero and reads well past 90.
 * The calibration knob for how forgiving "still one direction" is.
 */
export const MIXED_SPREAD_THRESHOLD_DEGREES = 90;

/** Wraps a bearing into [0, 360). */
function normaliseDegrees(degrees: number): number {
  return ((degrees % 360) + 360) % 360;
}

/**
 * The initial bearing from one coordinate to the next, on the same spherical
 * model `haversineMetres` uses — 0° north, clockwise.
 */
function bearingBetween(from: Position, to: Position): number {
  const [fromLongitude, fromLatitude] = from;
  const [toLongitude, toLatitude] = to;
  const fromLatitudeRadians = (fromLatitude * Math.PI) / 180;
  const toLatitudeRadians = (toLatitude * Math.PI) / 180;
  const deltaLongitudeRadians = ((toLongitude - fromLongitude) * Math.PI) / 180;

  const y = Math.sin(deltaLongitudeRadians) * Math.cos(toLatitudeRadians);
  const x =
    Math.cos(fromLatitudeRadians) * Math.sin(toLatitudeRadians) -
    Math.sin(fromLatitudeRadians) * Math.cos(toLatitudeRadians) * Math.cos(deltaLongitudeRadians);

  return normaliseDegrees((Math.atan2(y, x) * 180) / Math.PI);
}

/** The coordinate indices whose segments fall inside a window, rounded outwards. */
function windowRange(
  distances: number[],
  atMetres: number,
  windowMetres: number,
): { startIndex: number; endIndex: number } | null {
  // Only ever called behind bearingsInWindow's own length check, so there are
  // at least two points here and a last index of at least one.
  const lastIndex = distances.length - 1;
  const total = distances[lastIndex] ?? 0;
  const half = windowMetres / 2;
  const from = Math.min(Math.max(atMetres - half, 0), total);
  const to = Math.min(Math.max(atMetres + half, 0), total);

  let startIndex = 0;
  while (startIndex < lastIndex && (distances[startIndex + 1] ?? 0) <= from) {
    startIndex++;
  }
  let endIndex = lastIndex;
  while (endIndex > startIndex && (distances[endIndex - 1] ?? 0) >= to) {
    endIndex--;
  }
  // A window that collapsed to a single point — the very start or end of the
  // route, or a window narrower than one segment — is widened by one segment
  // rather than left with nothing to take a bearing from.
  if (startIndex === endIndex) {
    if (endIndex < lastIndex) {
      endIndex++;
    } else if (startIndex > 0) {
      startIndex--;
    }
  }

  return startIndex === endIndex ? null : { startIndex, endIndex };
}

interface WindowBearing {
  meanBearingDegrees: number;
  /**
   * 0 when every segment in the window points the same way, rising toward 180
   * as they cancel each other out — the measure `bearingIsMixed` thresholds.
   */
  spreadDegrees: number;
}

/**
 * The window's segments, reduced to a length-weighted mean bearing and how
 * much they disagree about it.
 *
 * Weighted by segment length rather than by point count, so that resampling
 * the same road at twice the point density — the same segments, cut in
 * half — changes neither the mean nor the spread: two collinear half-segments
 * contribute the same vector sum as the one segment they replaced.
 */
function bearingsInWindow(
  coordinates: Position[],
  distances: number[],
  atMetres: number,
  windowMetres: number,
): WindowBearing | null {
  if (coordinates.length !== distances.length || coordinates.length < 2) {
    return null;
  }
  const range = windowRange(distances, atMetres, windowMetres);
  if (!range) {
    return null;
  }

  let sumSin = 0;
  let sumCos = 0;
  let totalWeight = 0;
  for (let index = range.startIndex; index < range.endIndex; index++) {
    const from = coordinates[index];
    const to = coordinates[index + 1];
    const weight = (distances[index + 1] ?? 0) - (distances[index] ?? 0);
    if (!from || !to || weight <= 0) {
      continue;
    }
    const radians = (bearingBetween(from, to) * Math.PI) / 180;
    sumSin += weight * Math.sin(radians);
    sumCos += weight * Math.cos(radians);
    totalWeight += weight;
  }
  if (totalWeight <= 0) {
    return null;
  }

  const meanBearingDegrees = normaliseDegrees((Math.atan2(sumSin, sumCos) * 180) / Math.PI);
  // The resultant vector's length relative to the weight that built it: 1 when
  // every segment agreed, falling toward 0 as they point in different, even
  // opposing, directions. Circular statistics' usual measure of concentration.
  const resultantLength = Math.sqrt(sumSin * sumSin + sumCos * sumCos) / totalWeight;

  return { meanBearingDegrees, spreadDegrees: (1 - resultantLength) * 180 };
}

/**
 * The direction of travel at `atMetres`, averaged over `windowMetres` of
 * route centred on it, or null when there is no ground to measure — fewer
 * than two coordinates, or a window with no segment of positive length inside
 * it (coincident points at the very start or end of the route).
 *
 * `distances` is `cumulativeMetres(coordinates)`, supplied by the caller
 * rather than recomputed here.
 */
export function bearingAt(
  coordinates: Position[],
  distances: number[],
  atMetres: number,
  windowMetres: number,
): number | null {
  return (
    bearingsInWindow(coordinates, distances, atMetres, windowMetres)?.meanBearingDegrees ?? null
  );
}

/**
 * Whether the window behind a bearing disagreed with itself past
 * `thresholdDegrees` — the switchback case `bearingAt` alone cannot report,
 * because it only ever hands back one confident-looking number.
 *
 * False for a window `bearingAt` itself could not measure, which is the same
 * "nothing to say" answer as a null bearing: a stretch with no direction is
 * not a stretch with a disputed one.
 */
export function bearingIsMixed(
  coordinates: Position[],
  distances: number[],
  atMetres: number,
  windowMetres: number,
  thresholdDegrees: number = MIXED_SPREAD_THRESHOLD_DEGREES,
): boolean {
  const window = bearingsInWindow(coordinates, distances, atMetres, windowMetres);

  return window !== null && window.spreadDegrees > thresholdDegrees;
}

export const WIND_RELATIONS = ["head", "tail", "cross"] as const;
export type WindRelation = (typeof WIND_RELATIONS)[number];

/** How a reported wind sits against the direction of travel. */
export interface WindReading {
  relation: WindRelation;
  /**
   * The wind's component along the direction of travel, as a fraction of its
   * reported speed: multiply by `windSpeedKmh` for the along-travel speed in
   * km/h. Positive pushes the rider forward (a tailwind component), negative
   * pushes back (a headwind component) — signed so a `"cross"` reading still
   * carries whichever way it leans rather than reading as exactly zero.
   */
  componentKmhPerKmh: number;
}

/**
 * Within this many degrees of dead ahead or dead behind, the wind counts as a
 * head- or tailwind rather than a crosswind. Symmetric, so head and tail each
 * claim a 90° cone and the remaining 180° — 90° either side — reads as cross.
 */
const HEAD_TAIL_CONE_DEGREES = 45;

/**
 * Where a wind blowing from `windFromDegrees` sits against a rider heading
 * `bearingDegrees`, both compass bearings, 0° north.
 *
 * `windFromDegrees` is meteorological convention — the direction the wind
 * blows *from* — matching `WeatherPoint.windDirectionDegrees`. A wind from
 * directly ahead of the rider's own heading is a headwind: it is blowing
 * toward them.
 */
export function windRelation(bearingDegrees: number, windFromDegrees: number): WindReading {
  const angleDifference = normaliseDegrees(windFromDegrees - bearingDegrees);
  // Folded to [0, 180]: how far the wind's source sits from "dead ahead",
  // without caring which side.
  const foldedDifference = angleDifference > 180 ? 360 - angleDifference : angleDifference;

  const relation: WindRelation =
    foldedDifference <= HEAD_TAIL_CONE_DEGREES
      ? "head"
      : foldedDifference >= 180 - HEAD_TAIL_CONE_DEGREES
        ? "tail"
        : "cross";

  // A wind from dead ahead (foldedDifference 0) is a full headwind — negative,
  // since it opposes the ride — and a wind from dead behind (180) is a full
  // tailwind. cos(0)=1, cos(180)=-1, so negating the cosine gives exactly that.
  const componentKmhPerKmh = -Math.cos((angleDifference * Math.PI) / 180);

  return { relation, componentKmhPerKmh };
}
