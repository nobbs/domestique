/**
 * The 10 m wind grid for one hour, read straight from Open-Meteo's spatial
 * files on S3 with range requests. Nothing here goes through the service; the
 * bucket has to be named in the Content-Security-Policy's `connect-src`.
 */

import { OmDataType, OmFileReader, OmHttpBackend } from "@openmeteo/file-reader";
import type { Bbox, WindGrid } from "../lib/windGrid";

const BUCKET = "https://openmeteo.s3.amazonaws.com/data_spatial";
const MODEL = "dwd_icon_d2";
// From the layer package's domain table; the .om file carries no georeference.
const GRID = { lonMin: -3.94, latMin: 43.18, dx: 0.02, dy: 0.02, nx: 1215, ny: 746 } as const;
/** Slices are cut on the file's 32-cell chunks, so a small pan re-reads nothing. */
const CHUNK = 32;

interface Latest {
  reference_time: string;
  valid_times: string[];
}

function pad(value: number): string {
  return String(value).padStart(2, "0");
}

/** `2026-09-05T15:00Z` → the object key that hour is stored under. */
function omUrl(referenceTime: Date, validTime: string): string {
  const stamp = validTime.replace(/:(\d\d)Z$/, "$1");
  const dir = [
    referenceTime.getUTCFullYear(),
    pad(referenceTime.getUTCMonth() + 1),
    pad(referenceTime.getUTCDate()),
    `${pad(referenceTime.getUTCHours())}00Z`,
  ].join("/");

  return `${BUCKET}/${MODEL}/${dir}/${stamp}.om`;
}

/** The slice of the model grid covering `bbox`, aligned to chunk edges. */
export function gridWindow(bbox: Bbox): { x0: number; x1: number; y0: number; y1: number } {
  const down = (value: number, max: number) =>
    Math.min(Math.max(Math.floor(value / CHUNK) * CHUNK, 0), max);
  const up = (value: number, max: number) =>
    Math.min(Math.max(Math.ceil(value / CHUNK) * CHUNK, 0), max);

  return {
    x0: down((bbox[0] - GRID.lonMin) / GRID.dx, GRID.nx),
    x1: up((bbox[2] - GRID.lonMin) / GRID.dx, GRID.nx),
    y0: down((bbox[1] - GRID.latMin) / GRID.dy, GRID.ny),
    y1: up((bbox[3] - GRID.latMin) / GRID.dy, GRID.ny),
  };
}

async function readSlice(
  root: OmFileReader,
  name: string,
  window: ReturnType<typeof gridWindow>,
): Promise<Float32Array> {
  const variable = await root.getChildByName(name);
  if (!variable) {
    throw new Error(`${name} missing from ${MODEL}`);
  }
  try {
    return await variable.read({
      type: OmDataType.FloatArray,
      ranges: [
        { start: window.y0, end: window.y1 },
        { start: window.x0, end: window.x1 },
      ],
    });
  } finally {
    variable.dispose();
  }
}

/** The grid for the valid hour nearest `at`, over `bbox`; null when the bbox misses the model. */
export async function fetchWindGrid(bbox: Bbox, at: Date): Promise<WindGrid | null> {
  const window = gridWindow(bbox);
  if (window.x1 <= window.x0 || window.y1 <= window.y0) {
    return null;
  }
  const latest = (await (await fetch(`${BUCKET}/${MODEL}/latest.json`)).json()) as Latest;
  const wanted = at.getTime();
  const validTime = latest.valid_times.reduce((best, candidate) =>
    Math.abs(Date.parse(candidate) - wanted) < Math.abs(Date.parse(best) - wanted)
      ? candidate
      : best,
  );
  const root = await OmFileReader.create(
    new OmHttpBackend({ url: omUrl(new Date(latest.reference_time), validTime) }),
  );
  try {
    const [u, v] = await Promise.all([
      readSlice(root, "wind_u_component_10m", window),
      readSlice(root, "wind_v_component_10m", window),
    ]);

    return {
      lonMin: GRID.lonMin + window.x0 * GRID.dx,
      latMin: GRID.latMin + window.y0 * GRID.dy,
      dx: GRID.dx,
      dy: GRID.dy,
      nx: window.x1 - window.x0,
      ny: window.y1 - window.y0,
      u,
      v,
    };
  } finally {
    root.dispose();
  }
}
