/**
 * One synthetic ride with the shape of the demo's longest: three climbs to
 * the same summit, and a fast valley between each. Spike-local and static —
 * what is being tried is how the page divides and weights what it holds, and
 * none of that needs the numbers to be live. Everything below is derived from
 * the one per-kilometre ascent list so the totals, splits, zones and profile
 * cannot disagree with each other.
 */

import type { ActivityMetrics, ActivitySplit, Position, RideWeatherStep } from "../../../api/types";
import { cumulativeMetres } from "../../../lib/profile";

export const TITLE = "6 Sept 2026, 10:00";

/** Metres climbed in each kilometre; nought where the road fell or lay flat. */
const ASCENT_PER_KM = [
  25, 64, 90, 116, 133, 155, 174, 137, 17, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 8, 45, 81, 103, 108, 107,
  111, 105, 106, 90, 44, 0, 0, 0, 0, 0, 0, 0, 0, 0, 4, 26, 48, 77, 108, 143, 179, 180, 113, 32,
];
const VALLEY_METRES = 180;
const KILOMETRES = ASCENT_PER_KM.length;
export const TOTAL_METRES = KILOMETRES * 1000;

/** Altitude at each kilometre mark: up by the list, and back down to the valley between climbs. */
function kilometreAltitudes(): number[] {
  const marks = [VALLEY_METRES];
  let index = 0;
  while (index < KILOMETRES) {
    const ascent = ASCENT_PER_KM[index] ?? 0;
    if (ascent > 0) {
      marks.push((marks[index] ?? VALLEY_METRES) + ascent);
      index += 1;
      continue;
    }
    let end = index;
    while (end < KILOMETRES && (ASCENT_PER_KM[end] ?? 0) === 0) {
      end += 1;
    }
    const top = marks[index] ?? VALLEY_METRES;
    for (let step = 1; step <= end - index; step += 1) {
      marks.push(top - ((top - VALLEY_METRES) * step) / (end - index));
    }
    index = end;
  }

  return marks;
}

const MARKS = kilometreAltitudes();

/** Altitude between the marks: one gradient per kilometre, which is the resolution the splits have. */
function altitudeAt(metres: number): number {
  const km = Math.min(Math.floor(metres / 1000), KILOMETRES - 1);
  const t = metres / 1000 - km;
  const from = MARKS[km] ?? VALLEY_METRES;
  const to = MARKS[km + 1] ?? from;

  return from + (to - from) * t;
}

/** A wobbly loop whose measured length is the ride's, with the altitudes laid along it. */
function loop(): Position[] {
  const count = 490;
  const shape = (scale: number): Position[] =>
    Array.from({ length: count + 1 }, (_, index) => {
      const theta = (index / count) * Math.PI * 2;
      const wobble = 1 + 0.08 * Math.sin(5 * theta) + 0.05 * Math.cos(9 * theta);
      return [
        7.7 + scale * 0.105 * wobble * Math.cos(theta),
        48.1 + scale * 0.07 * wobble * Math.sin(theta),
      ];
    });
  const unit = cumulativeMetres(shape(1));
  const measured = unit[unit.length - 1] ?? 1;
  const positions = shape(TOTAL_METRES / measured);
  const along = cumulativeMetres(positions);

  return positions.map(([longitude = 0, latitude = 0], index) => [
    longitude,
    latitude,
    altitudeAt(Math.min(along[index] ?? 0, TOTAL_METRES)),
  ]);
}

export const COORDINATES: Position[] = loop();

export const SPLITS: ActivitySplit[] = ASCENT_PER_KM.map((ascentMetres) => {
  const speedKmh = ascentMetres > 0 ? Math.max(4.8, 24 - 0.107 * ascentMetres) : 24;
  return {
    distanceMetres: 1000,
    movingSeconds: Math.round(3600 / speedKmh),
    ascentMetres,
    heartRateBpm: Math.min(172, 104 + ascentMetres * 0.42),
    powerWatts: Math.min(320, 115 + ascentMetres * 1.2),
  };
});

export const MOVING_SECONDS = SPLITS.reduce((sum, split) => sum + split.movingSeconds, 0);
export const ELAPSED_SECONDS = MOVING_SECONDS + 17 * 60;
export const ASCENT_METRES = ASCENT_PER_KM.reduce((sum, metres) => sum + metres, 0);
export const AVERAGE_KMH = ((TOTAL_METRES / MOVING_SECONDS) * 3.6).toFixed(1);

const ZONE_BOUNDS = [143, 151, 159, 167];

export const METRICS: ActivityMetrics = {
  zoneBoundsBpm: ZONE_BOUNDS,
  zoneSeconds: SPLITS.reduce(
    (zones, split) => {
      const zone = ZONE_BOUNDS.filter((bound) => (split.heartRateBpm ?? 0) >= bound).length;
      zones[zone] = (zones[zone] ?? 0) + split.movingSeconds;
      return zones;
    },
    [0, 0, 0, 0, 0],
  ),
  trimp: 130,
  heartRateTss: 73,
  powerTss: 118,
  normalizedPowerWatts: 232,
  intensityFactor: 0.88,
  averageHeartRateBpm: 143,
  maxHeartRateBpm: 172,
  averageCadenceRpm: 76,
  averagePowerWatts: 232,
};

export const WEATHER: RideWeatherStep[] = [
  [10, 17, 315, 2, 0, 40],
  [13, 20, 0, 2, 0, 50],
  [16, 21, 0, 3, 0, 70],
  [18, 20, 45, 3, 0, 80],
  [19, 17, 45, 61, 4.6, 90],
].map(([temperature = 0, wind = 0, direction = 0, code = 0, rain = 0, cloud = 0], hour) => ({
  time: `2026-09-06T${String(10 + hour).padStart(2, "0")}:00:00+02:00`,
  stepSeconds: 3600,
  temperatureCelsius: temperature,
  apparentTemperatureCelsius: temperature - 1,
  precipitationMillimetres: rain,
  windSpeedKmh: wind,
  windDirectionDegrees: direction,
  weatherCode: code,
  cloudCoverPercent: cloud,
}));

/** Where along the ride each weather step began, from the splits' own clock. */
export const STEP_METRES: number[] = WEATHER.map((_, step) => {
  let elapsed = 0;
  for (const [index, split] of SPLITS.entries()) {
    if (elapsed >= step * 3600) {
      return index * 1000;
    }
    elapsed += split.movingSeconds;
  }
  return TOTAL_METRES;
});
