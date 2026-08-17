import { describe, expect, it } from "vitest";
import type { Position, SurfaceKind, SurfaceRange } from "../api/types";
import { SURFACE_KINDS } from "../api/types";
import {
  SURFACE_STYLES,
  summariseSurface,
  surfaceKindAt,
  surfaceLines,
  swatchBackground,
} from "./surface";

/** Points spaced evenly by latitude, so every stretch is the same length. */
function route(pointCount: number): Position[] {
  return Array.from({ length: pointCount }, (_, index) => [8, 49 + index * 0.001] as Position);
}

function range(kind: SurfaceKind, startIndex: number, endIndex: number): SurfaceRange {
  return { kind, startIndex, endIndex };
}

describe("summariseSurface", () => {
  it("splits the stage so the shares add up to its whole length", () => {
    // Five points, four equal stretches. Points 0–1 are asphalt and own the two
    // stretches leaving them; points 2–4 are gravel and own the other two.
    const summary = summariseSurface(route(5), [range("asphalt", 0, 1), range("gravel", 2, 4)]);

    expect(summary).not.toBeNull();
    const shares = summary?.shares ?? [];
    expect(shares.map((entry) => entry.kind)).toEqual(["asphalt", "gravel"]);
    expect(shares[0]?.share).toBeCloseTo(0.5, 6);
    expect(shares[1]?.share).toBeCloseTo(0.5, 6);
    expect(shares.reduce((total, entry) => total + entry.metres, 0)).toBeCloseTo(
      summary?.totalMetres ?? 0,
      6,
    );
  });

  it("credits a boundary stretch to one side only, never to both", () => {
    const summary = summariseSurface(route(3), [range("asphalt", 0, 0), range("ground", 1, 2)]);
    const [first, second] = summary?.bands ?? [];

    expect(first?.endMetres).toBeCloseTo(second?.startMetres ?? -1, 6);
    expect(second?.endMetres).toBeCloseTo(summary?.totalMetres ?? 0, 6);
  });

  it("places bands contiguously from the start of the stage to its end", () => {
    const summary = summariseSurface(route(7), [
      range("asphalt", 0, 1),
      range("compacted", 2, 4),
      range("paving", 5, 6),
    ]);
    const bands = summary?.bands ?? [];

    expect(bands[0]?.startMetres).toBe(0);
    expect(bands[bands.length - 1]?.endMetres).toBeCloseTo(summary?.totalMetres ?? 0, 6);
    for (let index = 1; index < bands.length; index++) {
      expect(bands[index]?.startMetres).toBeCloseTo(bands[index - 1]?.endMetres ?? -1, 6);
    }
  });

  it("reports the classes in one fixed order however the ranges arrive", () => {
    const summary = summariseSurface(route(5), [
      range("ground", 0, 1),
      range("asphalt", 2, 3),
      range("ground", 4, 4),
    ]);

    // Sealed before unsealed, whatever order the route happens to visit them in,
    // so the legend does not reshuffle itself between stages.
    expect(summary?.shares.map((entry) => entry.kind)).toEqual(["asphalt", "ground"]);
  });

  it("gathers the same class from stretches all over the stage", () => {
    const summary = summariseSurface(route(5), [
      range("gravel", 0, 0),
      range("asphalt", 1, 2),
      range("gravel", 3, 4),
    ]);
    const gravel = summary?.shares.find((entry) => entry.kind === "gravel");

    expect(gravel?.share).toBeCloseTo(0.5, 6);
  });

  it("refuses geometry or ranges it cannot measure", () => {
    expect(summariseSurface(route(5), [])).toBeNull();
    expect(summariseSurface([[8, 49]], [range("asphalt", 0, 0)])).toBeNull();
    expect(
      summariseSurface(
        [
          [8, 49],
          [8, 49],
        ],
        [range("asphalt", 0, 1)],
      ),
    ).toBeNull();
  });

  it("survives indices reaching past the geometry", () => {
    const summary = summariseSurface(route(3), [range("asphalt", 0, 99)]);

    expect(summary?.shares[0]?.share).toBeCloseTo(1, 6);
  });
});

describe("surfaceKindAt", () => {
  const summary = summariseSurface(route(5), [range("asphalt", 0, 1), range("gravel", 2, 4)]);
  if (!summary) {
    throw new Error("expected a summary");
  }

  it("answers with the class under a position along the stage", () => {
    expect(surfaceKindAt(summary, 0)).toBe("asphalt");
    expect(surfaceKindAt(summary, summary.totalMetres * 0.9)).toBe("gravel");
  });

  it("holds the last class past the end rather than blanking the readout", () => {
    expect(surfaceKindAt(summary, summary.totalMetres + 500)).toBe("gravel");
  });
});

describe("surfaceLines", () => {
  it("runs each stretch one point on, so neighbours meet on the shared point", () => {
    const coordinates = route(5);
    const lines = surfaceLines(coordinates, [range("asphalt", 0, 1), range("gravel", 2, 4)]);
    const asphalt = lines.find((entry) => entry.kind === "asphalt")?.lines[0] ?? [];
    const gravel = lines.find((entry) => entry.kind === "gravel")?.lines[0] ?? [];

    expect(asphalt[asphalt.length - 1]).toEqual(gravel[0]);
    expect(gravel[gravel.length - 1]).toEqual(coordinates[4]);
  });

  it("groups every stretch of one class together, in the legend's order", () => {
    const lines = surfaceLines(route(6), [
      range("gravel", 0, 1),
      range("asphalt", 2, 3),
      range("gravel", 4, 5),
    ]);

    expect(lines.map((entry) => entry.kind)).toEqual(["asphalt", "gravel"]);
    expect(lines.find((entry) => entry.kind === "gravel")?.lines).toHaveLength(2);
  });

  it("drops a range covering only the final point, which spans no ground", () => {
    const lines = surfaceLines(route(3), [range("asphalt", 0, 1), range("ground", 2, 2)]);

    expect(lines.map((entry) => entry.kind)).toEqual(["asphalt"]);
  });
});

describe("SURFACE_STYLES", () => {
  it("styles every class the service can report", () => {
    for (const kind of SURFACE_KINDS) {
      expect(SURFACE_STYLES[kind].colour).toMatch(/^#[0-9A-Fa-f]{6}$/);
      expect(SURFACE_STYLES[kind].label).not.toBe("");
    }
  });

  it("gives every class its own colour, so none is mistaken for another", () => {
    const colours = SURFACE_KINDS.map((kind) => SURFACE_STYLES[kind].colour);

    expect(new Set(colours).size).toBe(SURFACE_KINDS.length);
  });
});

describe("swatchBackground", () => {
  it("echoes a dashed line as a dashed swatch, not a flat chip", () => {
    expect(swatchBackground("gravel")).toContain("repeating-linear-gradient");
    expect(swatchBackground("gravel")).toContain(SURFACE_STYLES.gravel.colour);
  });

  it("leaves a solid class solid", () => {
    expect(swatchBackground("asphalt")).toBe(SURFACE_STYLES.asphalt.colour);
  });
});
