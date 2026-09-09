/**
 * Where a virtual world's coordinates land on that world's map artwork.
 *
 * Zwift publishes each world's image with the corners it spans, and the
 * projection between the two is linear in degrees — the images are small and
 * the worlds a few kilometres across, so nothing here is a map projection.
 */

import type { ActivityTrackWorld, Position } from "../api/types";

/** A point in the artwork's own pixels, with (0, 0) at its top-left corner. */
export interface WorldPoint {
  x: number;
  y: number;
}

/** The artwork's pixel size, read from the loaded image. */
export interface ImageSize {
  width: number;
  height: number;
}

/**
 * One coordinate placed on the artwork. A world with no extent in either
 * direction would divide by zero, so it places everything at the origin.
 */
export function worldPoint(
  world: ActivityTrackWorld,
  size: ImageSize,
  [longitude, latitude]: Position,
): WorldPoint {
  const { north, west, south, east } = world.bounds;
  const spanX = east - west;
  const spanY = north - south;

  return {
    x: spanX === 0 ? 0 : ((longitude - west) / spanX) * size.width,
    y: spanY === 0 ? 0 : ((north - latitude) / spanY) * size.height,
  };
}

/** The whole track as an SVG polyline's `points`, in the artwork's pixels. */
export function worldPolyline(
  world: ActivityTrackWorld,
  size: ImageSize,
  coordinates: Position[],
): string {
  return coordinates
    .map((coordinate) => {
      const { x, y } = worldPoint(world, size, coordinate);

      return `${x.toFixed(1)},${y.toFixed(1)}`;
    })
    .join(" ");
}
