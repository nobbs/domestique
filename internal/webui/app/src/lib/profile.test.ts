import { describe, expect, it } from "vitest";
import type { Position } from "../api/types";
import {
  buildProfile,
  buildWindowedProfile,
  coordinateRange,
  nearestSample,
  niceStep,
  rangeBounds,
  sampleAt,
  ticksFor,
} from "./profile";

/** Points spaced by latitude, so distance grows predictably along the route. */
function route(elevations: Array<number | undefined>, latitudeStep = 0.001): Position[] {
  return elevations.map((elevation, index) =>
    elevation === undefined
      ? ([8, 49 + index * latitudeStep] as Position)
      : ([8, 49 + index * latitudeStep, elevation] as Position),
  );
}

/** One point every 0.001° of latitude is one point every 111.19 metres. */
const POINT_SPACING_METRES = 111.19;

/**
 * Twenty points of flat ground, then twenty at a steady ten percent.
 *
 * The climb begins about 2113 m in, which is what makes it possible to ask what
 * a window opening partway up it reports for its own first sample.
 */
function flatThenSteep(): Position[] {
  const points: Position[] = [];
  for (let index = 0; index < 20; index++) {
    points.push([8, 49 + index * 0.001, 100]);
  }
  for (let index = 1; index <= 20; index++) {
    points.push([8, 49 + (19 + index) * 0.001, 100 + index * (POINT_SPACING_METRES / 10)]);
  }

  return points;
}

const CLIMB_STARTS_METRES = 19 * POINT_SPACING_METRES;

describe("buildProfile", () => {
  it("describes the whole route as the stretch it covers", () => {
    const profile = buildProfile(route([100, 200, 300]), 5);

    expect(profile?.startMetres).toBe(0);
    expect(profile?.endMetres).toBeCloseTo(profile?.totalDistanceMetres ?? 0, 6);
  });

  it("samples evenly by distance and spans the whole route", () => {
    const profile = buildProfile(route([100, 200, 300]), 5);

    expect(profile).not.toBeNull();
    expect(profile?.samples).toHaveLength(5);
    expect(profile?.samples[0]?.distanceMetres).toBe(0);
    expect(profile?.samples[4]?.distanceMetres).toBeCloseTo(profile?.totalDistanceMetres ?? 0, 6);
  });

  it("reports the elevation range actually present", () => {
    // Sampling evenly by distance can land just past a turning point, so the
    // reported extreme approaches the true one as sampling gets denser rather
    // than matching it exactly.
    const profile = buildProfile(route([120, 80, 240]), 400);

    expect(profile?.minElevationMetres).toBeGreaterThanOrEqual(80);
    expect(profile?.minElevationMetres).toBeLessThan(81);
    expect(profile?.maxElevationMetres).toBeCloseTo(240, 1);
  });

  it("interpolates between source points rather than stepping", () => {
    // Midpoint of a straight 100 m climb should read about halfway up.
    const profile = buildProfile(route([100, 200]), 3);

    expect(profile?.samples[1]?.elevationMetres).toBeCloseTo(150, 0);
  });

  it("spaces samples by distance, not by point index", () => {
    // A dense cluster then a long leg: an index-based profile would give the
    // cluster half the width, misplacing where the climb happens.
    const coordinates: Position[] = [
      [8, 49, 100],
      [8, 49.0001, 100],
      [8, 49.0002, 100],
      [8, 49.05, 200],
    ];
    const profile = buildProfile(coordinates, 5);
    const midpoint = profile?.samples[2];

    // Halfway along the route by distance is out on the long leg, already
    // climbing — not still in the flat cluster.
    expect(midpoint?.elevationMetres ?? 0).toBeGreaterThan(120);
  });

  it("refuses a route with incomplete elevation rather than implying flat ground", () => {
    expect(buildProfile(route([100, undefined, 300]))).toBeNull();
  });

  it("refuses geometry too short or with no length", () => {
    expect(buildProfile([[8, 49, 100]])).toBeNull();
    expect(
      buildProfile([
        [8, 49, 100],
        [8, 49, 100],
      ]),
    ).toBeNull();
  });

  it("refuses fewer than two samples rather than dividing by zero", () => {
    for (const sampleCount of [-1, 0, 1]) {
      expect(buildProfile(route([100, 200, 300]), sampleCount)).toBeNull();
    }
  });
});

describe("buildWindowedProfile", () => {
  it("spends the whole sample budget on the stretch asked for", () => {
    const profile = buildWindowedProfile(
      route([100, 200, 300]),
      {
        startMetres: 100,
        endMetres: 200,
      },
      40,
    );

    expect(profile?.samples).toHaveLength(40);
    expect(profile?.startMetres).toBe(100);
    expect(profile?.endMetres).toBe(200);
  });

  // The whole link between map and chart rests on this: a distance means the
  // same ground in both, whatever either happens to be showing.
  it("measures distances from the start of the route, not of the window", () => {
    const profile = buildWindowedProfile(
      route([100, 200, 300]),
      {
        startMetres: 100,
        endMetres: 200,
      },
      40,
    );

    expect(profile?.samples[0]?.distanceMetres).toBeCloseTo(100, 6);
    expect(profile?.samples.at(-1)?.distanceMetres).toBeCloseTo(200, 6);
    expect(profile?.totalDistanceMetres).toBeCloseTo(2 * POINT_SPACING_METRES, 1);
  });

  it("fits the elevation range to the ground on show", () => {
    const profile = buildWindowedProfile(
      route([100, 200, 300]),
      {
        startMetres: 100,
        endMetres: 200,
      },
      40,
    );

    // The window opens most of the way up the first leg and closes most of the
    // way up the second, so neither end of the route's own range is in it.
    expect(profile?.minElevationMetres ?? 0).toBeGreaterThan(150);
    expect(profile?.maxElevationMetres ?? 0).toBeLessThan(300);
  });

  // Without a run-up the look-back has nothing behind it to measure against, and
  // the steepest pitch on a route would open as flat ground.
  it("measures gradient across the leading edge of the stretch", () => {
    const profile = buildWindowedProfile(flatThenSteep(), {
      startMetres: CLIMB_STARTS_METRES + 150,
      endMetres: CLIMB_STARTS_METRES + 650,
    });

    expect(profile?.samples[0]?.gradientPercent ?? 0).toBeCloseTo(10, 0);
  });

  it("leaves a stretch at the very start the shortfall the whole route has", () => {
    const profile = buildWindowedProfile(flatThenSteep(), { startMetres: 0, endMetres: 500 });

    // There is no ground before the first metre to measure a gradient against,
    // and inventing one would be worse than reporting none.
    expect(profile?.samples[0]?.gradientPercent).toBe(0);
    expect(profile?.startMetres).toBe(0);
  });

  it("clamps a window a drag pushed past either end of the route", () => {
    const coordinates = route([100, 200, 300]);
    const total = buildProfile(coordinates)?.totalDistanceMetres ?? 0;
    const profile = buildWindowedProfile(coordinates, {
      startMetres: -500,
      endMetres: total + 500,
    });

    expect(profile?.startMetres).toBe(0);
    expect(profile?.endMetres).toBeCloseTo(total, 6);
  });

  it("refuses a window of no length rather than dividing by it", () => {
    const coordinates = route([100, 200, 300]);

    expect(buildWindowedProfile(coordinates, { startMetres: 100, endMetres: 100 })).toBeNull();
    expect(buildWindowedProfile(coordinates, { startMetres: 200, endMetres: 100 })).toBeNull();
  });
});

describe("sampleAt", () => {
  it("interpolates between the samples a distance falls between", () => {
    const profile = buildProfile(route([100, 200]), 3);
    if (!profile) {
      throw new Error("expected a profile");
    }
    const quarter = profile.totalDistanceMetres / 4;

    expect(sampleAt(profile, quarter)?.elevationMetres).toBeCloseTo(125, 0);
    expect(sampleAt(profile, quarter)?.distanceMetres).toBeCloseTo(quarter, 6);
  });

  // A band is a class, and the average of two classes is not one of them.
  it("takes the band of the nearer sample rather than blending two", () => {
    const profile = buildWindowedProfile(flatThenSteep(), {
      startMetres: CLIMB_STARTS_METRES - 200,
      endMetres: CLIMB_STARTS_METRES + 400,
    });
    if (!profile) {
      throw new Error("expected a profile");
    }
    const bands = new Set(profile.samples.map((sample) => sample.band));

    for (const sample of profile.samples) {
      const found = sampleAt(profile, sample.distanceMetres);
      expect(bands).toContain(found?.band);
    }
  });

  // A chart showing two kilometres cannot mark a position five kilometres away,
  // and drawing a cursor at the nearest edge would claim that it can.
  it("reports nothing for a position outside the stretch it describes", () => {
    const profile = buildWindowedProfile(route([100, 200, 300]), {
      startMetres: 100,
      endMetres: 200,
    });
    if (!profile) {
      throw new Error("expected a profile");
    }

    expect(sampleAt(profile, 50)).toBeNull();
    expect(sampleAt(profile, 220)).toBeNull();
    // A hair past the edge is float error, not a position elsewhere.
    expect(sampleAt(profile, 200.2)).not.toBeNull();
  });
});

describe("coordinateRange", () => {
  // What is drawn from the range has to cover every metre the stretch asked for
  // rather than stopping just inside it.
  it("rounds outwards to the points either side of the stretch", () => {
    const coordinates = route([100, 200, 300, 400, 500]);

    const range = coordinateRange(coordinates, 150, 250);

    // Point 1 sits at 111 m and point 3 at 334 m, so the pair straddles both ends.
    expect(range).toEqual({ startIndex: 1, endIndex: 3 });
  });

  it("covers the whole route for a stretch that is the whole route", () => {
    const coordinates = route([100, 200, 300, 400, 500]);
    const total = buildProfile(coordinates)?.totalDistanceMetres ?? 0;

    expect(coordinateRange(coordinates, 0, total)).toEqual({ startIndex: 0, endIndex: 4 });
  });

  it("refuses a stretch of no length or geometry too short to have one", () => {
    expect(coordinateRange(route([100, 200, 300]), 100, 100)).toBeNull();
    expect(coordinateRange([[8, 49, 100]], 0, 100)).toBeNull();
  });
});

describe("rangeBounds", () => {
  /** A dog-leg, so west and east are not simply the first and last points. */
  const zigzag: Position[] = [
    [8.0, 49.0],
    [8.3, 49.2],
    [8.1, 49.4],
    [8.5, 49.1],
  ];

  it("contains every point of the stretch and nothing beyond it", () => {
    // Point 3 is the westernmost of the four and lies outside the range.
    expect(rangeBounds(zigzag, { startIndex: 1, endIndex: 2 })).toEqual([8.1, 49.2, 8.3, 49.4]);
  });

  it("spans the whole route when the range does", () => {
    expect(rangeBounds(zigzag, { startIndex: 0, endIndex: 3 })).toEqual([8.0, 49.0, 8.5, 49.4]);
  });

  it("gives a stretch of one point a box of no area", () => {
    // Somewhere to centre on rather than nothing: the map reads it as a place.
    expect(rangeBounds(zigzag, { startIndex: 2, endIndex: 2 })).toEqual([8.1, 49.4, 8.1, 49.4]);
  });

  it("refuses a range that starts past the end of the geometry", () => {
    expect(rangeBounds(zigzag, { startIndex: 9, endIndex: 12 })).toBeNull();
  });

  it("stops at the last point when the range runs past it", () => {
    expect(rangeBounds(zigzag, { startIndex: 2, endIndex: 9 })).toEqual([8.1, 49.1, 8.5, 49.4]);
  });
});

describe("niceStep", () => {
  it.each([
    [100, 4, 20],
    [1000, 5, 200],
    [7, 3, 2],
    [300, 3, 100],
  ])("splits a range of %p into about %p readable steps", (range, target, expected) => {
    expect(niceStep(range, target)).toBe(expected);
  });

  it("does not overshoot on a small range", () => {
    // A 7 km route must get more than a first and last label.
    expect(ticksFor(0, 7, 3).length).toBeGreaterThanOrEqual(3);
  });

  it("never returns zero for a degenerate range", () => {
    expect(niceStep(0, 4)).toBeGreaterThan(0);
  });
});

describe("ticksFor", () => {
  it("produces round values inside the range", () => {
    const ticks = ticksFor(96, 312, 3);

    expect(ticks.length).toBeGreaterThan(1);
    for (const tick of ticks) {
      expect(tick).toBeGreaterThanOrEqual(96);
      expect(tick).toBeLessThanOrEqual(312);
      expect(Number.isInteger(tick)).toBe(true);
    }
  });
});

describe("nearestSample", () => {
  it("finds the sample under a point on the route", () => {
    const profile = buildProfile(route([100, 150, 200]), 21);
    if (!profile) {
      throw new Error("expected a profile");
    }
    const target = profile.samples[10];
    if (!target) {
      throw new Error("expected a sample");
    }

    const found = nearestSample(profile, target.longitude, target.latitude);

    expect(found).toBe(10);
  });

  it("carries coordinates on every sample so the map can mark one", () => {
    const profile = buildProfile(route([100, 200]), 5);

    for (const sample of profile?.samples ?? []) {
      expect(Number.isFinite(sample.longitude)).toBe(true);
      expect(Number.isFinite(sample.latitude)).toBe(true);
    }
  });

  it("weights longitude by latitude, so north-south distance is not understated", () => {
    // Two candidates equidistant in raw degrees: one along the route's own
    // north-south line, one displaced east. Near 49° a degree east is about
    // two-thirds of a degree north, so the eastern point is genuinely closer.
    const profile = buildProfile(route([100, 100, 100]), 3);
    if (!profile) {
      throw new Error("expected a profile");
    }
    const middle = profile.samples[1];
    if (!middle) {
      throw new Error("expected a sample");
    }

    const eastward = nearestSample(profile, middle.longitude + 0.0005, middle.latitude);

    expect(eastward).toBe(1);
  });
});
