import { describe, expect, it } from "vitest";
import type { Position } from "../api/types";
import { buildProfile, nearestSample, niceStep, ticksFor } from "./profile";

/** Points spaced by latitude, so distance grows predictably along the route. */
function route(elevations: Array<number | undefined>, latitudeStep = 0.001): Position[] {
  return elevations.map((elevation, index) =>
    elevation === undefined
      ? ([8, 49 + index * latitudeStep] as Position)
      : ([8, 49 + index * latitudeStep, elevation] as Position),
  );
}

describe("buildProfile", () => {
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
