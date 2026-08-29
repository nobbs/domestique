/**
 * A route built to tell the four cards apart.
 *
 * The shared story fixture is forty points climbing at a constant rate along a
 * straight line, with two surfaces split down the middle. Every encoding in
 * this spike looks identical on it — one gradient band, two ground classes, no
 * order worth preserving — which is the one thing a comparison must not do.
 *
 * So: a hundred and thirty kilometre loop over three cols, rippled so the
 * steepness comes out as ramps rather than as one block per leg, and six
 * classes of ground including the unsurveyed stretch every real route has.
 *
 * Every figure the cards print is *measured off this geometry* rather than
 * asserted beside it — distance, ascent, steepest hundred metres, the climbs,
 * both mixes. A card therefore cannot quietly disagree with its own ribbon,
 * and a layout that looks good here is not looking good on numbers chosen to
 * flatter it.
 */

import type { Position, Route, SurfaceKind } from "../../../api/types";
import { SURFACE_KINDS } from "../../../api/types";
import type { Climb } from "../../../lib/climbs";
import { findClimbs } from "../../../lib/climbs";
import type { BandShare } from "../../../lib/profile";
import { buildProfile, cumulativeMetres, gradientMix, gradientShares } from "../../../lib/profile";
import type { SurfaceBand, SurfaceShare, SurfaceSummary } from "../../../lib/surface";

/** Somewhere in the western Alps, so the loop sits on plausible ground. */
const START: readonly [number, number] = [7.42, 46.31];

const TARGET_METRES = 130_000;
const POINT_COUNT = 2_001;

/**
 * Height above sea level at each turning point, by distance in kilometres.
 *
 * Three cols with a valley between each. The last keyframe returns to the
 * first: the route is a loop, and a loop that ended two hundred metres above
 * where it started would put that error into the ascent figure every card
 * prints.
 */
const ELEVATION_KEYFRAMES: ReadonlyArray<readonly [number, number]> = [
  [0, 420],
  [18, 465],
  [29.5, 1_115],
  [31, 1_310],
  [42, 640],
  [58, 690],
  [74, 1_495],
  [90, 610],
  [110, 700],
  [120, 1_210],
  [130, 420],
];

/**
 * The road's own unevenness, in metres, at a distance in kilometres.
 *
 * Three wavelengths: the long roll of a valley floor, the pitch between
 * hairpins, and the short ramps inside a pitch. Without it every leg is one
 * constant gradient, which classifies as a single band and makes the steepness
 * mix a picture of this function rather than of a road.
 *
 * Deliberately shallower than the gentlest col's own gradient. Ripple deep
 * enough to turn a trough into a descent splits one col into eight climbs, and
 * the climbs line every card prints would then be counting this function's
 * wavelengths rather than the road's cols.
 */
function ripple(km: number): number {
  return (
    2.1 * Math.sin((2 * Math.PI * km) / 1.4) +
    0.85 * Math.sin((2 * Math.PI * km) / 0.42 + 1.1) +
    0.2 * Math.sin((2 * Math.PI * km) / 0.17 + 2.3)
  );
}

/** Base height at a distance in kilometres, straight between the keyframes. */
function baseElevation(km: number): { metres: number; gradientPercent: number } {
  const last = ELEVATION_KEYFRAMES[ELEVATION_KEYFRAMES.length - 1] ?? [0, 0];
  for (let index = 1; index < ELEVATION_KEYFRAMES.length; index++) {
    const from = ELEVATION_KEYFRAMES[index - 1];
    const to = ELEVATION_KEYFRAMES[index];
    if (!from || !to || km > to[0]) {
      continue;
    }
    const span = to[0] - from[0];
    const fraction = span > 0 ? (km - from[0]) / span : 0;

    return {
      metres: from[1] + (to[1] - from[1]) * fraction,
      gradientPercent: span > 0 ? (to[1] - from[1]) / (span * 10) : 0,
    };
  }

  return { metres: last[1], gradientPercent: 0 };
}

/**
 * Height at a distance in kilometres.
 *
 * The ripple is scaled by how hard the leg already is. Valley floors are
 * smooth and cols are ramped, and a flat eighteen kilometres carrying the same
 * unevenness as a hairpin would fill the gentlest band with steepness the road
 * does not have.
 */
function elevationAt(km: number): number {
  const { metres, gradientPercent } = baseElevation(km);
  const weight = 0.25 + 0.75 * Math.min(Math.abs(gradientPercent) / 6, 1);

  return metres + ripple(km) * weight;
}

/** A wobbling closed ring, so the shape reads as a road rather than as a circle. */
function ringAt(fraction: number, radiusLon: number, radiusLat: number): [number, number] {
  const angle = 2 * Math.PI * fraction;
  const wobble = 1 + 0.16 * Math.sin(6 * angle) + 0.09 * Math.cos(9 * angle);

  return [
    START[0] + radiusLon * Math.cos(angle) * wobble,
    START[1] + radiusLat * Math.sin(angle) * wobble,
  ];
}

function ringOf(radiusLat: number): Array<[number, number]> {
  const radiusLon = radiusLat / Math.cos((START[1] * Math.PI) / 180);

  return Array.from({ length: POINT_COUNT }, (_, index) =>
    ringAt(index / (POINT_COUNT - 1), radiusLon, radiusLat),
  );
}

/**
 * The ring at the radius that measures out to the length asked for.
 *
 * Solved rather than derived: the wobble makes the perimeter something other
 * than a circle's, and the distance the cards print has to be the distance the
 * geometry actually is. Three passes converge well inside a hundred metres.
 */
function scaledRing(): { ring: Array<[number, number]>; distances: number[] } {
  let radius = 0.18;
  let ring = ringOf(radius);
  let distances = cumulativeMetres(ring);

  for (let pass = 0; pass < 3; pass++) {
    const measured = distances[distances.length - 1] ?? TARGET_METRES;
    radius *= TARGET_METRES / measured;
    ring = ringOf(radius);
    distances = cumulativeMetres(ring);
  }

  return { ring, distances };
}

const { ring, distances } = scaledRing();

export const spikeCoordinates: Position[] = ring.map(([longitude, latitude], index) => [
  longitude,
  latitude,
  elevationAt((distances[index] ?? 0) / 1_000),
]);

const totalMetres = distances[distances.length - 1] ?? 0;

/** Measured, not asserted: what the geometry above actually climbs. */
function ascentOf(coordinates: Position[]): number {
  let gained = 0;
  for (let index = 1; index < coordinates.length; index++) {
    const rise = (coordinates[index]?.[2] ?? 0) - (coordinates[index - 1]?.[2] ?? 0);
    if (rise > 0) {
      gained += rise;
    }
  }

  return gained;
}

export const spikeProfile = buildProfile(spikeCoordinates);

const steepest = spikeProfile
  ? Math.max(...spikeProfile.samples.map((sample) => sample.gradientPercent))
  : 0;

/**
 * Where each class of ground starts and stops, as fractions of the route.
 *
 * Fractions rather than metres so the table stays right whatever the ring
 * solves to. Contiguous and covering the whole loop, because a summary with
 * gaps in it would let a ribbon draw holes the classifier never reported.
 */
const GROUND: ReadonlyArray<readonly [SurfaceKind, number]> = [
  ["asphalt", 0.24],
  ["paving", 0.262],
  ["asphalt", 0.34],
  ["compacted", 0.41],
  ["gravel", 0.455],
  ["asphalt", 0.52],
  ["gravel", 0.578],
  ["ground", 0.63],
  ["unknown", 0.695],
  ["asphalt", 0.8],
  ["compacted", 0.845],
  ["asphalt", 1],
];

const surfaceBands: SurfaceBand[] = GROUND.map(([kind, until], index) => ({
  kind,
  startMetres: (GROUND[index - 1]?.[1] ?? 0) * totalMetres,
  endMetres: until * totalMetres,
}));

const surfaceShares: SurfaceShare[] = SURFACE_KINDS.flatMap((kind) => {
  const metres = surfaceBands
    .filter((band) => band.kind === kind)
    .reduce((sum, band) => sum + (band.endMetres - band.startMetres), 0);

  return metres > 0 ? [{ kind, metres, share: metres / totalMetres }] : [];
});

export const spikeSurface: SurfaceSummary = {
  bands: surfaceBands,
  shares: surfaceShares,
  totalMetres,
};

export const spikeBands: BandShare[] = gradientShares(spikeCoordinates);
/** The same steepness in ride order rather than totalled, for the positional ribbons. */
export const spikeRuns: BandShare[] = gradientMix(spikeCoordinates);
export const spikeClimbs: Climb[] = findClimbs(spikeCoordinates);

export const spikeRoute: Route = {
  provider: "veloplanner",
  sourceRouteId: 208,
  stageOrder: 1,
  title: "Trois Cols — the long way round",
  sourceRouteName: "Trois Cols",
  routeName: "the long way round",
  sourceRevision: "2026-08-24",
  contentHash: "panel-spike",
  distanceMetres: totalMetres,
  ascentMetres: ascentOf(spikeCoordinates),
  maxGradientPercent: steepest,
  pointCount: spikeCoordinates.length,
  // Around twenty-two an hour over this much climbing, which is what the
  // service's own model lands on for a route this shape.
  movingSeconds: 21_240,
  validation: { biasPercent: -0.8, maePercent: 7.4, p90Percent: 15.2, evaluatedRides: 63 },
};

export const spikeHighestMetres = spikeProfile?.maxElevationMetres ?? 0;
export const spikeSubtitle = "Haute-Savoie · read 19:38";
