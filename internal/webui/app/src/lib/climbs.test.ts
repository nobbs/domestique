import { describe, expect, it } from "vitest";
import type { Position } from "../api/types";
import { findClimbs } from "./climbs";

/** One ten-thousandth of a degree of latitude is 11.119 metres. */
const FINE_SPACING_METRES = 11.119;

/**
 * A route whose segments run at the given gradients, in percent.
 *
 * Points sit eleven metres apart, so a run of ten segments covers about the
 * hundred-metre window a climb is measured and merged over — the same
 * spacing `profile.test.ts` uses for the same reason.
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

/** `count` segments at a steady `percent` gradient. */
function steady(percent: number, count: number): number[] {
  return Array.from({ length: count }, () => percent);
}

describe("findClimbs", () => {
  it("finds nothing on flat ground", () => {
    expect(findClimbs(ramp(steady(0, 20)))).toEqual([]);
  });

  it("finds nothing on a descent, however steep", () => {
    expect(findClimbs(ramp(steady(-12, 20)))).toEqual([]);
  });

  it("reports a steady climb held over the window", () => {
    const climbs = findClimbs(ramp(steady(10, 20)));

    expect(climbs).toHaveLength(1);
    expect(climbs[0]?.startMetres).toBeCloseTo(0, 0);
    expect(climbs[0]?.averageGradePercent).toBeCloseTo(10, 0);
    expect(climbs[0]?.maxGradePercent).toBeCloseTo(10, 0);
    expect(climbs[0]?.ascentMetres).toBeGreaterThan(0);
  });

  // Three segments at five percent lift the ground under a metre — never
  // enough, even smeared across the whole look-back window, to reach the
  // gradient a climb opens at. The window itself is what absorbs a bump this
  // brief; nothing here has to notice its length on purpose.
  it("finds nothing in a bump too slight to reach a climbing gradient", () => {
    const coordinates = ramp([...steady(0, 10), ...steady(5, 3), ...steady(0, 10)]);

    expect(findClimbs(coordinates)).toEqual([]);
  });

  it("holds at a gradient just over the one that opens the first non-flat band", () => {
    expect(findClimbs(ramp(steady(3.1, 20)))).toHaveLength(1);
  });

  it("refuses the same run just under that gradient", () => {
    expect(findClimbs(ramp(steady(2.9, 20)))).toEqual([]);
  });

  it("merges two climbs a short flat gap apart into one", () => {
    // ~56 m of flat between two climbs, under the 100 m floor that separates them.
    const coordinates = ramp([...steady(10, 15), ...steady(0, 5), ...steady(10, 15)]);

    const climbs = findClimbs(coordinates);

    expect(climbs).toHaveLength(1);
    expect(climbs[0]?.endMetres).toBeGreaterThan(climbs[0]?.startMetres ?? 0);
  });

  it("keeps two climbs apart across a flat long enough to matter", () => {
    // ~145 m of flat between two climbs, over the 100 m floor.
    const coordinates = ramp([...steady(10, 15), ...steady(0, 13), ...steady(10, 15)]);

    expect(findClimbs(coordinates)).toHaveLength(2);
  });

  it("refuses geometry with no elevation", () => {
    expect(
      findClimbs([
        [8, 49],
        [8, 49.001],
      ]),
    ).toEqual([]);
  });

  it("refuses geometry with only some elevation", () => {
    expect(
      findClimbs([
        [8, 49, 100],
        [8, 49.001],
        [8, 49.002, 120],
      ]),
    ).toEqual([]);
  });

  it("refuses a single point", () => {
    expect(findClimbs([[8, 49, 100]])).toEqual([]);
  });

  it("refuses geometry with elevation but no length", () => {
    expect(
      findClimbs([
        [8, 49, 100],
        [8, 49, 140],
      ]),
    ).toEqual([]);
  });
});
