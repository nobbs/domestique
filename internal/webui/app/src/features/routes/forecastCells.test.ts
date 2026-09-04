/**
 * What the wind is doing to the rider, along a road whose direction is known.
 *
 * The bug worth a test here is a sign error: a road ridden into the wind and the
 * same road ridden the other way have to come out as opposite readings, and
 * nothing about a headwind drawn as a tailwind looks wrong on a map. The other
 * is the switchback — a road that is honestly heading two ways at once must not
 * be reported as a confident crosswind.
 */

import { describe, expect, it } from "vitest";
import type { Position, WeatherPoint } from "../../api/types";
import type { ForecastSample } from "../../lib/forecastSamples";
import { cumulativeMetres } from "../../lib/profile";
import { buildCells, relationAt, windRuns } from "./forecastCells";

/** Points spaced evenly along one compass bearing, roughly `stepMetres` apart. */
function straightRoute(bearingDegrees: number, pointCount: number, stepMetres = 150): Position[] {
  const metresPerDegreeLatitude = 111_320;
  const latitude = 49;
  const metresPerDegreeLongitude = metresPerDegreeLatitude * Math.cos((latitude * Math.PI) / 180);
  const radians = (bearingDegrees * Math.PI) / 180;

  return Array.from({ length: pointCount }, (_, index): Position => {
    const northMetres = Math.cos(radians) * stepMetres * index;
    const eastMetres = Math.sin(radians) * stepMetres * index;

    return [
      8 + eastMetres / metresPerDegreeLongitude,
      latitude + northMetres / metresPerDegreeLatitude,
    ];
  });
}

/** A road that heads due east and then turns and comes back along itself. */
function switchback(): Position[] {
  const outbound = straightRoute(90, 9);
  const last = outbound[outbound.length - 1] as Position;
  const inbound = straightRoute(270, 9).map(
    ([longitude, latitude]): Position => [longitude + (last[0] - 8), latitude + (last[1] - 49)],
  );

  return [...outbound, ...inbound.slice(1)];
}

function point(windDirectionDegrees: number, windSpeedKmh = 20): WeatherPoint {
  return {
    time: new Date().toISOString(),
    temperatureCelsius: 14,
    apparentTemperatureCelsius: 14,
    precipitationMillimetres: 0,
    precipitationProbabilityPercent: 0,
    windSpeedKmh,
    windDirectionDegrees,
    weatherCode: 1,
    cloudCoverPercent: 5,
  };
}

/** Three readings spread evenly along a route, the shape a stage's forecast has. */
function samplesAlong(route: Position[]): ForecastSample[] {
  const distances = cumulativeMetres(route);
  const total = distances[distances.length - 1] ?? 0;

  return [0, 0.5, 1].map((share) => ({
    position: route[Math.round(share * (route.length - 1))] as Position,
    distanceMetres: share * total,
    arrivalAt: new Date(Date.now() + (1 + share) * 3_600_000),
  }));
}

/** Every stop the tint would draw along one route under one wind, in order. */
function stopsAlong(route: Position[], windFromDegrees: number): (number | null)[] {
  const samples = samplesAlong(route);
  const points = samples.map(() => point(windFromDegrees));

  return windRuns(buildCells(samples, points, route), route, cumulativeMetres(route)).map(
    (run) => run.stop,
  );
}

/** The stops the tint uses, without the run-per-reading cuts between them. */
function distinctStops(route: Position[], windFromDegrees: number): (number | null)[] {
  return [...new Set(stopsAlong(route, windFromDegrees))];
}

const EAST = straightRoute(90, 21);
const WEST = straightRoute(270, 21);

describe("the wind against the way the road runs", () => {
  it("reads a road run into the wind as a headwind, end to end", () => {
    expect(distinctStops(EAST, 90)).toEqual([0]);
  });

  it("reads the same road ridden the other way as a tailwind", () => {
    expect(distinctStops(WEST, 90)).toEqual([3]);
  });

  /*
   * The reversal, asked of one road rather than of two fixtures: the same
   * geometry, the same wind, the direction of travel flipped. A sign error
   * cannot survive both of these and the pair above.
   */
  it("swaps head for tail when the road is reversed under the same wind", () => {
    const there = distinctStops(EAST, 90);
    const back = distinctStops([...EAST].reverse(), 90);

    expect(there).toEqual([0]);
    expect(back).toEqual([3]);
  });

  it("leans a crosswind to the side it is pushing, rather than to neither", () => {
    // Riding east: a wind out of the north-east is still coming from ahead of
    // the rider, and one out of the south-west is already behind them, even
    // though `windRelation` calls both of them a crosswind.
    expect(distinctStops(EAST, 40)).toEqual([1]);
    expect(distinctStops(EAST, 220)).toEqual([2]);
  });
});

describe("a road that turns back on itself", () => {
  it("reads as mixed rather than as a confident crosswind", () => {
    const route = switchback();
    const distances = cumulativeMetres(route);
    const turn = (distances[distances.length - 1] ?? 0) / 2;

    // Due north, which would read as a crosswind on either arm taken alone.
    expect(relationAt(route, distances, turn, 0).relation).toBe("mixed");
  });

  it("draws the turn in the neutral, and the arms either side in the ramp", () => {
    const stops = stopsAlong(switchback(), 0);

    expect(stops).toContain(null);
    expect(stops.some((stop) => stop !== null)).toBe(true);
  });
});

describe("the stretches the tint is cut into", () => {
  it("tiles the route, each stretch starting where the last one ended", () => {
    const route = switchback();
    const distances = cumulativeMetres(route);
    const totalMetres = distances[distances.length - 1] ?? 0;
    const samples = samplesAlong(route);
    const runs = windRuns(
      buildCells(
        samples,
        samples.map(() => point(0)),
        route,
      ),
      route,
      distances,
    );

    expect(runs.length).toBeGreaterThan(1);
    expect(runs[0]?.fromMetres).toBe(0);
    expect(runs[runs.length - 1]?.toMetres).toBeCloseTo(totalMetres, 6);
    for (const [index, run] of runs.entries()) {
      expect(run.fromMetres).toBeCloseTo(runs[index - 1]?.toMetres ?? 0, 6);
      expect(run.toMetres).toBeGreaterThan(run.fromMetres);
    }
  });

  it("carries the wind speed of the reading whose stretch it falls in", () => {
    const samples = samplesAlong(EAST);
    const points = [point(90, 5), point(90, 40), point(90, 5)];
    const runs = windRuns(buildCells(samples, points, EAST), EAST, cumulativeMetres(EAST));

    expect(new Set(runs.map((run) => run.windSpeedKmh))).toEqual(new Set([5, 40]));
  });

  it("has nothing to cut on a route with no ground", () => {
    const route: Position[] = [[8, 49]];
    const samples = samplesAlong(route);

    expect(
      windRuns(
        buildCells(
          samples,
          samples.map(() => point(90)),
          route,
        ),
        route,
        [0],
      ),
    ).toEqual([]);
  });
});
