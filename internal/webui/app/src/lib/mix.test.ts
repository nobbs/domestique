import { describe, expect, it } from "vitest";
import { steepnessEntries } from "./mix";
import type { SignedBandShare } from "./profile";

describe("steepnessEntries", () => {
  it("maps a signed share into climbing and descending metres, summed into metres and share", () => {
    const shares: SignedBandShare[] = [
      { band: 0, climbing: 0.1, descending: 0.15 },
      { band: 3, climbing: 0.6, descending: 0 },
    ];

    const entries = steepnessEntries(shares, 1000);

    expect(entries).toEqual([
      expect.objectContaining({
        highlight: { type: "band", band: 0 },
        label: "flat",
        share: 0.25,
        metres: 250,
        climbingMetres: 100,
        descendingMetres: 150,
      }),
      expect.objectContaining({
        highlight: { type: "band", band: 3 },
        label: "9%",
        share: 0.6,
        metres: 600,
        climbingMetres: 600,
        descendingMetres: 0,
      }),
    ]);
  });

  it("has nothing to show for no shares", () => {
    expect(steepnessEntries([], 1000)).toEqual([]);
  });
});
