import { describe, expect, it } from "vitest";
import type { Position } from "../api/types";
import type { ForecastSample } from "./forecastSamples";
import {
  forecastLeadHours,
  forecastSamples,
  MAX_SAMPLES,
  MIN_SAMPLE_SPACING_METRES,
  SAMPLE_INTERVAL_SECONDS,
} from "./forecastSamples";
import { cumulativeMetres } from "./profile";

const START_AT = new Date("2026-08-24T06:00:00Z");

/**
 * Points spaced evenly by latitude, paired with a moving-time series spaced
 * evenly by point index. A synthetic, unrealistic pairing of ground and clock
 * — real predictions never march in lockstep with point index — but an exact
 * one, which is what makes the sample indices this test expects predictable
 * rather than approximate.
 */
function evenlyTimedRoute(pointCount: number, latitudeStep: number, secondsPerPoint: number) {
  const coordinates: Position[] = Array.from({ length: pointCount }, (_, index) => [
    8,
    49 + index * latitudeStep,
  ]);
  const cumulativeSeconds = Array.from(
    { length: pointCount },
    (_, index) => index * secondsPerPoint,
  );

  return { coordinates, cumulativeSeconds };
}

/**
 * A ride at constant speed: moving time is the real great-circle distance
 * between consecutive points divided by the speed, using the same
 * `cumulativeMetres` `forecastSamples` measures with — so this fixture can
 * never disagree with the function under test about where the ground is.
 */
function constantSpeedRoute(
  pointCount: number,
  latitudeStep: number,
  speedMetresPerSecond: number,
) {
  const coordinates: Position[] = Array.from({ length: pointCount }, (_, index) => [
    8,
    49 + index * latitudeStep,
  ]);
  const cumulativeSeconds = cumulativeMetres(coordinates).map(
    (metres) => metres / speedMetresPerSecond,
  );

  return { coordinates, cumulativeSeconds };
}

describe("forecastSamples", () => {
  it("lands a sample every 30 minutes of moving time, with the first and last coordinate always present", () => {
    // 13 points, 10 minutes apart in moving time and about 2.2 km apart on
    // the ground — comfortably clear of the 5 km floor, so only the timing
    // rule decides which points are kept.
    const { coordinates, cumulativeSeconds } = evenlyTimedRoute(13, 0.02, 600);

    const samples = forecastSamples(coordinates, cumulativeSeconds, START_AT);

    expect(samples).toHaveLength(5);
    expect(samples[0]?.position).toEqual(coordinates[0]);
    expect(samples[4]?.position).toEqual(coordinates[12]);
    for (let index = 1; index < samples.length; index++) {
      const previous = samples[index - 1];
      const current = samples[index];
      const gapSeconds =
        ((current?.arrivalAt.getTime() ?? 0) - (previous?.arrivalAt.getTime() ?? 0)) / 1000;
      expect(gapSeconds).toBeCloseTo(SAMPLE_INTERVAL_SECONDS, 6);
    }
  });

  it("thins samples where 30 minutes of a sustained climb covers less than the 5 km floor", () => {
    // 361 points, 10 seconds and about 20 metres apart — so the 30-minute mark
    // (180 points in) sits only about 3.6 km from the start, inside the floor,
    // while the full hour (360 points in) clears it.
    const { coordinates, cumulativeSeconds } = evenlyTimedRoute(361, 0.00018, 10);

    const samples = forecastSamples(coordinates, cumulativeSeconds, START_AT);

    // Without the floor this would be three samples (0, 30 and 60 minutes in);
    // the 30-minute mark is thinned out because it lies too close to the start.
    expect(samples).toHaveLength(2);
    expect(samples[0]?.position).toEqual(coordinates[0]);
    expect(samples[1]?.position).toEqual(coordinates[360]);
    const keptGapMetres = (samples[1]?.distanceMetres ?? 0) - (samples[0]?.distanceMetres ?? 0);
    expect(keptGapMetres).toBeGreaterThanOrEqual(MIN_SAMPLE_SPACING_METRES);
  });

  it("drops the sample before the finish rather than the finish itself when the two sit inside the floor", () => {
    // Three long legs and then a few metres more: the ride ends just past the
    // 90-minute mark, so the last sample lands within the floor of the one
    // before it. The finish is the sample a rider cares about, so it stays and
    // its too-close predecessor goes.
    const coordinates: Position[] = [
      [8, 49],
      [8, 49.1],
      [8, 49.2],
      [8, 49.2001],
    ];

    const samples = forecastSamples(coordinates, [0, 1800, 3600, 5400], START_AT);

    expect(samples.map((sample) => sample.position)).toEqual([
      coordinates[0],
      coordinates[1],
      coordinates[3],
    ]);
  });

  it("sets each arrival time to the start plus the moving time at that coordinate, ending at the whole moving time", () => {
    const { coordinates, cumulativeSeconds } = evenlyTimedRoute(13, 0.02, 600);
    const totalSeconds = cumulativeSeconds[cumulativeSeconds.length - 1] ?? 0;

    const samples = forecastSamples(coordinates, cumulativeSeconds, START_AT);

    for (const sample of samples) {
      const coordinateIndex = coordinates.indexOf(sample.position);
      const expectedSeconds = cumulativeSeconds[coordinateIndex] ?? 0;
      expect(sample.arrivalAt.getTime()).toBe(START_AT.getTime() + expectedSeconds * 1000);
    }
    expect(samples.at(-1)?.arrivalAt.getTime()).toBe(START_AT.getTime() + totalSeconds * 1000);
  });

  it("leaves samples materially unchanged when a route's point density doubles", () => {
    // The same stretch of meridian, described first at roughly 111 m and then
    // roughly 56 m between points — twice the density, the same road.
    const speedMetresPerSecond = 6;
    const sparse = constantSpeedRoute(601, 0.001, speedMetresPerSecond);
    const dense = constantSpeedRoute(1201, 0.0005, speedMetresPerSecond);

    const sparseSamples = forecastSamples(sparse.coordinates, sparse.cumulativeSeconds, START_AT);
    const denseSamples = forecastSamples(dense.coordinates, dense.cumulativeSeconds, START_AT);

    expect(denseSamples).toHaveLength(sparseSamples.length);
    sparseSamples.forEach((sample, index) => {
      const match = denseSamples[index];
      // Tolerance is one sparse-route point spacing (~111 m, ~19 s at 6 m/s):
      // the most either density's quantization to its nearest point can move
      // a sample from where the exact 30-minute mark actually falls.
      const distanceGapMetres = Math.abs((match?.distanceMetres ?? 0) - sample.distanceMetres);
      expect(distanceGapMetres).toBeLessThan(150);
      const arrivalGapSeconds =
        Math.abs((match?.arrivalAt.getTime() ?? 0) - sample.arrivalAt.getTime()) / 1000;
      expect(arrivalGapSeconds).toBeLessThan(30);
    });
  });

  it("caps at 48 samples on a route long enough to need more, spanning the whole route with a widened interval", () => {
    // ~1111 km at 6 m/s is a touch over 51 hours moving — more than the 48
    // half-hour slots the cap allows for, and the ~1.1 km between points
    // stays well clear of the 5 km floor.
    const { coordinates, cumulativeSeconds } = constantSpeedRoute(1000, 0.01, 6);

    const samples = forecastSamples(coordinates, cumulativeSeconds, START_AT);

    expect(samples).toHaveLength(MAX_SAMPLES);
    expect(samples[0]?.position).toEqual(coordinates[0]);
    expect(samples.at(-1)?.position).toEqual(coordinates[999]);
    const firstGapSeconds =
      ((samples[1]?.arrivalAt.getTime() ?? 0) - (samples[0]?.arrivalAt.getTime() ?? 0)) / 1000;
    expect(firstGapSeconds).toBeGreaterThan(SAMPLE_INTERVAL_SECONDS);
    const totalSeconds = cumulativeSeconds[cumulativeSeconds.length - 1] ?? 0;
    expect(samples.at(-1)?.arrivalAt.getTime()).toBeCloseTo(
      START_AT.getTime() + totalSeconds * 1000,
      -3,
    );
  });

  it("returns a single sample rather than throwing for a single-point stage with a predicted time", () => {
    const samples = forecastSamples([[8, 49]], [500], START_AT);

    expect(samples).toEqual([
      { position: [8, 49], distanceMetres: 0, arrivalAt: new Date(START_AT.getTime() + 500_000) },
    ]);
  });

  it("returns an empty list rather than throwing for a stage with zero total moving time", () => {
    expect(forecastSamples([[8, 49]], [0], START_AT)).toEqual([]);
    expect(
      forecastSamples(
        [
          [8, 49],
          [8, 50],
        ],
        [0, 0],
        START_AT,
      ),
    ).toEqual([]);
  });

  it("returns something sane rather than throwing for a stage with zero ground length", () => {
    const coordinates: Position[] = [
      [8, 49],
      [8, 49],
      [8, 49],
    ];

    const samples = forecastSamples(coordinates, [0, 300, 600], START_AT);

    expect(samples.map((sample) => sample.position)).toEqual([
      [8, 49],
      [8, 49],
    ]);
    expect(samples.map((sample) => sample.arrivalAt.getTime())).toEqual([
      START_AT.getTime(),
      START_AT.getTime() + 600_000,
    ]);
  });

  it("samples the finish even when the last coordinates share one moving time", () => {
    // A stage ending in a zero-length segment: the clock stops before the
    // geometry does, so the naive cursor would stop on the second-to-last
    // coordinate and never sample the point the rider finishes at.
    const coordinates: Position[] = [
      [8, 49],
      [8, 49.1],
      [8, 49.2],
      [8, 49.2],
    ];

    const samples = forecastSamples(coordinates, [0, 1800, 3600, 3600], START_AT);

    expect(samples.at(-1)?.position).toEqual(coordinates[3]);
    expect(samples.at(-1)?.arrivalAt.getTime()).toBe(START_AT.getTime() + 3_600_000);
  });

  it("keeps the finish inside the cap on a long route that ends in a zero-length segment", () => {
    // Both rules at once: a ride long enough to fill all 48 slots, ending in a
    // segment the clock does not advance over. The finish must still be the
    // last sample, and the cap must still hold — 49 points would be refused by
    // the weather endpoint outright.
    const { coordinates, cumulativeSeconds } = constantSpeedRoute(1000, 0.01, 6);
    const stalled: Position[] = [...coordinates, coordinates[999] as Position];
    const stalledSeconds = [...cumulativeSeconds, cumulativeSeconds[999] as number];

    const samples = forecastSamples(stalled, stalledSeconds, START_AT);

    expect(samples).toHaveLength(MAX_SAMPLES);
    expect(samples.at(-1)?.position).toEqual(stalled[1000]);
  });

  /*
   * ForecastStrip tells its cells apart by distance and arrival together, so
   * that pair has to be unique — a duplicate would collide as a React key and
   * let reconciliation drop or misplace a row. The shapes below are the ones
   * that push samples together: a stage standing still, a finish sharing its
   * predecessor's clock, and a ride whose whole timeline is one instant.
   */
  it("never returns two samples sharing both a distance and an arrival", () => {
    const stalledFinish: Position[] = [
      [8, 49],
      [8, 49.1],
      [8, 49.2],
      [8, 49.2],
    ];
    const motionless: Position[] = [
      [8, 49],
      [8, 49],
      [8, 49],
    ];
    const cases: Array<[Position[], number[]]> = [
      [stalledFinish, [0, 1800, 3600, 3600]],
      [motionless, [0, 300, 600]],
      [motionless, [0, 0, 600]],
      [stalledFinish, [0, 0, 0, 5400]],
    ];

    for (const [coordinates, cumulativeSeconds] of cases) {
      const samples = forecastSamples(coordinates, cumulativeSeconds, START_AT);
      const keys = samples.map(
        (sample) => `${sample.distanceMetres}-${sample.arrivalAt.getTime()}`,
      );

      expect(new Set(keys).size, `distinct keys for ${JSON.stringify(cumulativeSeconds)}`).toBe(
        keys.length,
      );
    }
  });

  /*
   * A stage shorter than the 5 km floor cannot satisfy it: its only two
   * samples are the departure and the finish, and they are unavoidably closer
   * together than the floor asks. Dropping the finish there would leave a
   * rider with the weather at the start line and nothing about getting home,
   * on exactly the rides that are quickest to fit in.
   */
  it("keeps the finish on a stage shorter than the sample spacing floor", () => {
    // Four kilometres, twenty minutes: inside the floor and inside one slot.
    const coordinates = evenlyTimedRoute(5, 0.009, 300).coordinates;
    const cumulativeSeconds = [0, 300, 600, 900, 1200];

    const samples = forecastSamples(coordinates, cumulativeSeconds, START_AT);

    expect(samples).toHaveLength(2);
    expect(samples[0]?.position).toEqual(coordinates[0]);
    expect(samples[1]?.position).toEqual(coordinates[4]);
  });

  it("returns an empty list rather than throwing for a stage nothing has predicted", () => {
    expect(
      forecastSamples(
        [
          [8, 49],
          [8, 50],
        ],
        undefined,
        START_AT,
      ),
    ).toEqual([]);
  });

  it("returns an empty list rather than throwing for an empty moving-time series", () => {
    expect(
      forecastSamples(
        [
          [8, 49],
          [8, 50],
        ],
        [],
        START_AT,
      ),
    ).toEqual([]);
  });

  it("returns an empty list rather than throwing when coordinates and moving times disagree in length", () => {
    expect(
      forecastSamples(
        [
          [8, 49],
          [8, 50],
          [8, 51],
        ],
        [0, 600],
        START_AT,
      ),
    ).toEqual([]);
  });
});

describe("forecastLeadHours", () => {
  function sampleAt(arrivalAt: Date): ForecastSample[] {
    return [{ position: [8, 49], distanceMetres: 0, arrivalAt }];
  }

  it("reports the hours until the first sample arrives", () => {
    const now = new Date("2026-08-24T06:00:00Z");
    const arrivalAt = new Date("2026-08-24T09:00:00Z");

    expect(forecastLeadHours(sampleAt(arrivalAt), now)).toBe(3);
  });

  it("clamps at 0 for a start already past", () => {
    const now = new Date("2026-08-24T09:00:00Z");
    const arrivalAt = new Date("2026-08-24T06:00:00Z");

    expect(forecastLeadHours(sampleAt(arrivalAt), now)).toBe(0);
  });

  it("is 0 without any samples", () => {
    expect(forecastLeadHours([], new Date())).toBe(0);
  });
});
