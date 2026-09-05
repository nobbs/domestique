import { describe, expect, it } from "vitest";
import type { WindGrid } from "./windGrid";
import { advanceGridField, sampleGrid, seedGridField } from "./windGrid";

const grid: WindGrid = {
  lonMin: 10,
  latMin: 48,
  dx: 1,
  dy: 1,
  nx: 2,
  ny: 2,
  u: new Float32Array([0, 10, 0, 10]),
  v: new Float32Array([5, 5, 5, 5]),
};

describe("sampleGrid", () => {
  it("interpolates between columns and refuses points off the grid", () => {
    expect(sampleGrid(grid, 10.5, 48.5)).toEqual([5, 5]);
    expect(sampleGrid(grid, 9, 48)).toBeNull();
  });
});

describe("advanceGridField", () => {
  it("carries a particle east and north with the wind", () => {
    const random = () => 0.5;
    const [particle] = seedGridField([10, 48, 11, 49], 1, random);
    if (!particle) {
      throw new Error("no particle");
    }
    const before = { lon: particle.lon, lat: particle.lat };
    advanceGridField([particle], grid, [0, 0, 20, 60], 0.01, 10, random);
    expect(particle.lon).toBeGreaterThan(before.lon);
    expect(particle.lat).toBeGreaterThan(before.lat);
  });
});
