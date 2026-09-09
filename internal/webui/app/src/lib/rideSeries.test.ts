/**
 * Laying a recorded series onto the profile's axis. The two are indexed
 * differently on purpose, and getting this wrong puts a heart rate under the
 * wrong climb rather than failing loudly.
 */

import { describe, expect, it } from "vitest";
import type { Position } from "../api/types";
import { buildActivityProfile } from "./profile";
import { alignSeries } from "./rideSeries";

/** A straight run east, every sample carrying an altitude. */
function coordinates(): Position[] {
  return [
    [8.4, 49, 100],
    [8.5, 49, 120],
    [8.6, 49, 140],
    [8.7, 49, 160],
  ];
}

describe("alignSeries", () => {
  it("puts the first and last readings at the ends of the profile", () => {
    const profile = buildActivityProfile(coordinates(), 8);
    if (!profile) {
      throw new Error("the fixture carries altitudes throughout");
    }

    const aligned = alignSeries([10, 20, 30, 40], coordinates(), profile);

    expect(aligned).toHaveLength(profile.samples.length);
    expect(aligned[0]).toBe(10);
    expect(aligned.at(-1)).toBe(40);
  });

  // A gap belongs to the recording. Interpolating over one would draw a reading
  // across ground the sensor said nothing about.
  it("keeps a gap the sensor left as a gap", () => {
    const profile = buildActivityProfile(coordinates(), 4);
    if (!profile) {
      throw new Error("the fixture carries altitudes throughout");
    }

    const aligned = alignSeries([null, null, null, null], coordinates(), profile);

    expect(aligned.every((value) => value === null)).toBe(true);
  });

  // The profile drops the samples that carried no altitude, so its axis starts
  // where the altitudes do — and the series must follow that axis, not its own
  // index.
  it("follows the profile's axis when the profile starts late", () => {
    const late: Position[] = [
      [8.4, 49],
      [8.5, 49],
      [8.6, 49, 140],
      [8.7, 49, 160],
    ];
    const profile = buildActivityProfile(late, 4);
    if (!profile) {
      throw new Error("two altitudes are enough for a profile");
    }

    const aligned = alignSeries([1, 2, 3, 4], late, profile);

    expect(aligned[0]).toBe(3);
    expect(aligned.at(-1)).toBe(4);
  });
  // The reason this averages at all: a ride records once a second and the
  // profile holds a few hundred samples, so one coasted second used to draw a
  // spike to zero across ground the rider pedalled.
  it("averages the readings over the ground a sample stands for", () => {
    const dense: Position[] = Array.from({ length: 40 }, (_, index) => [
      8.4 + index * 0.01,
      49,
      100 + index,
    ]);
    const profile = buildActivityProfile(dense, 4);
    if (!profile) {
      throw new Error("the fixture carries altitudes throughout");
    }
    // Ninety, but for the one coasted second the last sample sits on — which a
    // nearest-neighbour pick would have drawn as a cadence of zero.
    const cadence = dense.map((_, index) => (index === dense.length - 1 ? 0 : 90));

    const aligned = alignSeries(cadence, dense, profile);

    expect(aligned.at(-1)).toBeGreaterThan(70);
  });

  // A stretch that recorded nothing but gaps stays a gap; the mean must not
  // reach into a neighbouring stretch to fill it.
  it("keeps a stretch of gaps as a gap while its neighbours draw", () => {
    const dense: Position[] = Array.from({ length: 20 }, (_, index) => [
      8.4 + index * 0.01,
      49,
      100 + index,
    ]);
    const profile = buildActivityProfile(dense, 4);
    if (!profile) {
      throw new Error("the fixture carries altitudes throughout");
    }
    const readings = dense.map((_, index) => (index < 10 ? 100 : null));

    const aligned = alignSeries(readings, dense, profile);

    expect(aligned[0]).toBe(100);
    expect(aligned.at(-1)).toBeNull();
  });

  // The profile's axis begins at the first altitude, but the records before it
  // still carry sensors. Averaging those into the opening sample would draw
  // readings from ground the chart does not show.
  it("leaves the records before the first altitude out of the opening sample", () => {
    const warmUp: Position[] = Array.from({ length: 20 }, (_, index) =>
      // The altimeter needs a moment; everything before it is off the axis.
      index < 3 ? [8.4 + index * 0.01, 49] : [8.4 + index * 0.01, 49, 100 + index],
    );
    const profile = buildActivityProfile(warmUp, 4);
    if (!profile) {
      throw new Error("seventeen altitudes are enough for a profile");
    }
    const readings = warmUp.map((_, index) => (index < 3 ? 1_000 : 80));

    const aligned = alignSeries(readings, warmUp, profile);

    expect(aligned[0]).toBe(80);
  });
});
