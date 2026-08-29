/**
 * One ride, described by both chosen alternatives.
 *
 * The two spikes were drawn against two fixtures — a hundred and thirty
 * kilometre loop for the panel, a two hundred and twenty kilometre one for the
 * forecast — because each was built to stress a different thing. Putting the
 * chosen pair in one frame against two different rides would show a
 * composition that cannot exist, so this takes the forecast's route, which is
 * the one that has weather, and measures the panel's figures off it.
 *
 * Measured, again, rather than asserted: the distance under the pill and the
 * lengths in the columns come from the same geometry the filmstrip lays its
 * tiles along, so the two panels cannot disagree about how long the day is.
 *
 * The ground is the one invention here. Nothing classifies the forecast
 * fixture's loop, so it is given a plausible six-class surface — the same
 * shape of table `panel-spike/fixture.ts` carries, over a longer route.
 */

import type { Position, Route, SurfaceKind } from "../../../api/types";
import { SURFACE_KINDS } from "../../../api/types";
import {
  START_AT,
  spikeCoordinates,
  spikeSamples,
} from "../../../components/route/forecast-spike/fixture";
import type { Climb } from "../../../lib/climbs";
import { findClimbs } from "../../../lib/climbs";
import type { BandShare } from "../../../lib/profile";
import { buildProfile, cumulativeMetres, gradientMix, gradientShares } from "../../../lib/profile";
import type { SurfaceBand, SurfaceShare, SurfaceSummary } from "../../../lib/surface";

export const rideCoordinates: Position[] = spikeCoordinates;

const distances = cumulativeMetres(rideCoordinates);
const totalMetres = distances[distances.length - 1] ?? 0;

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

export const rideProfile = buildProfile(rideCoordinates);

/** Two cols on tarmac, a gravel shoulder over the second, and a lane nobody has surveyed. */
const GROUND: ReadonlyArray<readonly [SurfaceKind, number]> = [
  ["asphalt", 0.21],
  ["compacted", 0.27],
  ["asphalt", 0.44],
  ["gravel", 0.52],
  ["ground", 0.565],
  ["asphalt", 0.68],
  ["unknown", 0.735],
  ["paving", 0.755],
  ["asphalt", 0.88],
  ["gravel", 0.925],
  ["asphalt", 1],
];

const bands: SurfaceBand[] = GROUND.map(([kind, until], index) => ({
  kind,
  startMetres: (GROUND[index - 1]?.[1] ?? 0) * totalMetres,
  endMetres: until * totalMetres,
}));

const shares: SurfaceShare[] = SURFACE_KINDS.flatMap((kind) => {
  const metres = bands
    .filter((band) => band.kind === kind)
    .reduce((sum, band) => sum + (band.endMetres - band.startMetres), 0);

  return metres > 0 ? [{ kind, metres, share: metres / totalMetres }] : [];
});

export const rideSurface: SurfaceSummary = { bands, shares, totalMetres };
export const rideBands: BandShare[] = gradientShares(rideCoordinates);
export const rideRuns: BandShare[] = gradientMix(rideCoordinates);
export const rideClimbs: Climb[] = findClimbs(rideCoordinates);

/**
 * How long the day takes, from the forecast's own arrival times.
 *
 * The samples are placed by riding the geometry at a speed that falls with the
 * gradient, so the last one's arrival is the moving time that produced the
 * weather — reading it back here is what keeps the panel's figure and the
 * filmstrip's axis describing the same ride.
 */
const lastArrival = spikeSamples[spikeSamples.length - 1]?.arrivalAt ?? START_AT;
const movingSeconds = Math.round((lastArrival.getTime() - START_AT.getTime()) / 1_000);

export const rideRoute: Route = {
  provider: "veloplanner",
  sourceRouteId: 208,
  stageOrder: 1,
  title: "Trois Cols — the long way round",
  sourceRouteName: "Trois Cols",
  routeName: "the long way round",
  sourceRevision: "2026-08-24",
  contentHash: "panel-spike-together",
  distanceMetres: totalMetres,
  ascentMetres: ascentOf(rideCoordinates),
  maxGradientPercent: rideProfile
    ? Math.max(...rideProfile.samples.map((sample) => sample.gradientPercent))
    : 0,
  pointCount: rideCoordinates.length,
  movingSeconds,
  validation: { biasPercent: -0.8, maePercent: 7.4, p90Percent: 15.2, evaluatedRides: 63 },
};

export const rideHighestMetres = rideProfile?.maxElevationMetres ?? 0;
