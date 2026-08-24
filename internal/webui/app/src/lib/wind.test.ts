import { describe, expect, it } from "vitest";
import type { Position } from "../api/types";
import { cumulativeMetres } from "./profile";
import { bearingAt, bearingIsMixed, windRelation } from "./wind";

/** Points spaced evenly along one compass bearing, roughly `stepMetres` apart. */
function straightRoute(bearingDegrees: number, pointCount: number, stepMetres = 100): Position[] {
  // Small enough steps that the flat-earth approximation below tracks the
  // spherical model wind.ts actually measures against closely enough for a
  // fixture — this is a route to take a bearing across, not a distance to
  // trust to the metre.
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

/** A route that heads due east, then turns and heads back the way it came. */
function switchback(): Position[] {
  const outbound = straightRoute(90, 6, 150);
  const last = outbound[outbound.length - 1] as Position;
  const inbound = straightRoute(270, 6, 150).map(
    ([longitude, latitude]): Position => [longitude + (last[0] - 8), latitude + (last[1] - 49)],
  );

  return [...outbound, ...inbound.slice(1)];
}

describe("bearingAt", () => {
  it("reads a due-north route as heading 0 degrees", () => {
    const route = straightRoute(0, 8);
    const distances = cumulativeMetres(route);

    const bearing = bearingAt(route, distances, distances[4] ?? 0, 400);

    expect(bearing).not.toBeNull();
    expect(bearing ?? 0).toBeCloseTo(0, 0);
  });

  it("agrees with itself at twice the point density", () => {
    const sparse = straightRoute(35, 8, 200);
    const dense = straightRoute(35, 15, 100);
    const sparseDistances = cumulativeMetres(sparse);
    const denseDistances = cumulativeMetres(dense);

    const atMetres = sparseDistances[4] ?? 0;
    const sparseBearing = bearingAt(sparse, sparseDistances, atMetres, 600);
    const denseBearing = bearingAt(dense, denseDistances, atMetres, 600);

    expect(sparseBearing).not.toBeNull();
    expect(denseBearing).not.toBeNull();
    expect(denseBearing ?? 0).toBeCloseTo(sparseBearing ?? 0, 1);
  });

  it("returns null off the end of a route with no ground to measure", () => {
    const bearing = bearingAt([[8, 49]], [0], 0, 400);

    expect(bearing).toBeNull();
  });

  it("still reads a bearing at the very first and very last point", () => {
    // Not an edge case in practice: forecastSamples always includes both ends,
    // so every strip drawn takes these two bearings. Half the window falls off
    // the route at each, leaving one side to measure from.
    const route = straightRoute(0, 8);
    const distances = cumulativeMetres(route);

    const atStart = bearingAt(route, distances, 0, 600);
    const atEnd = bearingAt(route, distances, distances[7] ?? 0, 600);

    expect(atStart ?? 0).toBeCloseTo(0, 0);
    expect(atEnd ?? 0).toBeCloseTo(0, 0);
  });

  it("reads a window wider than the route as the whole route's bearing", () => {
    const route = straightRoute(90, 4, 100);
    const distances = cumulativeMetres(route);

    const bearing = bearingAt(route, distances, distances[2] ?? 0, 100_000);

    expect(bearing ?? 0).toBeCloseTo(90, 0);
  });

  it("returns null when the coordinates and their distances disagree", () => {
    const route = straightRoute(0, 4);

    expect(bearingAt(route, [0, 100], 50, 400)).toBeNull();
  });

  it("widens a window narrower than one segment rather than measuring nothing", () => {
    // Points 100 m apart, a window of 10 m: every candidate segment falls
    // outside it. One segment is taken anyway, because a bearing of "no
    // answer" here would blank the wind reading on a perfectly ordinary route.
    const route = straightRoute(180, 6, 100);
    const distances = cumulativeMetres(route);

    const bearing = bearingAt(route, distances, distances[3] ?? 0, 10);

    expect(bearing ?? 0).toBeCloseTo(180, 0);
  });

  it("widens backwards when the narrow window sits at the finish", () => {
    // The same widening as above, at the one place there is no segment ahead
    // to reach for — the final coordinate, which every strip samples.
    const route = straightRoute(270, 6, 100);
    const distances = cumulativeMetres(route);

    const bearing = bearingAt(route, distances, distances[5] ?? 0, 10);

    expect(bearing ?? 0).toBeCloseTo(270, 0);
  });

  it("ignores a repeated coordinate that covers no ground", () => {
    const route = straightRoute(90, 5, 100);
    const stalled: Position[] = [...route.slice(0, 3), route[2] as Position, ...route.slice(3)];
    const distances = cumulativeMetres(stalled);

    const bearing = bearingAt(stalled, distances, distances[3] ?? 0, 600);

    expect(bearing ?? 0).toBeCloseTo(90, 0);
  });

  it("returns null for a route that never leaves its first point", () => {
    const stuck: Position[] = [
      [8, 49],
      [8, 49],
      [8, 49],
    ];

    expect(bearingAt(stuck, cumulativeMetres(stuck), 0, 600)).toBeNull();
  });
});

describe("windRelation", () => {
  it("reads a due-north wind on a due-north route as a headwind", () => {
    const reading = windRelation(0, 0);

    expect(reading.relation).toBe("head");
    expect(reading.componentKmhPerKmh).toBeLessThan(0);
  });

  it("flips to a tailwind once the route reverses", () => {
    const reading = windRelation(180, 0);

    expect(reading.relation).toBe("tail");
    expect(reading.componentKmhPerKmh).toBeGreaterThan(0);
  });

  it("reads a perpendicular wind as a crosswind", () => {
    const reading = windRelation(0, 90);

    expect(reading.relation).toBe("cross");
  });

  it("does not care which side the crosswind comes from", () => {
    expect(windRelation(0, 90).relation).toBe("cross");
    expect(windRelation(0, 270).relation).toBe("cross");
  });
});

describe("bearingIsMixed", () => {
  it("reads a switchback as mixed", () => {
    const route = switchback();
    const distances = cumulativeMetres(route);
    const turnMetres = distances[5] ?? 0;

    expect(bearingIsMixed(route, distances, turnMetres, 1400)).toBe(true);
  });

  it("does not read a straight road as mixed", () => {
    const route = straightRoute(60, 8, 150);
    const distances = cumulativeMetres(route);

    expect(bearingIsMixed(route, distances, distances[4] ?? 0, 600)).toBe(false);
  });
});
