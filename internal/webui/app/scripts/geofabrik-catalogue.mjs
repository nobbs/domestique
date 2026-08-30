/**
 * Regenerates the bundled Geofabrik catalogue the region picker offers.
 *
 * Run by hand — `node scripts/geofabrik-catalogue.mjs` — and commit what it
 * writes. Geofabrik's tree changes a few times a year, and a build-time fetch
 * would put the settings page behind someone else's uptime.
 *
 * The slug is derived from the published `.osm.pbf` URL rather than from the
 * `id` field, because the URL path is literally what the service concatenates
 * (`osmindex.extractURL`); deriving it any other way would let the catalogue
 * offer a region the builder then cannot fetch.
 *
 * Sizes come from a one-byte range request. A plain HEAD is answered with a 302
 * whose Content-Length describes the redirect, not the extract.
 */

import { writeFile } from "node:fs/promises";
import process from "node:process";

const BASE = "https://download.geofabrik.de/";
const INDEX = `${BASE}index-v1-nogeom.json`;
const SUFFIX = "-latest.osm.pbf";

/** The country this setting exists for, and the only subtree that is measured. */
const GERMANY_SLUG = "europe/germany";

/** Matches `runtimeconfig.regionSlug`, so the catalogue cannot offer a slug the service refuses. */
const SLUG = /^[a-z0-9]+(?:-[a-z0-9]+)*(?:\/[a-z0-9]+(?:-[a-z0-9]+)*)*$/;

/**
 * How many size probes are in flight at once. Low on purpose: a burst of
 * ranged requests is what the retries below exist to survive, and keeping the
 * burst small is what stops them being needed.
 */
const PROBES = 3;

/**
 * Which regions are measured. Sizing all five hundred means five hundred ranged
 * requests, which takes tens of minutes from here and is refused often enough to
 * need retrying; sizing Germany takes seconds. The tree and the names still
 * cover the world — they come free with the index — so anywhere else is still
 * selectable, it just carries no size until this is widened.
 *
 * Widen by returning true for more of the catalogue: nothing downstream cares
 * which regions have a size, only whether one does.
 */
function worthSizing(slug) {
  return slug === GERMANY_SLUG || slug.startsWith(`${GERMANY_SLUG}/`);
}

const OUT = new URL("../src/features/settings/regions/", import.meta.url);

async function json(url) {
  const response = await fetch(url);
  if (!response.ok) {
    throw new Error(`${url}: ${response.status} ${response.statusText}`);
  }

  return response.json();
}

/** How long one size probe may wait for headers before it is abandoned. */
const PROBE_TIMEOUT_MS = 20_000;
const PROBE_ATTEMPTS = 6;
const PROBE_BACKOFF_MS = 300;

const sleep = (ms) => new Promise((resolve) => setTimeout(resolve, ms));

/**
 * The size of one extract, taken from the total in the `Content-Range` of a
 * one-byte ranged GET. The index publishes no sizes and a HEAD is answered with
 * a redirect whose Content-Length describes the redirect, so this is the cheap
 * way to ask: one byte to measure a file of several gigabytes.
 *
 * Retried on anything that is not an answer with a total in it. A refusal here
 * arrives as a well-formed 502 rather than as a thrown error — no size and a
 * transport that dropped the request look identical to the caller, so both are
 * tried again rather than quietly recorded as "no size published".
 */
async function extractBytes(slug) {
  for (let attempt = 1; attempt <= PROBE_ATTEMPTS; attempt++) {
    try {
      const response = await fetch(`${BASE}${slug}${SUFFIX}`, {
        headers: { Range: "bytes=0-0" },
        signal: AbortSignal.timeout(PROBE_TIMEOUT_MS),
      });
      await response.body?.cancel();
      const total = response.headers.get("content-range")?.split("/")[1];
      if (total && /^\d+$/.test(total)) {
        return Number(total);
      }
      if (response.status === 404) {
        console.warn(`no extract published for ${slug}`);

        return null;
      }
    } catch {
      // A timeout or a dropped socket, retried the same way a refusal is.
    }
    await sleep(PROBE_BACKOFF_MS * attempt);
  }
  console.warn(`no size for ${slug} after ${PROBE_ATTEMPTS} attempts`);

  return null;
}

/** Runs `work` over `items` a few at a time, keeping the results in order. */
async function pooled(items, limit, work) {
  const results = new Array(items.length);
  let next = 0;
  const workers = Array.from({ length: limit }, async () => {
    while (next < items.length) {
      const index = next++;
      results[index] = await work(items[index], index);
    }
  });
  await Promise.all(workers);

  return results;
}

/** A size in the unit it is worth reading at, for the progress line only. */
function readable(bytes) {
  return bytes >= 1024 ** 3
    ? `${(bytes / 1024 ** 3).toFixed(1)} GB`
    : `${Math.round(bytes / 1024 ** 2)} MB`;
}

function slugOf(properties) {
  const url = properties.urls?.pbf;

  return url?.startsWith(BASE) && url.endsWith(SUFFIX)
    ? url.slice(BASE.length, -SUFFIX.length)
    : null;
}

const index = await json(INDEX);
const regions = [];
for (const feature of index.features) {
  const properties = feature.properties;
  const slug = slugOf(properties);
  if (!slug) {
    console.warn(`skipping ${properties.id}: no extract under ${BASE}`);
    continue;
  }
  if (!SLUG.test(slug)) {
    console.warn(`skipping ${properties.id}: ${slug} is not a slug the service accepts`);
    continue;
  }
  regions.push({ slug, name: properties.name, parent: properties.parent ?? null });
}
regions.sort((a, b) => a.slug.localeCompare(b.slug));

const measured = regions.filter((region) => worthSizing(region.slug));
console.error(`probing ${measured.length} of ${regions.length} extract sizes`);

let done = 0;
const probed = new Map();
await pooled(measured, PROBES, async (region) => {
  const bytes = await extractBytes(region.slug);
  done += 1;
  // Reported as each one lands rather than at the end: a probe against a host
  // that refuses half its connections is slow enough that a silent run is
  // indistinguishable from a hung one.
  console.error(
    `[${done}/${measured.length}] ${region.slug} ${bytes === null ? "no size" : readable(bytes)}`,
  );
  probed.set(region.slug, bytes);
});

const unsized = measured.filter((region) => probed.get(region.slug) === null);

const entries = regions.map((region) => ({
  slug: region.slug,
  name: region.name,
  parent: region.parent,
  depth: region.slug.split("/").length,
  bytes: probed.get(region.slug) ?? null,
}));

const germany = entries.filter(
  (entry) => entry.slug === GERMANY_SLUG || entry.parent === "germany",
);
if (germany.length !== 17) {
  throw new Error(`expected Germany and its 16 states, found ${germany.length}`);
}

await writeFile(
  new URL("catalogue.generated.ts", OUT),
  `/**
 * The Geofabrik regions the picker offers, and what each one costs to fetch.
 *
 * Generated by \`scripts/geofabrik-catalogue.mjs\` — do not edit. Sizes are the
 * extracts as published on the day it was run, so they drift slowly; they are
 * shown as a scale, not a promise.
 */

export interface CatalogueRegion {
  /** The path the service turns into a download URL, such as "europe/germany/bayern". */
  slug: string;
  name: string;
  /** The Geofabrik id of the region this sits inside, or null for a top-level one. */
  parent: string | null;
  /** How many segments the slug has, so a tree can indent without splitting it again. */
  depth: number;
  /** The published extract's size, or null where the host did not answer with one. */
  bytes: number | null;
}

export const CATALOGUE: readonly CatalogueRegion[] = ${JSON.stringify(entries, null, 2)};

/** Germany's own extract. The picker leads with it and its states. */
export const GERMANY = "europe/germany";
`,
);

console.error(
  `wrote ${entries.length} regions, ${measured.length - unsized.length} of them sized`,
);
process.exitCode = 0;
