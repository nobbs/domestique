/**
 * Slices of Open-Meteo's spatial files for one hour, relayed by the service
 * itself rather than read straight from the provider's bucket: the browser
 * only ever reaches this service's own origin for this data, so the
 * Content-Security-Policy needs no new `connect-src` entry for it. The one
 * grant this still needs is `'wasm-unsafe-eval'` in `script-src`, for the
 * reader's own WebAssembly module (`@openmeteo/file-reader`'s dependency on
 * `@openmeteo/file-format-wasm`) — decoding still happens in the browser,
 * only the bytes' origin moved.
 */

import {
  LruBlockCache,
  OmDataType,
  type OmFileReader,
  OmHttpBackendPool,
} from "@openmeteo/file-reader";
import type { Bbox, GridGeometry, ScalarGrid, WindGrid } from "../lib/windGrid";
import { getGetWeatherGridObjectUrl, getWeatherGridLatest } from "./generated";
import { unwrap } from "./request";

// From the layer package's domain table; the .om file carries no georeference.
const GRID = { lonMin: -3.94, latMin: 43.18, dx: 0.02, dy: 0.02, nx: 1215, ny: 746 } as const;
/** Slices are cut on the file's 32-cell chunks, so a small pan re-reads nothing. */
const CHUNK = 32;
/**
 * Reads go through aligned blocks rather than one range per chunk row: a
 * variable is under half a megabyte for the whole domain, and a plain range
 * backend spends twenty seconds on a hundred sequential requests for it.
 */
const BLOCK_BYTES = 256 * 1024;
const BLOCKS_HELD = 64;

// Module-wide on purpose: the cache is keyed by file, so both overlays and
// every hour share it, and a slice read once is never fetched again.
const backends = new OmHttpBackendPool();
const blocks = new LruBlockCache(BLOCK_BYTES, BLOCKS_HELD);

interface Latest {
  reference_time: string;
  valid_times: string[];
}

/**
 * How long a fetched `latest.json` is trusted, matching the model's own
 * publishing package. It changes once a run, not once a pan — without this
 * every overlay's every hour re-fetched it on every slice read.
 */
const LATEST_TTL_MS = 60_000;
let latestCache: { fetchedAt: number; value: Promise<Latest> } | null = null;

function fetchLatest(): Promise<Latest> {
  const now = Date.now();
  if (!latestCache || now - latestCache.fetchedAt >= LATEST_TTL_MS) {
    // domestiqueRequest wraps every generated call in { data, status, headers
    // } — unwrap gets past that envelope to the actual body. Cast beyond
    // that: the operation's success response carries no schema (it is a
    // relay, not this service's own shape), so Orval types the envelope's
    // data as the union of its error responses alone.
    const value = getWeatherGridLatest().then((response) => unwrap(response) as unknown as Latest);
    // Evicted on failure so the next read retries instead of replaying the
    // same rejection for the rest of the minute.
    value.catch(() => {
      latestCache = null;
    });
    latestCache = { fetchedAt: now, value };
  }

  return latestCache.value;
}

/** The relay's own URL for one run's hour, built the same way every other generated operation's is. */
export function omUrl(referenceTime: Date, validTime: string): string {
  return getGetWeatherGridObjectUrl({
    // Normalised to a full timestamp with seconds: Open-Meteo's own
    // valid_times omit them ("2026-09-05T15:00Z"), which Go's RFC3339
    // parser refuses outright rather than defaulting to :00.
    referenceTime: referenceTime.toISOString(),
    validTime: new Date(validTime).toISOString(),
  });
}

/** Whichever of the model's published hours falls closest to `at`. */
export function nearestValidTime(first: string, rest: readonly string[], at: Date): string {
  const wanted = at.getTime();

  return rest.reduce(
    (best, candidate) =>
      Math.abs(Date.parse(candidate) - wanted) < Math.abs(Date.parse(best) - wanted)
        ? candidate
        : best,
    first,
  );
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
    throw new Error(`${name} missing from the model`);
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

/** The named variables for the valid hour nearest `at`, over `bbox`; null when the bbox misses the model. */
async function readSlices(
  bbox: Bbox,
  at: Date,
  names: readonly string[],
): Promise<(GridGeometry & { values: Float32Array[] }) | null> {
  const window = gridWindow(bbox);
  if (window.x1 <= window.x0 || window.y1 <= window.y0) {
    return null;
  }
  const latest = await fetchLatest();
  const [firstValidTime, ...restValidTimes] = latest.valid_times;
  if (!firstValidTime) {
    throw new Error("the model's latest.json has no valid times");
  }
  const url = omUrl(
    new Date(latest.reference_time),
    nearestValidTime(firstValidTime, restValidTimes, at),
  );
  const values = await backends.withReader(url, blocks, (root) =>
    Promise.all(names.map((name) => readSlice(root, name, window))),
  );

  return {
    lonMin: GRID.lonMin + window.x0 * GRID.dx,
    latMin: GRID.latMin + window.y0 * GRID.dy,
    dx: GRID.dx,
    dy: GRID.dy,
    nx: window.x1 - window.x0,
    ny: window.y1 - window.y0,
    values,
  };
}

export async function fetchWindGrid(bbox: Bbox, at: Date): Promise<WindGrid | null> {
  const slices = await readSlices(bbox, at, ["wind_u_component_10m", "wind_v_component_10m"]);
  if (!slices) {
    return null;
  }
  const { values, ...geometry } = slices;
  const [u, v] = values;

  return u && v ? { ...geometry, u, v } : null;
}

/**
 * A reader for one scalar variable, memoised so a hook keyed on it sees the
 * same function each render. Units are the model's: °C, mm in the hour, %.
 */
const readers = new Map<string, (bbox: Bbox, at: Date) => Promise<ScalarGrid | null>>();

export function scalarGridReader(
  variable: string,
): (bbox: Bbox, at: Date) => Promise<ScalarGrid | null> {
  let reader = readers.get(variable);
  if (!reader) {
    reader = async (bbox, at) => {
      const slices = await readSlices(bbox, at, [variable]);
      if (!slices) {
        return null;
      }
      const { values, ...geometry } = slices;
      const [cells] = values;

      return cells ? { ...geometry, values: cells } : null;
    };
    readers.set(variable, reader);
  }

  return reader;
}
