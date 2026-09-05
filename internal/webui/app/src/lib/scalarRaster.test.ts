import { describe, expect, it } from "vitest";
import { gridCorners, paintGrid } from "./scalarRaster";
import type { ScalarGrid } from "./windGrid";

const grid: ScalarGrid = {
  lonMin: 10,
  latMin: 48,
  dx: 1,
  dy: 1,
  nx: 2,
  ny: 2,
  // South row cold, north row warm.
  values: new Float32Array([0, 0, 30, 30]),
};

describe("gridCorners", () => {
  it("bounds the cells by their edges, not their centres", () => {
    expect(gridCorners(grid)).toEqual([
      [9.5, 49.5],
      [11.5, 49.5],
      [11.5, 47.5],
      [9.5, 47.5],
    ]);
  });
});

describe("paintGrid", () => {
  it("puts the north row at the top and paints every pixel", () => {
    const into = new Uint8ClampedArray(2 * 4 * 4);
    paintGrid(grid, 4, (value) => (value > 15 ? [255, 0, 0, 255] : [0, 0, 255, 255]), into);
    expect(Array.from(into.slice(0, 4))).toEqual([255, 0, 0, 255]);
    expect(Array.from(into.slice(into.length - 4))).toEqual([0, 0, 255, 255]);
  });
});
