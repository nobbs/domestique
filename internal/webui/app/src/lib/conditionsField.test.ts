import { describe, expect, it } from "vitest";
import type { Position } from "../api/types";
import {
  corridorRadii,
  corridorWeight,
  projectToRoute,
  type ScalarSample,
  sampleScalarAt,
  sampleVectorAt,
  type WindSample,
  windComponents,
  windVector,
} from "./conditionsField";
import { cumulativeMetres } from "./profile";

/** One long straight segment due east at 49° N, about 7.3 km of it. */
const SEGMENT: Position[] = [
  [8, 49],
  [8.1, 49],
];
const SEGMENT_DISTANCES = cumulativeMetres(SEGMENT);
const SEGMENT_METRES = SEGMENT_DISTANCES[1] ?? 0;

const scalars: ScalarSample[] = [
  { distanceMetres: 1000, value: 10 },
  { distanceMetres: 3000, value: 20 },
];

describe("projectToRoute", () => {
  it("lands perpendicular onto the segment rather than on a vertex", () => {
    const projection = projectToRoute(SEGMENT, SEGMENT_DISTANCES, [8.05, 49.01]);

    expect(projection).not.toBeNull();
    expect(projection?.alongMetres ?? 0).toBeCloseTo(SEGMENT_METRES / 2, 0);
    expect(projection?.offsetMetres ?? 0).toBeCloseTo(1112, -1);
  });

  it("finds the nearer of two segments and projects onto it", () => {
    const dogleg: Position[] = [
      [8, 49],
      [8.05, 49],
      [8.05, 49.05],
    ];
    const distances = cumulativeMetres(dogleg);

    const projection = projectToRoute(dogleg, distances, [8.06, 49.03]);

    expect(projection?.alongMetres ?? 0).toBeGreaterThan(distances[1] ?? 0);
    expect(projection?.offsetMetres ?? 0).toBeCloseTo(730, -2);
  });

  it("survives a zero-length segment from coincident points", () => {
    const doubled: Position[] = [
      [8, 49],
      [8.05, 49],
      [8.05, 49],
      [8.1, 49],
    ];
    const distances = cumulativeMetres(doubled);

    const projection = projectToRoute(doubled, distances, [8.05, 49.01]);

    expect(projection?.alongMetres ?? Number.NaN).toBeCloseTo(distances[1] ?? 0, 0);
    expect(projection?.offsetMetres ?? Number.NaN).toBeCloseTo(1112, -1);
  });

  it("clamps a point beyond the start to the start", () => {
    const projection = projectToRoute(SEGMENT, SEGMENT_DISTANCES, [7.9, 49]);

    expect(projection?.alongMetres).toBe(0);
    expect(projection?.offsetMetres ?? 0).toBeCloseTo(7295, -2);
  });

  it("clamps a point beyond the finish to the finish", () => {
    const projection = projectToRoute(SEGMENT, SEGMENT_DISTANCES, [8.2, 49]);

    expect(projection?.alongMetres ?? 0).toBeCloseTo(SEGMENT_METRES, 6);
    expect(projection?.offsetMetres ?? 0).toBeCloseTo(7295, -2);
  });

  it("has nothing to say without a route or with distances that do not match", () => {
    const lone: Position[] = [[8, 49]];
    const nowhere: Position = [Number.NaN, 49];

    expect(projectToRoute(lone, [0], [8, 49])).toBeNull();
    expect(projectToRoute(SEGMENT, [0], [8, 49])).toBeNull();
    expect(projectToRoute(SEGMENT, SEGMENT_DISTANCES, nowhere)).toBeNull();
  });
});

describe("corridorWeight", () => {
  it("is full strength on the route itself", () => {
    expect(corridorWeight(0, 2000)).toBe(1);
  });

  it("holds full strength out to the core radius", () => {
    const { coreMetres, edgeMetres } = corridorRadii(2000);

    expect(coreMetres).toBe(1500);
    expect(edgeMetres).toBe(4000);
    expect(corridorWeight(coreMetres, 2000)).toBe(1);
  });

  it("is nought at the outer edge and beyond it", () => {
    expect(corridorWeight(4000, 2000)).toBe(0);
    expect(corridorWeight(9000, 2000)).toBe(0);
  });

  it("stays continuous across the core boundary", () => {
    expect(corridorWeight(1501, 2000)).toBeCloseTo(1, 4);
  });

  it("never rises as the offset grows", () => {
    const weights = Array.from({ length: 60 }, (_, step) => corridorWeight(step * 100, 2000));

    weights.forEach((weight, index) => {
      expect(weight).toBeLessThanOrEqual(weights[index - 1] ?? 1);
      expect(weight).toBeGreaterThanOrEqual(0);
      expect(weight).toBeLessThanOrEqual(1);
    });
  });

  it("draws a coarser forecast as a wider corridor", () => {
    expect(corridorWeight(3000, 7000)).toBeGreaterThan(corridorWeight(3000, 2000));
    expect(corridorWeight(8000, 11000)).toBeGreaterThan(corridorWeight(8000, 7000));
  });

  it("claims nothing off the route for a grid size it cannot read", () => {
    expect(corridorWeight(100, 0)).toBe(0);
    expect(corridorWeight(Number.NaN, 2000)).toBe(0);
  });
});

describe("sampleScalarAt", () => {
  it("interpolates between the two samples either side", () => {
    expect(sampleScalarAt(scalars, 2000)).toBeCloseTo(15, 6);
    expect(sampleScalarAt(scalars, 1500)).toBeCloseTo(12.5, 6);
  });

  it("clamps to the nearest sample outside their range", () => {
    expect(sampleScalarAt(scalars, 0)).toBeCloseTo(10, 6);
    expect(sampleScalarAt(scalars, 50_000)).toBeCloseTo(20, 6);
  });

  it("reads a lone sample as covering the whole route", () => {
    expect(sampleScalarAt([{ distanceMetres: 1000, value: 7 }], 9000)).toBeCloseTo(7, 6);
  });

  it("has nothing to say without samples or a distance", () => {
    expect(sampleScalarAt([], 100)).toBeNull();
    expect(sampleScalarAt(scalars, Number.NaN)).toBeNull();
  });
});

describe("sampleVectorAt", () => {
  it("interpolates across the 0/360 wrap instead of averaging backwards", () => {
    const samples: WindSample[] = [
      { distanceMetres: 0, speedKmh: 12, directionDegrees: 350 },
      { distanceMetres: 1000, speedKmh: 12, directionDegrees: 10 },
    ];

    const wind = sampleVectorAt(samples, 500);
    const degrees = wind?.directionDegrees ?? 0;

    expect(Math.min(degrees, 360 - degrees)).toBeCloseTo(0, 6);
    expect(wind?.speedKmh ?? 0).toBeCloseTo(12 * Math.cos((10 * Math.PI) / 180), 6);
  });

  it("cancels two opposing winds to almost no speed", () => {
    const samples: WindSample[] = [
      { distanceMetres: 0, speedKmh: 20, directionDegrees: 0 },
      { distanceMetres: 1000, speedKmh: 20, directionDegrees: 180 },
    ];

    expect(sampleVectorAt(samples, 500)?.speedKmh ?? 1).toBeCloseTo(0, 6);
  });

  it("leans toward the nearer sample", () => {
    const samples: WindSample[] = [
      { distanceMetres: 0, speedKmh: 10, directionDegrees: 90 },
      { distanceMetres: 1000, speedKmh: 30, directionDegrees: 90 },
    ];

    expect(sampleVectorAt(samples, 250)?.speedKmh ?? 0).toBeCloseTo(15, 6);
    expect(sampleVectorAt(samples, 250)?.directionDegrees ?? 0).toBeCloseTo(90, 6);
  });

  it("clamps outside the samples' range", () => {
    const samples: WindSample[] = [
      { distanceMetres: 2000, speedKmh: 8, directionDegrees: 270 },
      { distanceMetres: 4000, speedKmh: 16, directionDegrees: 270 },
    ];

    expect(sampleVectorAt(samples, 0)?.speedKmh ?? 0).toBeCloseTo(8, 6);
    expect(sampleVectorAt(samples, 9000)?.speedKmh ?? 0).toBeCloseTo(16, 6);
  });

  it("has nothing to say without samples", () => {
    expect(sampleVectorAt([], 100)).toBeNull();
  });
});

describe("windComponents", () => {
  it("round-trips a reading through its components", () => {
    const vector = windVector(windComponents({ speedKmh: 17, directionDegrees: 235 }));

    expect(vector.speedKmh).toBeCloseTo(17, 6);
    expect(vector.directionDegrees).toBeCloseTo(235, 6);
  });

  it("splits a northerly into a component blowing from the north", () => {
    const components = windComponents({ speedKmh: 10, directionDegrees: 0 });

    expect(components.northwardKmh).toBeCloseTo(10, 6);
    expect(components.eastwardKmh).toBeCloseTo(0, 6);
  });
});
