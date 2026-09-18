import { describe, expect, it } from "vitest";
import type { SurfaceSummary } from "./surface";
import { climbingVerdict, surfaceVerdict } from "./verdict";

describe("climbingVerdict", () => {
  it.each([
    [400, 50_000, "Flat", "good"],
    [750, 50_000, "Rolling", "info"],
    [1_250, 50_000, "Hilly", "hold"],
    [2_730, 49_000, "Mountainous", "alert"],
  ])("reads %d m over %d m", (ascent, distance, label, tone) => {
    expect(climbingVerdict(ascent, distance)).toEqual({ label, tone });
  });

  it("says nothing about a route with no length", () => {
    expect(climbingVerdict(100, 0)).toBeNull();
  });
});

function summary(
  shares: Array<[SurfaceSummary["shares"][number]["kind"], number]>,
): SurfaceSummary {
  const total = shares.reduce((sum, [, metres]) => sum + metres, 0);
  return {
    bands: [],
    shares: shares.map(([kind, metres]) => ({ kind, metres, share: metres / total })),
    totalMetres: total,
  };
}

describe("surfaceVerdict", () => {
  it("calls a route with a trace of gravel sealed", () => {
    expect(
      surfaceVerdict(
        summary([
          ["asphalt", 9_800],
          ["gravel", 200],
        ]),
      ),
    ).toMatchObject({
      title: "Sealed surface",
      label: "Sealed",
      tone: "good",
      unsealedMetres: 200,
    });
  });

  it("counts compacted, gravel and ground as unsealed and ignores unsurveyed ground", () => {
    const verdict = surfaceVerdict(
      summary([
        ["asphalt", 6_000],
        ["compacted", 1_000],
        ["gravel", 2_000],
        ["ground", 1_000],
        ["unknown", 5_000],
      ]),
    );

    expect(verdict).toMatchObject({ title: "Mixed surface", label: "40% unsealed", tone: "hold" });
    expect(verdict?.unsealedMetres).toBe(4_000);
  });

  it("calls a mostly loose route unsealed", () => {
    expect(
      surfaceVerdict(
        summary([
          ["asphalt", 2_000],
          ["gravel", 8_000],
        ]),
      ),
    ).toMatchObject({
      title: "Unsealed surface",
      label: "80% unsealed",
      tone: "alert",
    });
  });

  it("says nothing without a survey", () => {
    expect(surfaceVerdict(null)).toBeNull();
    expect(surfaceVerdict(summary([["unknown", 5_000]]))).toBeNull();
  });
});
