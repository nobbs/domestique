/**
 * A gridded 10 m wind over a patch of the map, and particles adrift in it.
 *
 * The corridor field (`windField.ts`) drifts along the route's own forecast;
 * this one drifts through a model grid read straight from Open-Meteo's spatial
 * files, so the air moves over the whole viewport, not just beside the road.
 * Particles are held as longitude and latitude and advected by the u/v
 * components in metres per second, which is what the grid stores.
 *
 * Pure: no DOM, no fetching, no WebGL.
 */

import type { Position } from "../api/types";
import { haversineMetres } from "./profile";
import {
  FLOATS_PER_VERTEX,
  lifeAlpha,
  mercatorXY,
  PARTICLE_LIFE_SECONDS,
  STREAK_WIDTH_PIXELS,
  writeWedge,
} from "./windField";

const METRES_PER_DEGREE_LATITUDE = haversineMetres([0, 0], [0, 1]);

/**
 * Speed is drawn in screen space, the way Windy does it: a 5 m/s wind carries a
 * streak this many pixels a second whatever the zoom, so a zoomed-out map is
 * not a still one and a zoomed-in map is not a blur.
 */
const PIXELS_PER_SECOND_PER_METRE_PER_SECOND = 6;

/** How long a tail is, in pixels per m/s of wind, and the least any streak gets. */
const TAIL_PIXELS_PER_METRE_PER_SECOND = 2.5;
const TAIL_MIN_PIXELS = 3;

/**
 * A streak is a quad, two triangles, tapering from nothing at the tail to its
 * full width at the head: a hairline is lost on a dense display, and this
 * overlay is the thing being looked at rather than a texture beside it.
 */
export const GRID_VERTICES_PER_STREAK = 6;

/** Calm air is drawn fainter, never invisible. */
const CALM_ALPHA_FLOOR = 0.35;
const CALM_METRES_PER_SECOND = 8;

/** A regular lon/lat grid: cell `[iy][ix]` is centred at `lonMin + ix*dx`, `latMin + iy*dy`. */
export interface GridGeometry {
  lonMin: number;
  latMin: number;
  dx: number;
  dy: number;
  nx: number;
  ny: number;
}

/** One row-major `[ny][nx]` field over the grid. */
export interface ScalarGrid extends GridGeometry {
  values: Float32Array;
}

/** Row-major `[ny][nx]` u and v components over a regular lon/lat grid. */
export interface WindGrid extends GridGeometry {
  /** Eastward component, m/s, positive toward east. */
  u: Float32Array;
  /** Northward component, m/s, positive toward north. */
  v: Float32Array;
}

/** West, south, east, north in degrees. */
export type Bbox = readonly [number, number, number, number];

export interface GridParticle {
  lon: number;
  lat: number;
  ageSeconds: number;
  lifeSeconds: number;
  u: number;
  v: number;
}

function bilinear(values: Float32Array, grid: GridGeometry, fx: number, fy: number): number {
  const x0 = Math.floor(fx);
  const y0 = Math.floor(fy);
  const x1 = Math.min(x0 + 1, grid.nx - 1);
  const y1 = Math.min(y0 + 1, grid.ny - 1);
  const tx = fx - x0;
  const ty = fy - y0;
  const at = (x: number, y: number) => values[y * grid.nx + x] ?? 0;

  return (
    (1 - ty) * ((1 - tx) * at(x0, y0) + tx * at(x1, y0)) +
    ty * ((1 - tx) * at(x0, y1) + tx * at(x1, y1))
  );
}

/** The wind at a position, bilinear in the grid; null off the grid or where a cell is missing. */
export function sampleGrid(grid: WindGrid, lon: number, lat: number): [number, number] | null {
  const fx = (lon - grid.lonMin) / grid.dx;
  const fy = (lat - grid.latMin) / grid.dy;
  if (fx < 0 || fy < 0 || fx > grid.nx - 1 || fy > grid.ny - 1) {
    return null;
  }
  const u = bilinear(grid.u, grid, fx, fy);
  const v = bilinear(grid.v, grid, fx, fy);

  return Number.isFinite(u) && Number.isFinite(v) ? [u, v] : null;
}

export function respawnGridParticle(
  particle: GridParticle,
  bbox: Bbox,
  random: () => number,
): void {
  particle.lon = bbox[0] + random() * (bbox[2] - bbox[0]);
  particle.lat = bbox[1] + random() * (bbox[3] - bbox[1]);
  particle.ageSeconds = 0;
  particle.lifeSeconds = PARTICLE_LIFE_SECONDS * (0.6 + 0.8 * random());
  particle.u = 0;
  particle.v = 0;
}

export function seedGridField(
  bbox: Bbox,
  count: number,
  random: () => number = Math.random,
): GridParticle[] {
  return Array.from({ length: Math.max(count, 0) }, () => {
    const particle: GridParticle = { lon: 0, lat: 0, ageSeconds: 0, lifeSeconds: 1, u: 0, v: 0 };
    respawnGridParticle(particle, bbox, random);
    particle.ageSeconds = random() * particle.lifeSeconds;

    return particle;
  });
}

function outside(particle: GridParticle, bbox: Bbox): boolean {
  return (
    particle.lon < bbox[0] ||
    particle.lon > bbox[2] ||
    particle.lat < bbox[1] ||
    particle.lat > bbox[3]
  );
}

export function advanceGridField(
  particles: GridParticle[],
  grid: WindGrid,
  bbox: Bbox,
  seconds: number,
  metresPerPixel: number,
  random: () => number = Math.random,
): void {
  const metres = seconds * PIXELS_PER_SECOND_PER_METRE_PER_SECOND * metresPerPixel;
  for (const particle of particles) {
    particle.ageSeconds += seconds;
    const wind = sampleGrid(grid, particle.lon, particle.lat);
    if (wind) {
      [particle.u, particle.v] = wind;
      const cosLat = Math.cos((particle.lat * Math.PI) / 180) || 1;
      particle.lon += (particle.u * metres) / (METRES_PER_DEGREE_LATITUDE * cosLat);
      particle.lat += (particle.v * metres) / METRES_PER_DEGREE_LATITUDE;
    }
    if (particle.ageSeconds >= particle.lifeSeconds || outside(particle, bbox) || !wind) {
      respawnGridParticle(particle, bbox, random);
    }
  }
}

/**
 * Writes each particle as a tapered quad, six vertices, for `windStreakLayer`
 * drawing triangles; returns how many vertices were written.
 *
 * `mercatorPerPixel` is the world square's own size of one screen pixel, which
 * is what the quad's width has to be measured in.
 */
export function writeGridStreaks(
  particles: GridParticle[],
  into: Float32Array,
  metresPerPixel: number,
  mercatorPerPixel: number,
): number {
  let written = 0;
  const halfWidth = (STREAK_WIDTH_PIXELS / 2) * mercatorPerPixel;
  for (const particle of particles) {
    if (written + GRID_VERTICES_PER_STREAK > into.length / FLOATS_PER_VERTEX) {
      break;
    }
    const speed = Math.hypot(particle.u, particle.v);
    // Direction is undefined at exactly zero, not the streak itself: dead calm
    // is a real reading, and skipping it here would contradict the alpha floor
    // below, which already draws it faint rather than invisible. North stands
    // in for a heading that does not exist.
    const [unitEast, unitNorth] = speed > 0 ? [particle.u / speed, particle.v / speed] : [0, 1];
    const tailMetres =
      Math.max(TAIL_MIN_PIXELS, speed * TAIL_PIXELS_PER_METRE_PER_SECOND) * metresPerPixel;
    const cosLat = Math.cos((particle.lat * Math.PI) / 180) || 1;
    const tail: Position = [
      particle.lon - (unitEast * tailMetres) / (METRES_PER_DEGREE_LATITUDE * cosLat),
      particle.lat - (unitNorth * tailMetres) / METRES_PER_DEGREE_LATITUDE,
    ];
    const alpha =
      lifeAlpha(particle) *
      Math.min(1, CALM_ALPHA_FLOOR + (1 - CALM_ALPHA_FLOOR) * (speed / CALM_METRES_PER_SECOND));
    const [tailX, tailY] = mercatorXY(tail);
    const [headX, headY] = mercatorXY([particle.lon, particle.lat]);
    writeWedge(into, written * FLOATS_PER_VERTEX, tailX, tailY, headX, headY, alpha, halfWidth);
    written += GRID_VERTICES_PER_STREAK;
  }

  return written;
}
