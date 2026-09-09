import { describe, expect, it } from "vitest";
import type { ActivityTrackWorld } from "../api/types";
import { worldPoint, worldPolyline } from "./zwiftWorld";

const WORLD: ActivityTrackWorld = {
  id: 9,
  name: "Makuri Islands",
  mapUrl: "/v1/zwift/worlds/9/map",
  bounds: { north: -10.73746, west: 165.76591, south: -10.85234, east: 165.88222 },
};

const SIZE = { width: 1000, height: 800 };

describe("worldPoint", () => {
  it("puts the north-west corner at the image's origin", () => {
    expect(worldPoint(WORLD, SIZE, [WORLD.bounds.west, WORLD.bounds.north])).toEqual({
      x: 0,
      y: 0,
    });
  });

  it("puts the south-east corner at the far edge", () => {
    const point = worldPoint(WORLD, SIZE, [WORLD.bounds.east, WORLD.bounds.south]);

    expect(point.x).toBeCloseTo(1000, 6);
    expect(point.y).toBeCloseTo(800, 6);
  });

  it("puts the centre in the middle", () => {
    const { north, west, south, east } = WORLD.bounds;
    const point = worldPoint(WORLD, SIZE, [(west + east) / 2, (north + south) / 2]);

    expect(point.x).toBeCloseTo(500, 6);
    expect(point.y).toBeCloseTo(400, 6);
  });

  it("places everything at the origin for a world with no extent", () => {
    const flat: ActivityTrackWorld = {
      ...WORLD,
      bounds: { north: 1, west: 2, south: 1, east: 2 },
    };

    expect(worldPoint(flat, SIZE, [2, 1])).toEqual({ x: 0, y: 0 });
  });
});

describe("worldPolyline", () => {
  it("writes one point per coordinate, in order", () => {
    const { north, west, south, east } = WORLD.bounds;

    expect(
      worldPolyline(WORLD, SIZE, [
        [west, north],
        [east, south],
      ]),
    ).toBe("0.0,0.0 1000.0,800.0");
  });

  it("is empty for a track with no coordinates", () => {
    expect(worldPolyline(WORLD, SIZE, [])).toBe("");
  });
});
