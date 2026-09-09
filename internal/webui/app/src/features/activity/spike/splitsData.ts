/**
 * Three ride shapes for the "By the kilometre" spike (#646): a flat ride
 * matched to a route with no sustained climb, a hilly ride matched to a
 * route whose climbs carry the rider's own attempt history, and a rolling
 * ride the service could not match to any route.
 *
 * Spike-local and static. Each ride is derived from one per-kilometre ascent
 * list, so its splits, its terrain cuts and its climbs cannot disagree with
 * each other — the same discipline `data.ts` (the activity-page spike) uses.
 */

import type { ActivityMetrics, ActivitySplit, Position, RouteClimb } from "../../../api/types";
import { cumulativeMetres } from "../../../lib/profile";

/** Metres climbed in each kilometre of a flat ride: rollers, never a sustained grade. */
const ASCENT_FLAT = [2, 0, 4, 0, 0, 3, 5, 0, 0, 2, 4, 0, 0, 0, 3, 0, 4, 0, 0, 0];
/** Three climbs to one summit and back, the activity-page spike's own shape. */
const ASCENT_HILLY = [
  25, 64, 90, 116, 133, 155, 174, 137, 17, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 8, 45, 81, 103, 108, 107,
  111, 105, 106, 90, 44, 0, 0, 0, 0, 0, 0, 0, 0, 0, 4, 26, 48, 77, 108, 143, 179, 180, 113, 32,
];
/** Two short punchy ramps, neither long enough to be a sustained climb. */
const ASCENT_UNMATCHED = [0, 0, 38, 52, 12, 0, 0, 0, 0, 46, 58, 41, 0, 0, 0];

/** The three sustained climbs in `ASCENT_HILLY`, as whole-kilometre ranges. */
const HILLY_CLIMB_RANGES: ReadonlyArray<readonly [number, number]> = [
  [0, 9],
  [19, 30],
  [39, 49],
];

const FLAT_ACTIVITY_ID = 9001;
const HILLY_ACTIVITY_ID = 9002;
const UNMATCHED_ACTIVITY_ID = 9003;

export interface SplitsRideFixture {
  key: string;
  label: string;
  activityId: number;
  riddenAt: string;
  splits: ActivitySplit[];
  coordinates: Position[];
  totalMetres: number;
  metrics: ActivityMetrics;
  /** True once the service has matched this ride to a route at all. */
  routeMatched: boolean;
  /** The route's own climbs, with this ride's attempt among each one's history. Undefined when unmatched. */
  climbs?: RouteClimb[];
}

/** Altitude at every kilometre mark: up by the list, back down to the valley between climbs. */
function kilometreMarks(ascentPerKm: number[], valleyMetres: number): number[] {
  const marks = [valleyMetres];
  const km = ascentPerKm.length;
  let index = 0;
  while (index < km) {
    const ascent = ascentPerKm[index] ?? 0;
    if (ascent > 0) {
      marks.push((marks[index] ?? valleyMetres) + ascent);
      index += 1;
      continue;
    }
    let end = index;
    while (end < km && (ascentPerKm[end] ?? 0) === 0) {
      end += 1;
    }
    const top = marks[index] ?? valleyMetres;
    for (let step = 1; step <= end - index; step += 1) {
      marks.push(top - ((top - valleyMetres) * step) / (end - index));
    }
    index = end;
  }

  return marks;
}

function altitudeAt(marks: number[], km: number, valleyMetres: number, metres: number): number {
  const index = Math.min(Math.floor(metres / 1000), km - 1);
  const t = metres / 1000 - index;
  const from = marks[index] ?? valleyMetres;
  const to = marks[index + 1] ?? from;

  return from + (to - from) * t;
}

/** A wobbly loop of the ride's own measured length, its altitudes laid along it. */
function loopFor(
  ascentPerKm: number[],
  valleyMetres: number,
  center: readonly [number, number],
): Position[] {
  const km = ascentPerKm.length;
  const totalMetres = km * 1000;
  const marks = kilometreMarks(ascentPerKm, valleyMetres);
  const count = km * 10;
  const shape = (scale: number): Position[] =>
    Array.from({ length: count + 1 }, (_, index) => {
      const theta = (index / count) * Math.PI * 2;
      const wobble = 1 + 0.07 * Math.sin(6 * theta) + 0.04 * Math.cos(11 * theta);
      return [
        center[0] + scale * 0.09 * wobble * Math.cos(theta),
        center[1] + scale * 0.06 * wobble * Math.sin(theta),
      ];
    });
  const unit = cumulativeMetres(shape(1));
  const measured = unit[unit.length - 1] ?? 1;
  const positions = shape(totalMetres / measured);
  const along = cumulativeMetres(positions);

  return positions.map(([longitude = 0, latitude = 0], index) => [
    longitude,
    latitude,
    altitudeAt(marks, km, valleyMetres, Math.min(along[index] ?? 0, totalMetres)),
  ]);
}

/** Speed, heart rate and power for one stretch, from the ascent it climbed per kilometre it covers. */
export function stretchStats(ascentMetres: number, distanceMetres: number) {
  const perKm = distanceMetres > 0 ? (ascentMetres / distanceMetres) * 1000 : 0;
  const speedKmh = perKm > 0 ? Math.max(4.8, 24 - 0.107 * perKm) : Math.min(46, 24 - 0.3 * perKm);
  const heartRateBpm = Math.min(174, 102 + Math.max(perKm, 0) * 0.42);
  const powerWatts = Math.min(330, 108 + Math.max(perKm, 0) * 1.2);

  return { speedKmh, heartRateBpm, powerWatts };
}

function splitsFor(ascentPerKm: number[]): ActivitySplit[] {
  return ascentPerKm.map((ascentMetres) => {
    const { speedKmh, heartRateBpm, powerWatts } = stretchStats(ascentMetres, 1000);

    return {
      distanceMetres: 1000,
      movingSeconds: Math.round(3600 / speedKmh),
      ascentMetres,
      heartRateBpm,
      powerWatts,
    };
  });
}

const ZONE_BOUNDS = [128, 142, 155, 166];

function metricsFor(splits: ActivitySplit[]): ActivityMetrics {
  const totalSeconds = splits.reduce((sum, split) => sum + split.movingSeconds, 0);
  const zoneSeconds = splits.reduce<number[]>(
    (zones, split) => {
      const zone = ZONE_BOUNDS.filter((bound) => (split.heartRateBpm ?? 0) >= bound).length;
      zones[zone] = (zones[zone] ?? 0) + split.movingSeconds;
      return zones;
    },
    [0, 0, 0, 0, 0],
  );
  // Speed-per-heartbeat, first half against second: a fading rider needs more
  // beats for the same speed, so the ratio falls.
  const ratio = (subset: ActivitySplit[]) => {
    const seconds = subset.reduce((sum, split) => sum + split.movingSeconds, 0);
    const distance = subset.reduce((sum, split) => sum + split.distanceMetres, 0);
    const speed = seconds > 0 ? (distance / seconds) * 3.6 : 0;
    const heartRate =
      subset.reduce((sum, split) => sum + (split.heartRateBpm ?? 0) * split.movingSeconds, 0) /
      (seconds || 1);

    return heartRate > 0 ? speed / heartRate : 0;
  };
  const half = Math.floor(splits.length / 2);
  const first = ratio(splits.slice(0, half));
  const second = ratio(splits.slice(half));
  const averageHeartRateBpm =
    splits.reduce((sum, split) => sum + (split.heartRateBpm ?? 0) * split.movingSeconds, 0) /
    (totalSeconds || 1);
  const averagePowerWatts =
    splits.reduce((sum, split) => sum + (split.powerWatts ?? 0) * split.movingSeconds, 0) /
    (totalSeconds || 1);

  return {
    zoneBoundsBpm: ZONE_BOUNDS,
    zoneSeconds,
    trimp: Math.round((totalSeconds / 60) * (averageHeartRateBpm / 150)),
    heartRateTss: Math.round((totalSeconds / 3600) * 68),
    powerTss: Math.round((totalSeconds / 3600) * 88),
    normalizedPowerWatts: Math.round(averagePowerWatts * 1.05),
    intensityFactor: 0.82,
    averageHeartRateBpm: Math.round(averageHeartRateBpm),
    maxHeartRateBpm: Math.round(Math.max(...splits.map((split) => split.heartRateBpm ?? 0))),
    averageCadenceRpm: 78,
    averagePowerWatts: Math.round(averagePowerWatts),
    decouplingPercent: first > 0 ? Math.round(((first - second) / first) * 1000) / 10 : 0,
  };
}

/** One climb attempt read off a stretch of this ride's own splits. */
function attemptFromSplits(
  splits: ActivitySplit[],
  startKm: number,
  endKm: number,
  activityId: number,
  riddenAt: string,
) {
  const slice = splits.slice(startKm, endKm);
  const seconds = slice.reduce((sum, split) => sum + split.movingSeconds, 0);
  const ascentMetres = slice.reduce((sum, split) => sum + split.ascentMetres, 0);
  const heartRateBpm =
    slice.reduce((sum, split) => sum + (split.heartRateBpm ?? 0), 0) / slice.length;
  const powerWatts = slice.reduce((sum, split) => sum + (split.powerWatts ?? 0), 0) / slice.length;

  return {
    activityId,
    riddenAt,
    seconds,
    vamMetresPerHour: Math.round((ascentMetres / seconds) * 3600),
    heartRateBpm: Math.round(heartRateBpm),
    powerWatts: Math.round(powerWatts),
  };
}

/** An earlier attempt at the same climb, scaled off this ride's own — faster or slower by `factor`. */
function historicalAttempt(
  base: ReturnType<typeof attemptFromSplits>,
  factor: number,
  activityId: number,
  riddenAt: string,
  onABikeWithNoMeter: boolean,
) {
  const seconds = Math.round(base.seconds * factor);
  const ascentMetres = (base.vamMetresPerHour * base.seconds) / 3600;
  const power = Math.round(base.powerWatts / factor);

  return {
    activityId,
    riddenAt,
    seconds,
    vamMetresPerHour: Math.round((ascentMetres / seconds) * 3600),
    heartRateBpm: Math.round(base.heartRateBpm / factor),
    ...(onABikeWithNoMeter ? { estimatedPowerWatts: power } : { powerWatts: power }),
  };
}

function climbFor(
  splits: ActivitySplit[],
  [startKm, endKm]: readonly [number, number],
  activityId: number,
  riddenAt: string,
): RouteClimb {
  const slice = splits.slice(startKm, endKm);
  const distanceMetres = slice.reduce((sum, split) => sum + split.distanceMetres, 0);
  const ascentMetres = slice.reduce((sum, split) => sum + split.ascentMetres, 0);
  const maxGradePercent = Math.max(
    ...slice.map((split) => (split.ascentMetres / split.distanceMetres) * 100),
  );
  const current = attemptFromSplits(splits, startKm, endKm, activityId, riddenAt);
  const attempts = [
    historicalAttempt(current, 0.93, activityId - 100, "2026-06-14T07:40:00+02:00", true),
    current,
    historicalAttempt(current, 1.12, activityId - 200, "2026-07-19T08:05:00+02:00", false),
  ].sort((a, b) => a.seconds - b.seconds);

  return {
    startMetres: startKm * 1000,
    endMetres: endKm * 1000,
    distanceMetres,
    ascentMetres,
    averageGradePercent: (ascentMetres / distanceMetres) * 100,
    maxGradePercent,
    attempts,
  };
}

function fixtureFor(
  key: string,
  label: string,
  activityId: number,
  riddenAt: string,
  ascentPerKm: number[],
  center: readonly [number, number],
  routeMatched: boolean,
  climbRanges: ReadonlyArray<readonly [number, number]>,
): SplitsRideFixture {
  const splits = splitsFor(ascentPerKm);

  return {
    key,
    label,
    activityId,
    riddenAt,
    splits,
    coordinates: loopFor(ascentPerKm, 420, center),
    totalMetres: ascentPerKm.length * 1000,
    metrics: metricsFor(splits),
    routeMatched,
    ...(routeMatched
      ? { climbs: climbRanges.map((range) => climbFor(splits, range, activityId, riddenAt)) }
      : {}),
  };
}

export const RIDE_FLAT = fixtureFor(
  "flat",
  "Flat ride, matched, no sustained climb",
  FLAT_ACTIVITY_ID,
  "2026-09-06T09:00:00+02:00",
  ASCENT_FLAT,
  [7.5, 48.05],
  true,
  [],
);

export const RIDE_HILLY = fixtureFor(
  "hilly",
  "Hilly ride, matched, three timed climbs",
  HILLY_ACTIVITY_ID,
  "2026-08-30T08:00:00+02:00",
  ASCENT_HILLY,
  [7.7, 48.1],
  true,
  HILLY_CLIMB_RANGES,
);

export const RIDE_UNMATCHED = fixtureFor(
  "unmatched",
  "Rolling ride, no route match",
  UNMATCHED_ACTIVITY_ID,
  "2026-09-02T07:30:00+02:00",
  ASCENT_UNMATCHED,
  [7.3, 47.95],
  false,
  [],
);

export const RIDES: SplitsRideFixture[] = [RIDE_FLAT, RIDE_HILLY, RIDE_UNMATCHED];
