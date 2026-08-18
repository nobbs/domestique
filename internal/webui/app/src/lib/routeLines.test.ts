import { describe, expect, it } from "vitest";
import type { Position } from "../api/types";
import { gradientRanges, gradientSlices, routeLinesWithin } from "./routeLines";

/** Points spaced evenly by latitude, so every stretch is the same length. */
function route(pointCount: number): Position[] {
  return Array.from({ length: pointCount }, (_, index) => [8, 49 + index * 0.001] as Position);
}

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

describe("routeLinesWithin", () => {
  it("splits the whole route the way it splits one class", () => {
    const coordinates = route(5);
    const slices = routeLinesWithin(coordinates, [{ startIndex: 1, endIndex: 3 }]);

    expect(slices.inside[0]).toEqual(coordinates.slice(1, 4));
    expect(slices.outside).toHaveLength(2);
  });

  it("holds the whole route inside when nothing has been asked of it", () => {
    expect(routeLinesWithin(route(5), null).inside[0]).toHaveLength(5);
  });

  it("lights several scattered stretches at once, as a picked class is", () => {
    const coordinates = route(7);
    const slices = routeLinesWithin(coordinates, [
      { startIndex: 0, endIndex: 2 },
      { startIndex: 4, endIndex: 6 },
    ]);

    expect(slices.inside).toEqual([coordinates.slice(0, 3), coordinates.slice(4, 7)]);
    expect(slices.outside).toEqual([coordinates.slice(2, 5)]);
  });
});

describe("gradientRanges", () => {
  it("bands a steady climb as one run from its first metre", () => {
    expect(gradientRanges(ramp(Array(40).fill(10)))).toEqual([
      { band: 2, startIndex: 0, endIndex: 39 },
    ]);
  });

  it("reads a descent as steeply as the climb it mirrors", () => {
    expect(gradientRanges(ramp(Array(40).fill(-10)))).toEqual([
      { band: 2, startIndex: 0, endIndex: 39 },
    ]);
  });

  it("hands the steep ground its own run once the flat ends", () => {
    const ranges = gradientRanges(ramp([...Array(40).fill(0), ...Array(40).fill(10)]));

    // Two runs, not three: the climb passes through the middle band in well
    // under fifty metres, and a colour that brief is a smear, not a stretch.
    expect(ranges.map((range) => range.band)).toEqual([0, 2]);
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

  it("refuses geometry that is not fully surveyed", () => {
    const partial: Position[] = [
      [8, 49, 100],
      [8, 49.001],
      [8, 49.002, 120],
    ];

    expect(gradientRanges(partial)).toEqual([]);
    expect(gradientRanges([[8, 49, 100]])).toEqual([]);
  });
});

describe("gradientSlices", () => {
  const steps = ramp([...Array(40).fill(0), ...Array(40).fill(6), ...Array(40).fill(14)]);

  it("groups the route by band, gentlest first", () => {
    expect(gradientSlices(steps, null).map((slices) => slices.band)).toEqual([0, 1, 3]);
  });

  it("runs each band one point on, so neighbours meet on the shared point", () => {
    const slices = gradientSlices(steps, null);
    const gentle = slices[0]?.inside[0] ?? [];
    const middle = slices[1]?.inside[0] ?? [];

    expect(gentle[gentle.length - 1]).toEqual(middle[0]);
  });

  it("separates what a window is showing from what it is not", () => {
    const slices = gradientSlices(steps, [{ startIndex: 0, endIndex: 20 }]);
    const gentle = slices.find((entry) => entry.band === 0);
    const steep = slices.find((entry) => entry.band === 3);

    expect(gentle?.inside).not.toHaveLength(0);
    expect(gentle?.outside).not.toHaveLength(0);
    // The climb lies wholly beyond the window, so none of it is being shown.
    expect(steep?.inside).toHaveLength(0);
    expect(steep?.outside).not.toHaveLength(0);
  });

  it("has nothing to paint for a route with no elevation", () => {
    expect(gradientSlices(route(5), null)).toEqual([]);
  });
});
