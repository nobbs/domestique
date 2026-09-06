/**
 * A scalar grid painted as an image the map can drape over the ground.
 *
 * MapLibre places an image by its four corners and stretches it linearly in
 * Web Mercator, while the model grid is regular in latitude. Over a few
 * degrees the two disagree by a cell or more, so the rows are resampled here:
 * each output row is the latitude Mercator puts there, not the grid's.
 *
 * Pure apart from the pixel buffer it fills; the canvas belongs to the caller.
 */

import { mercatorXY } from "./windField";
import type { ScalarGrid } from "./windGrid";

/** Red, green, blue, alpha, 0..255. */
export type Rgba = readonly [number, number, number, number];

/** `[[west, north], [east, north], [east, south], [west, south]]`, as an image source wants. */
export type Corners = [[number, number], [number, number], [number, number], [number, number]];

/** The cell-edge bounds of a grid whose values sit at cell centres. */
export function gridCorners(grid: ScalarGrid): Corners {
  const west = grid.lonMin - grid.dx / 2;
  const east = grid.lonMin + (grid.nx - 0.5) * grid.dx;
  const south = grid.latMin - grid.dy / 2;
  const north = grid.latMin + (grid.ny - 0.5) * grid.dy;

  return [
    [west, north],
    [east, north],
    [east, south],
    [west, south],
  ];
}

/** Latitude back from the Mercator y `mercatorXY` produces. */
function latitudeOf(mercatorY: number): number {
  return (Math.atan(Math.exp(Math.PI * (1 - 2 * mercatorY))) * 360) / Math.PI - 90;
}

/**
 * Fills `into` (RGBA, `grid.nx` wide by `rows` high, top row north) from the
 * grid, nearest cell per pixel, with rows spaced evenly in Mercator.
 */
export function paintGrid(
  grid: ScalarGrid,
  rows: number,
  colour: (value: number) => Rgba,
  into: Uint8ClampedArray,
): void {
  const corners = gridCorners(grid);
  const [, top] = mercatorXY(corners[0]);
  const [, bottom] = mercatorXY(corners[3]);
  for (let row = 0; row < rows; row++) {
    const latitude = latitudeOf(top + ((row + 0.5) / rows) * (bottom - top));
    const iy = Math.min(Math.max(Math.round((latitude - grid.latMin) / grid.dy), 0), grid.ny - 1);
    for (let ix = 0; ix < grid.nx; ix++) {
      const value = grid.values[iy * grid.nx + ix];
      const [r, g, b, a] =
        value === undefined || Number.isNaN(value) ? [0, 0, 0, 0] : colour(value);
      const at = (row * grid.nx + ix) * 4;
      into[at] = r;
      into[at + 1] = g;
      into[at + 2] = b;
      into[at + 3] = a;
    }
  }
}
