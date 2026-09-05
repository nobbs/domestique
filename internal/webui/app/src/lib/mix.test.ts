import { describe, expect, it } from "vitest";
import { groundSegments, steepnessEntries } from "./mix";
import type { SignedBandShare } from "./profile";
import type { SurfaceSummary } from "./surface";

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

describe("groundSegments", () => {
  /** Asphalt 0–6000, gravel 6000–10000: a window straddles the boundary. */
  function surface(): SurfaceSummary {
    return {
      bands: [
        { kind: "asphalt", startMetres: 0, endMetres: 6_000 },
        { kind: "gravel", startMetres: 6_000, endMetres: 10_000 },
      ],
      shares: [
        { kind: "asphalt", metres: 6_000, share: 0.6 },
        { kind: "gravel", metres: 4_000, share: 0.4 },
      ],
      totalMetres: 10_000,
    };
  }

  it("shares the whole route when no window is given, matching today's behaviour", () => {
    const segments = groundSegments(surface());

    expect(segments).toEqual([
      expect.objectContaining({ share: 0.6, highlight: { type: "surface", kind: "asphalt" } }),
      expect.objectContaining({ share: 0.4, highlight: { type: "surface", kind: "gravel" } }),
    ]);
  });

  it("shares only the window, clipping a band that straddles its edge", () => {
    const segments = groundSegments(surface(), { startMetres: 4_000, endMetres: 8_000 });

    // asphalt: 4000-6000 (2000 of the 4000-wide window), gravel: 6000-8000 (2000).
    expect(segments).toEqual([
      expect.objectContaining({ share: 0.5, highlight: { type: "surface", kind: "asphalt" } }),
      expect.objectContaining({ share: 0.5, highlight: { type: "surface", kind: "gravel" } }),
    ]);
    expect(segments.reduce((sum, entry) => sum + entry.share, 0)).toBeCloseTo(1);
  });

  it("has nothing to show for an empty or inverted window", () => {
    expect(groundSegments(surface(), { startMetres: 8_000, endMetres: 8_000 })).toEqual([]);
    expect(groundSegments(surface(), { startMetres: 8_000, endMetres: 4_000 })).toEqual([]);
  });

  it("has nothing to show for no surface", () => {
    expect(groundSegments(null, { startMetres: 0, endMetres: 100 })).toEqual([]);
  });
});
