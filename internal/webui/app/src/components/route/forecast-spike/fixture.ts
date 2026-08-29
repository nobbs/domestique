/**
 * A day long enough, and unsettled enough, to tell the layouts apart.
 *
 * The strip's own story runs on three samples over three kilometres, which
 * every one of these alternatives survives — three cells three hundred pixels
 * wide flatter all of them equally. What actually decides between them is a
 * full day: two dozen cells at forty pixels, uneven because a climb takes
 * half an hour to cover four kilometres and the descent after it takes half
 * an hour to cover eighteen, with weather that changes underneath.
 *
 * So this is a 220 km alpine loop ridden from eight in the morning, with a
 * front arriving over the second climb: dry and warming until early
 * afternoon, thunderstorms from three, clearing into a cold headwind home.
 * A rider looking at this forecast would move their start time, which is the
 * whole reason the strip exists.
 *
 * Thrown away with the rest of this directory. If a layout wins, the fixture
 * it won on is worth keeping.
 */

import type { WeatherPoint } from "../../../api/types";
import type { ForecastSample } from "../../../lib/forecastSamples";
import { cumulativeMetres } from "../../../lib/profile";

/** Where the loop is ridden, near enough that the latitude scaling is honest. */
const ORIGIN: [number, number] = [8.0, 46.5];
const METRES_PER_DEGREE_LATITUDE = 111_200;
const METRES_PER_DEGREE_LONGITUDE = METRES_PER_DEGREE_LATITUDE * Math.cos((46.5 * Math.PI) / 180);

/**
 * The loop, as four legs on the compass.
 *
 * Four headings rather than a wander, because the wind relation is the one
 * reading that depends on which way the road points: against a steady
 * westerly, a rider going north has it across them, going east has it behind,
 * and coming home west has it in their face for thirty-five kilometres. A
 * route that curved gently would show one relation for the whole day and
 * prove nothing about how a layout draws the difference.
 */
const LEGS: ReadonlyArray<{
  readonly east: number;
  readonly north: number;
  readonly metres: number;
  /** Height against progress along the leg, interpolated between the marks. */
  readonly climb: ReadonlyArray<readonly [fraction: number, elevation: number]>;
}> = [
  {
    east: 0,
    north: 1,
    metres: 60_000,
    climb: [
      [0, 420],
      [0.42, 560],
      [1, 1_980],
    ],
  },
  {
    east: 1,
    north: 0,
    metres: 55_000,
    climb: [
      [0, 1_980],
      [0.45, 640],
      [1, 700],
    ],
  },
  {
    east: 0,
    north: -1,
    metres: 60_000,
    climb: [
      [0, 700],
      [0.5, 880],
      [1, 1_760],
    ],
  },
  {
    east: -1,
    north: 0,
    metres: 45_000,
    climb: [
      [0, 1_760],
      [0.55, 470],
      [1, 420],
    ],
  },
];

/** Where a leg's climb has reached, at `fraction` of the way along it. */
function elevationAt(climb: ReadonlyArray<readonly [number, number]>, fraction: number): number {
  for (let mark = 1; mark < climb.length; mark++) {
    const from = climb[mark - 1];
    const to = climb[mark];
    if (!from || !to || fraction > to[0]) {
      continue;
    }
    const span = to[0] - from[0];

    return span > 0 ? from[1] + ((to[1] - from[1]) * (fraction - from[0])) / span : to[1];
  }

  return climb[climb.length - 1]?.[1] ?? 0;
}

const POINTS_PER_LEG = 120;
const START_ELEVATION = 420;

/** The loop's geometry, as the API would hand it over: longitude, latitude, height. */
export const spikeCoordinates: [number, number, number][] = (() => {
  const points: [number, number, number][] = [[ORIGIN[0], ORIGIN[1], START_ELEVATION]];
  let [longitude, latitude] = ORIGIN;

  for (const leg of LEGS) {
    for (let step = 1; step <= POINTS_PER_LEG; step++) {
      const along = leg.metres / POINTS_PER_LEG;
      longitude += (leg.east * along) / METRES_PER_DEGREE_LONGITUDE;
      latitude += (leg.north * along) / METRES_PER_DEGREE_LATITUDE;
      points.push([longitude, latitude, elevationAt(leg.climb, step / POINTS_PER_LEG)]);
    }
  }

  return points;
})();

/**
 * How fast the ride is going, given the gradient under it.
 *
 * The calibration knob, and the reason the cells come out uneven: samples are
 * half an hour of *moving time* apart, so a constant speed would space them
 * evenly along the road and every layout would get a comfortable grid to draw
 * on. Real ones do not. Roughly a fit rider on a loaded bike — 26 km/h on the
 * flat, walking pace on a 10% wall, capped on the way down where the road
 * rather than the legs decides.
 */
function speedKmh(gradientPercent: number): number {
  return Math.min(Math.max(24 - gradientPercent * 2.2, 7), 38);
}

const SAMPLE_INTERVAL_SECONDS = 1_800;
export const START_AT = new Date("2026-08-18T06:00:00Z");

/** The loop, sampled every half hour of riding, the way `forecastSamples` would. */
export const spikeSamples: ForecastSample[] = (() => {
  const distances = cumulativeMetres(spikeCoordinates);
  const samples: ForecastSample[] = [];
  let seconds = 0;
  let nextSampleAt = 0;

  for (let index = 0; index < spikeCoordinates.length; index++) {
    if (seconds >= nextSampleAt) {
      samples.push({
        position: spikeCoordinates[index] as [number, number, number],
        distanceMetres: distances[index] ?? 0,
        arrivalAt: new Date(START_AT.getTime() + seconds * 1000),
      });
      nextSampleAt += SAMPLE_INTERVAL_SECONDS;
    }

    const previous = spikeCoordinates[index];
    const next = spikeCoordinates[index + 1];
    if (!previous || !next) {
      continue;
    }
    const run = (distances[index + 1] ?? 0) - (distances[index] ?? 0);
    const rise = (next[2] ?? 0) - (previous[2] ?? 0);
    const gradient = run > 0 ? (rise / run) * 100 : 0;
    seconds += run / ((speedKmh(gradient) * 1000) / 3600);
  }

  return samples;
})();

/**
 * The front, as a handful of moments the weather is interpolated between.
 *
 * Written as keyframes rather than two dozen literal readings because that is
 * how it reads as a day: warming into the early afternoon, breaking over the
 * second climb, clearing cold behind it. The wind backs from a helpful
 * south-westerly into a westerly as the front passes, which is what turns the
 * last leg home into the headwind.
 */
const KEYFRAMES: ReadonlyArray<{
  readonly at: number;
  readonly temperature: number;
  readonly apparent: number;
  readonly probability: number;
  readonly millimetres: number;
  readonly windKmh: number;
  readonly windFrom: number;
  readonly code: number;
}> = [
  {
    at: 0,
    temperature: 11,
    apparent: 9,
    probability: 0,
    millimetres: 0,
    windKmh: 8,
    windFrom: 250,
    code: 0,
  },
  {
    at: 5,
    temperature: 17,
    apparent: 16,
    probability: 5,
    millimetres: 0,
    windKmh: 11,
    windFrom: 250,
    code: 1,
  },
  {
    at: 9,
    temperature: 22,
    apparent: 22,
    probability: 30,
    millimetres: 0,
    windKmh: 16,
    windFrom: 245,
    code: 3,
  },
  {
    at: 12,
    temperature: 23,
    apparent: 24,
    probability: 60,
    millimetres: 0.4,
    windKmh: 22,
    windFrom: 240,
    code: 61,
  },
  {
    at: 14,
    temperature: 19,
    apparent: 18,
    probability: 90,
    millimetres: 4.2,
    windKmh: 31,
    windFrom: 235,
    code: 95,
  },
  {
    at: 16,
    temperature: 15,
    apparent: 12,
    probability: 80,
    millimetres: 2.6,
    windKmh: 28,
    windFrom: 245,
    code: 65,
  },
  {
    at: 18,
    temperature: 13,
    apparent: 10,
    probability: 30,
    millimetres: 0.2,
    windKmh: 18,
    windFrom: 265,
    code: 80,
  },
  {
    at: 30,
    temperature: 12,
    apparent: 9,
    probability: 5,
    millimetres: 0,
    windKmh: 12,
    windFrom: 275,
    code: 3,
  },
];

function between(from: number, to: number, fraction: number): number {
  return from + (to - from) * fraction;
}

/**
 * The forecast the endpoint would answer with for these samples.
 *
 * Everything but the weather code is interpolated: a code is a name for a
 * kind of weather, and there is no state halfway between overcast and a
 * thunderstorm, so it holds the last keyframe's until the next one arrives.
 */
export const spikePoints: WeatherPoint[] = spikeSamples.map((sample, index) => {
  const after = KEYFRAMES.findIndex((frame) => frame.at > index);
  const previous = KEYFRAMES[after === -1 ? KEYFRAMES.length - 1 : Math.max(after - 1, 0)];
  const next = KEYFRAMES[after === -1 ? KEYFRAMES.length - 1 : after];
  if (!previous || !next) {
    throw new Error("the forecast keyframes are empty");
  }
  const span = next.at - previous.at;
  const fraction = span > 0 ? Math.min(Math.max((index - previous.at) / span, 0), 1) : 0;

  return {
    time: sample.arrivalAt.toISOString(),
    temperatureCelsius: between(previous.temperature, next.temperature, fraction),
    apparentTemperatureCelsius: between(previous.apparent, next.apparent, fraction),
    precipitationMillimetres: between(previous.millimetres, next.millimetres, fraction),
    precipitationProbabilityPercent: between(previous.probability, next.probability, fraction),
    windSpeedKmh: between(previous.windKmh, next.windKmh, fraction),
    windDirectionDegrees: between(previous.windFrom, next.windFrom, fraction),
    weatherCode: previous.code,
  };
});
