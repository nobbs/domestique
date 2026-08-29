import { describe, expect, it } from "vitest";
import type { Position } from "../api/types";
import {
  buildProfile,
  buildWindowedProfile,
  coordinateRange,
  cumulativeMetres,
  GRADIENT_BANDS,
  GRADIENT_WINDOW_METRES,
  gradientBand,
  gradientMix,
  gradientRanges,
  gradientShares,
  movingSecondsForWindow,
  nearestSample,
  niceStep,
  rangeBounds,
  sampleAt,
  ticksFor,
} from "./profile";

/** The bands a stage has, gentlest first: what the key offers as chips. */
function bandsOf(coordinates: Position[]): number[] {
  return gradientShares(coordinates).map((entry) => entry.band);
}

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

/** One ten-thousandth of a degree of latitude is 11.119 metres. */
const FINE_SPACING_METRES = 11.119;

/**
 * A route whose segments run at the given gradients, in percent.
 *
 * Points sit eleven metres apart, so the hundred-metre look-back spans about
 * nine segments — close to the spacing a recorded track actually has, and the
 * only spacing at which the smoothing can be seen doing anything.
 */
function ramp(percents: number[]): Position[] {
  const points: Position[] = [[8, 49, 100]];
  percents.forEach((percent, index) => {
    const previous = points[index] as [number, number, number];
    points.push([
      8,
      49 + (index + 1) * 0.0001,
      previous[2] + (FINE_SPACING_METRES * percent) / 100,
    ]);
  });

  return points;
}

/**
 * A route from segments of the given length, in degrees of latitude, each
 * arriving at the given elevation.
 *
 * Unevenly spaced deliberately: how long a run is, and how long the window it
 * was measured over is, are two different lengths, and a uniform route cannot
 * hold one of them still while the other changes.
 */
function unevenRoute(steps: Array<[latitudeDelta: number, elevation: number]>): Position[] {
  let latitude = 49;

  return steps.map(([delta, elevation], index) => {
    latitude += index === 0 ? 0 : delta;

    return [8, latitude, elevation] as Position;
  });
}

/** Two hundred metres of latitude, twice the window a gradient is measured over. */
const LONG_STEP = 0.0018;
/** Sixty metres: a pitch shorter than the window that measured it. */
const SHORT_STEP = 0.00054;

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

describe("movingSecondsForWindow", () => {
  const coordinates = route([100, 200, 300, 400, 500]);
  // Ten seconds per point, so the window's own moving time is easy to check
  // against coordinateRange's own start/end indices.
  const cumulativeSeconds = [0, 10, 20, 30, 40];

  it("subtracts the cumulative series at the selection's rounded-outward boundaries", () => {
    const moving = movingSecondsForWindow(coordinates, cumulativeSeconds, {
      startMetres: 150,
      endMetres: 250,
    });

    // coordinateRange rounds this stretch out to indices 1 and 3.
    expect(moving).toBe((cumulativeSeconds[3] ?? 0) - (cumulativeSeconds[1] ?? 0));
  });

  it("is undefined with no selection", () => {
    expect(movingSecondsForWindow(coordinates, cumulativeSeconds, null)).toBeUndefined();
  });

  it("is undefined with no predicted series", () => {
    expect(
      movingSecondsForWindow(coordinates, undefined, { startMetres: 150, endMetres: 250 }),
    ).toBeUndefined();
  });

  it("is undefined for a selection too short to span two coordinates", () => {
    expect(
      movingSecondsForWindow(coordinates, cumulativeSeconds, { startMetres: 100, endMetres: 100 }),
    ).toBeUndefined();
  });

  it("is undefined for a non-monotonic or duplicated series rather than a zero or negative moving time", () => {
    const flat = [0, 10, 10, 10, 40];
    expect(
      movingSecondsForWindow(coordinates, flat, { startMetres: 150, endMetres: 250 }),
    ).toBeUndefined();

    const nonMonotonic = [0, 20, 20, 10, 40];
    expect(
      movingSecondsForWindow(coordinates, nonMonotonic, { startMetres: 150, endMetres: 250 }),
    ).toBeUndefined();
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

describe("gradientBand", () => {
  it("bands by magnitude, so a descent is as steep as the climb back up", () => {
    expect(gradientBand(-14)).toBe(gradientBand(14));
  });

  /*
   * The bands are half-open: a gradient on a boundary belongs to the band above
   * it. The labels have to say the same thing, or the key would name a band a
   * reader's own number does not fall into.
   */
  it("puts a gradient on a boundary in the band that boundary opens", () => {
    expect([0, 3, 6, 9, 12, 30].map(gradientBand)).toEqual([0, 1, 2, 3, 4, 4]);
    expect(GRADIENT_BANDS.map((band) => band.label)).toEqual(["flat", "3%", "6%", "9%", "12%+"]);
    // A chip is named by the gradient its band opens at, so the spoken form is
    // the one that has to carry the span the band actually covers.
    expect(GRADIENT_BANDS.map((band) => band.description)).toEqual([
      "under 3%",
      "3 to 6%",
      "6 to 9%",
      "9 to 12%",
      "12% and steeper",
    ]);
  });
});

describe("gradientRanges", () => {
  it("bands a steady climb as one run from its first metre", () => {
    expect(gradientRanges(ramp(Array(40).fill(10)))).toEqual([
      { band: 3, startIndex: 0, endIndex: 39 },
    ]);
  });

  it("reads a descent as steeply as the climb it mirrors", () => {
    expect(gradientRanges(ramp(Array(40).fill(-10)))).toEqual([
      { band: 3, startIndex: 0, endIndex: 39 },
    ]);
  });

  it("hands the steep ground its own run once the flat ends", () => {
    const ranges = gradientRanges(ramp([...Array(40).fill(0), ...Array(40).fill(10)]));

    // Two runs, not four: the climb passes through the bands between in well
    // under a hundred metres, and a colour that brief is a smear, not a stretch.
    expect(ranges.map((range) => range.band)).toEqual([0, 3]);
    // The gradient is measured backwards, so the climb is reported a little way
    // into itself — the same lag the chart shows for the same stretch.
    const steep = ranges[1]?.startIndex ?? 0;
    expect(steep).toBeGreaterThan(40);
    expect((steep - 40) * FINE_SPACING_METRES).toBeLessThan(100);
  });

  it("collapses a stipple of short runs into the drag it belongs to", () => {
    // Six percent and two, alternating: the look-back averages a little either
    // side of the four percent edge and crosses it at every single segment,
    // which unmerged would draw one steady drag as two colours of dotted line.
    const wobble = Array.from({ length: 60 }, (_, index) => (index % 2 === 0 ? 6 : 2));

    expect(gradientRanges(ramp(wobble))).toEqual([{ band: 1, startIndex: 0, endIndex: 59 }]);
  });

  // A run shorter than the window it was measured over was never sustained by
  // the definition that classified it, so the pair below differ in one thing
  // only: the length of the pitch, either side of that window.
  it("absorbs a pitch shorter than the gradient window", () => {
    const pitch = unevenRoute([
      [0, 100],
      [LONG_STEP, 100],
      [LONG_STEP, 100],
      [SHORT_STEP, 112],
      [LONG_STEP, 112],
      [LONG_STEP, 112],
    ]);

    expect(SHORT_STEP * (POINT_SPACING_METRES / 0.001)).toBeLessThan(GRADIENT_WINDOW_METRES);
    expect(bandsOf(pitch)).toEqual([0]);
  });

  it("keeps a pitch longer than the gradient window", () => {
    const pitch = unevenRoute([
      [0, 100],
      [LONG_STEP, 100],
      [LONG_STEP, 100],
      [LONG_STEP, 128],
      [LONG_STEP, 128],
      [LONG_STEP, 128],
    ]);

    expect(LONG_STEP * (POINT_SPACING_METRES / 0.001)).toBeGreaterThan(GRADIENT_WINDOW_METRES);
    expect(bandsOf(pitch)).toEqual([0, 4]);
  });

  it("classifies the same ground however finely the chart samples it", () => {
    const coordinates = flatThenSteep();
    const offered = bandsOf(coordinates);

    for (const sampleCount of [16, 40, 320, 1000]) {
      const profile = buildProfile(coordinates, sampleCount);
      if (!profile) {
        throw new Error("expected a profile");
      }
      const drawn = new Set(profile.samples.map((sample) => sample.band));

      expect(offered).toEqual(expect.arrayContaining([...drawn]));
    }
  });

  // The floor is a hundred metres and a whole-route chart of a long stage gives
  // each of them about a pixel and a half, so the question is whether a stage
  // full of terrain-model wobble comes out as a stipple of slivers. It does not:
  // the window the gradient is measured over smooths well past its own length,
  // and nothing shorter than a couple of pixels survives it. This is the check
  // that would notice if that stopped being true.
  it("does not shatter a long noisy stage into slivers", () => {
    // Rolling hills under a deterministic ±0.6 m of wobble, twelve metres a
    // point, sixty kilometres of it.
    let seed = 42;
    const wobble = () => {
      seed = (seed * 1103515245 + 12345) % 2 ** 31;

      return (seed / 2 ** 31 - 0.5) * 1.2;
    };
    const coordinates: Position[] = [];
    for (let index = 0; index < 5000; index++) {
      const metres = index * 12;
      const hills = 200 + 60 * Math.sin(metres / 1800) + 25 * Math.sin(metres / 430);
      coordinates.push([8, 49 + index * 0.000108, hills + wobble()]);
    }
    const distances = cumulativeMetres(coordinates);
    const lengths = gradientRanges(coordinates).map(
      (range) => (distances[range.endIndex + 1] ?? 0) - (distances[range.startIndex] ?? 0),
    );

    expect(lengths.length).toBeLessThan(200);
    expect(Math.min(...lengths)).toBeGreaterThanOrEqual(GRADIENT_WINDOW_METRES);
  });

  it("refuses geometry that is not fully surveyed", () => {
    expect(gradientRanges(route([100, undefined, 120]))).toEqual([]);
    expect(gradientRanges(route([100]))).toEqual([]);
  });
});

describe("gradientShares", () => {
  // Flat ground and the climb itself, and nothing between them: the transition
  // is over in one segment, which is the half of the answer that keeps unusable
  // classes out of the key.
  it("offers only the bands the stage actually has, gentlest first", () => {
    expect(bandsOf(flatThenSteep())).toEqual([0, 3]);
  });

  /*
   * Totalled per band, where `gradientMix` leaves one entry per run. A chip says
   * how much of the ride is this steep once, however many times the ride comes
   * back to it — a key that listed the same band three times would be a strip in
   * the wrong order rather than a key.
   */
  it("totals a band the route arrives at more than once", () => {
    const shares = gradientShares(
      ramp([...Array(40).fill(10), ...Array(40).fill(0), ...Array(40).fill(10)]),
    );

    expect(shares.map((entry) => entry.band)).toEqual([0, 3]);
    expect(shares.reduce((total, entry) => total + entry.share, 0)).toBeCloseTo(1, 10);
    // Two of the three stretches are the climb, so it is about two thirds of it.
    expect(shares.find((entry) => entry.band === 3)?.share ?? 0).toBeGreaterThan(0.6);
  });

  // The same fixture the Go MaxGradientPercent test uses: steps of about two
  // hundred metres climbing two metres and then twenty, which the service
  // reports as about ten percent. The two implementations are pinned to one
  // another here, so the summary line and the key cannot come to disagree.
  it("offers the band the reported max gradient falls in", () => {
    const stage = route([100, 102, 122], LONG_STEP);

    expect(bandsOf(stage)).toContain(gradientBand(10));
  });

  // What the key offers is a fact about the ground, so a reader zooming in and
  // out must not watch the chips reshuffle underneath their hand.
  it("offers the same bands whatever stretch the chart is showing", () => {
    const coordinates = flatThenSteep();
    const offered = bandsOf(coordinates);
    const windows = [
      { startMetres: 0, endMetres: 39 * POINT_SPACING_METRES },
      { startMetres: CLIMB_STARTS_METRES - 200, endMetres: CLIMB_STARTS_METRES + 400 },
      { startMetres: CLIMB_STARTS_METRES + 50, endMetres: CLIMB_STARTS_METRES + 200 },
    ];

    for (const window of windows) {
      const profile = buildWindowedProfile(coordinates, window);
      if (!profile) {
        throw new Error("expected a profile");
      }
      const drawn = new Set(profile.samples.map((sample) => sample.band));

      expect(offered).toEqual(expect.arrayContaining([...drawn]));
    }
  });

  it("reports the same band for one pitch at every zoom level", () => {
    const coordinates = flatThenSteep();
    const inTheClimb = CLIMB_STARTS_METRES + 300;
    const whole = buildProfile(coordinates);
    const zoomed = buildWindowedProfile(coordinates, {
      startMetres: inTheClimb - 150,
      endMetres: inTheClimb + 150,
    });
    if (!whole || !zoomed) {
      throw new Error("expected both profiles");
    }

    expect(sampleAt(zoomed, inTheClimb)?.band).toBe(sampleAt(whole, inTheClimb)?.band);
  });
});

describe("gradientMix", () => {
  it("gives each band the share of the route it covers", () => {
    const mix = gradientMix(ramp([...Array(40).fill(0), ...Array(40).fill(10)]));

    expect(mix.map((entry) => entry.band)).toEqual([0, 3]);
    // Every metre of the route is accounted for exactly once: the bar is read
    // as a whole, so a mix that does not sum to one is a bar with a gap in it.
    expect(mix.reduce((total, entry) => total + entry.share, 0)).toBeCloseTo(1, 10);
    expect(mix.every((entry) => entry.share > 0)).toBe(true);
  });

  it("carries a band that appears twice as two shares", () => {
    const mix = gradientMix(
      ramp([...Array(40).fill(10), ...Array(40).fill(0), ...Array(40).fill(10)]),
    );

    expect(mix.map((entry) => entry.band)).toEqual([3, 0, 3]);
  });

  // A listing row asks for this before the geometry has arrived, and a route
  // with one point has no length to divide by.
  it("has nothing to say about a route with no length", () => {
    expect(gradientMix([])).toEqual([]);
    expect(gradientMix([[8, 49, 100]])).toEqual([]);
    expect(
      gradientMix([
        [8, 49, 100],
        [8, 49, 140],
      ]),
    ).toEqual([]);
  });
});
