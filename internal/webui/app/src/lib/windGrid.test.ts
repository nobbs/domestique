import { describe, expect, it } from "vitest";
import type { GridParticle, WindGrid } from "./windGrid";
import {
  advanceGridField,
  GRID_VERTICES_PER_STREAK,
  sampleGrid,
  seedGridField,
  writeGridStreaks,
} from "./windGrid";

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

// Mid-life: `lifeAlpha` fades a streak in for its first quarter and out for
// its last, so a particle at the very start of a fresh life reads as alpha 0
// regardless of how fast it is moving — not the thing these tests are about.
function particleAt(lon: number, lat: number, u: number, v: number): GridParticle {
  return { lon, lat, u, v, ageSeconds: 0.5, lifeSeconds: 1 };
}

/** One vertex's three floats, read out of `into` at the layout this writer uses. */
function vertexAt(into: Float32Array, index: number): [number, number, number] {
  const at = index * 3;

  return [into[at] ?? 0, into[at + 1] ?? 0, into[at + 2] ?? 0];
}

describe("writeGridStreaks", () => {
  it("writes a tail vertex and two symmetric, equally faded head corners", () => {
    const into = new Float32Array(GRID_VERTICES_PER_STREAK * 3);
    const written = writeGridStreaks([particleAt(10, 48, 10, 0)], into, 5, 0.001);

    expect(written).toBe(GRID_VERTICES_PER_STREAK);
    const [tailX, tailY, tailAlpha] = vertexAt(into, 0);
    const [leftX, leftY, leftAlpha] = vertexAt(into, 1);
    const [rightX, rightY, rightAlpha] = vertexAt(into, 2);

    expect(tailAlpha).toBe(0);
    expect(leftAlpha).toBeGreaterThan(0);
    expect(leftAlpha).toBe(rightAlpha);
    // The second triangle repeats the same three points the other way round.
    expect(vertexAt(into, 3)).toEqual([tailX, tailY, tailAlpha]);
    expect(vertexAt(into, 4)).toEqual([rightX, rightY, rightAlpha]);
    expect(vertexAt(into, 5)).toEqual([leftX, leftY, leftAlpha]);
    // Equidistant from the tail and from each other, on opposite sides of it.
    const distanceFromTail = (x: number, y: number) => Math.hypot(x - tailX, y - tailY);

    expect(distanceFromTail(leftX, leftY)).toBeCloseTo(distanceFromTail(rightX, rightY), 6);
    expect(Math.hypot(leftX - rightX, leftY - rightY)).toBeGreaterThan(0);
  });

  it("still draws a dead-calm particle, as a faint point rather than nothing", () => {
    const into = new Float32Array(GRID_VERTICES_PER_STREAK * 3);
    const written = writeGridStreaks([particleAt(10, 48, 0, 0)], into, 5, 0.001);

    expect(written).toBe(GRID_VERTICES_PER_STREAK);
    const [, , alpha] = vertexAt(into, 1);

    expect(alpha).toBeGreaterThan(0);
  });

  it("stops once the buffer has no room for another whole streak", () => {
    const into = new Float32Array(GRID_VERTICES_PER_STREAK * 3);
    const written = writeGridStreaks(
      [particleAt(10, 48, 10, 0), particleAt(10, 48, 10, 0)],
      into,
      5,
      0.001,
    );

    expect(written).toBe(GRID_VERTICES_PER_STREAK);
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
