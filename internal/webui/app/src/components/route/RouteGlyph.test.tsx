import { describe, expect, it } from "vitest";
import type { Position } from "../../api/types";
import { glyphPoints } from "./RouteGlyph";

function parse(points: string): Array<[number, number]> {
  return points.split(" ").map((pair) => pair.split(",").map(Number) as [number, number]);
}

describe("glyphPoints", () => {
  it("fits the shape inside the padded viewBox", () => {
    const square: Position[] = [
      [8.0, 49.0],
      [8.1, 49.0],
      [8.1, 49.1],
      [8.0, 49.1],
      [8.0, 49.0],
    ];

    const parsed = parse(glyphPoints(square));
    const xs = parsed.map(([x]) => x);
    const ys = parsed.map(([, y]) => y);

    expect(Math.min(...xs)).toBeGreaterThanOrEqual(0);
    expect(Math.max(...xs)).toBeLessThanOrEqual(48);
    expect(Math.min(...ys)).toBeGreaterThanOrEqual(0);
    expect(Math.max(...ys)).toBeLessThanOrEqual(48);
  });

  it("puts north at the top", () => {
    const northward: Position[] = [
      [8.0, 49.0],
      [8.0, 49.5],
    ];

    const [south, north] = parse(glyphPoints(northward));

    // SVG y grows downwards, so the northern point must have the smaller y.
    expect(north?.[1]).toBeLessThan(south?.[1] ?? 0);
  });

  it("preserves aspect ratio rather than stretching to fill", () => {
    // A shape twice as wide as it is tall must stay twice as wide.
    const wide: Position[] = [
      [8.0, 49.0],
      [8.2, 49.0],
      [8.2, 49.05],
      [8.0, 49.05],
      [8.0, 49.0],
    ];

    const parsed = parse(glyphPoints(wide));
    const width = Math.max(...parsed.map(([x]) => x)) - Math.min(...parsed.map(([x]) => x));
    const height = Math.max(...parsed.map(([, y]) => y)) - Math.min(...parsed.map(([, y]) => y));

    expect(width).toBeGreaterThan(height);
  });

  it("returns nothing for geometry too short to draw", () => {
    expect(glyphPoints([])).toBe("");
    expect(glyphPoints([[8, 49]])).toBe("");
  });

  it("does not produce NaN for a degenerate repeated point", () => {
    const points = glyphPoints([
      [8, 49],
      [8, 49],
    ]);

    expect(points).not.toContain("NaN");
  });
});
