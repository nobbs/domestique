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

/** The country this setting exists for; leads the picker and seeds the sanity check below. */
const GERMANY_SLUG = "europe/germany";

/** Matches `runtimeconfig.regionSlug`, so the catalogue cannot offer a slug the service refuses. */
const SLUG = /^[a-z0-9]+(?:-[a-z0-9]+)*(?:\/[a-z0-9]+(?:-[a-z0-9]+)*)*$/;

/**
 * How many listing pages are read at once. One: the host refuses a burst far
 * more readily than a queue, and thirty-three sequential pages is a couple of
 * minutes either way. Concurrency here buys nothing and costs retries.
 */
const PAGES_AT_ONCE = 1;



const OUT = new URL("../src/features/settings/regions/", import.meta.url);

async function json(url) {
  const response = await fetch(url);
  if (!response.ok) {
    throw new Error(`${url}: ${response.status} ${response.statusText}`);
  }

  return response.json();
}

/** How long one listing page may take before it is abandoned. */
const PAGE_TIMEOUT_MS = 20_000;
const PAGE_ATTEMPTS = 25;
const PAGE_BACKOFF_MS = 700;
const PAGE_BACKOFF_CAP_MS = 5_000;

const sleep = (ms) => new Promise((resolve) => setTimeout(resolve, ms));

/**
 * One of Geofabrik's own listing pages, retried.
 *
 * A refusal arrives as a well-formed 5xx rather than as a thrown error, so no
 * page and a transport that dropped the request look identical to the caller
 * and both are tried again. A 404 is neither: it is the host saying there is no
 * such page, which is worth believing the first time.
 *
 * Returns the markup, `{ missing }` for a page that does not exist, or null
 * when it could not be reached at all — only the last of those fails the run.
 */
async function page(url) {
  for (let attempt = 1; attempt <= PAGE_ATTEMPTS; attempt++) {
    try {
      const response = await fetch(url, { signal: AbortSignal.timeout(PAGE_TIMEOUT_MS) });
      if (response.ok) {
        return { html: await response.text() };
      }
      await response.body?.cancel();
      // A page that is not there is an answer. Only a refusal or a dropped
      // connection is worth asking again, and only those should fail the run.
      if (response.status === 404) {
        return { missing: true };
      }
    } catch {
      // A timeout or a dropped socket, retried the way a refusal is.
    }
    // Capped, and jittered so a run that hits a rate limit does not keep
    // arriving in step with it.
    const backoff = Math.min(PAGE_BACKOFF_MS * attempt, PAGE_BACKOFF_CAP_MS);
    await sleep(backoff / 2 + Math.floor(Math.random() * backoff));
  }
  console.warn(`could not read ${url} after ${PAGE_ATTEMPTS} attempts`);

  return null;
}

const UNITS = { B: 1, kB: 1024, MB: 1024 ** 2, GB: 1024 ** 3 };

/**
 * Every extract size a listing page states, as slug to bytes.
 *
 * Geofabrik prints the size beside each download link — "(810&nbsp;MB)" — so a
 * parent's page prices all of its children at once. That is the whole reason
 * this reads pages rather than the extracts themselves: sizing 555 regions by
 * asking each file how big it is takes 555 requests and the better part of an
 * hour, and every one of them is a chance to be refused. Thirty-three pages
 * price the same 555 regions.
 *
 * The figures are the ones Geofabrik chose to display, so they are rounded —
 * 810 MB for an extract of 810.6. A picker compares orders of magnitude, and
 * nothing downstream does arithmetic that a megabyte would change.
 */
/**
 * Every region name a listing page states, as slug to name.
 *
 * Read separately from the sizes, and never allowed to cost one: the index
 * names fifty-two regions after their own path — "us/wyoming" rather than
 * "Wyoming" — and the page beside them has the name a reader would recognise.
 * A page that yields no name simply leaves the index's own in place.
 */
function namesOn(html, pageUrl) {
  const names = new Map();
  // Only the cell that names a subregion. Every page also carries navigation —
  // "[one level up]" and the like — which an unanchored link pattern happily
  // reads as a region's name.
  const link = /<td class="subregion"><a href="([^"]+)\.html">([^<]+)<\/a>/g;
  for (const [, href, name] of html.matchAll(link)) {
    const path = new URL(`${href}.html`, pageUrl).pathname.replace(/^\//, "").slice(0, -".html".length);
    names.set(path, plainName(name));
  }

  return names;
}

function sizesOn(html, pageUrl) {
  const sizes = new Map();
  const row = /href="([^"]+)-latest\.osm\.pbf"[\s\S]{0,400}?\(([\d.]+)(?:&nbsp;|\s)([kMG]?B)\)/g;
  for (const [, href, amount, unit] of html.matchAll(row)) {
    const path = new URL(`${href}-latest.osm.pbf`, pageUrl).pathname.replace(/^\//, "");
    const slug = path.slice(0, -SUFFIX.length);
    const scale = UNITS[unit];
    if (scale) {
      sizes.set(slug, Math.round(Number(amount) * scale));
    }
  }

  return sizes;
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

/**
 * A region name with Geofabrik's own markup taken out. Twenty-one of them carry
 * a `<br />` between a local name and its English gloss — the index is written
 * for a web page. React escapes it rather than obeying it, so left alone it
 * shows as literal angle brackets in the picker.
 */
function plainName(name) {
  return name
    .replace(/<[^>]*>/g, " ")
    .replace(/\s+/g, " ")
    .trim();
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
  regions.push({ slug, name: plainName(properties.name), parent: properties.parent ?? null });
}
regions.sort((a, b) => a.slug.localeCompare(b.slug));

// A region's size is printed on the page above it, so this is one page per
// parent rather than one request per region. Which page that is has two
// candidate answers and needs both, because each is wrong where the other is
// right: the index files every US state under `north-america` rather than under
// `us`, so its `parent` misses us.html; and Greater London's boroughs are
// published under a `london/` path whose page is called greater-london.html, so
// the slug path asks for a london.html that does not exist. Asking for both is
// cheap — a page that is not there answers 404 and is skipped.
const slugById = new Map(index.features.map((f) => [f.properties.id, slugOf(f.properties)]));
const byParent = new Map(
  index.features.filter((f) => slugOf(f.properties)).map((f) => [slugOf(f.properties), f.properties.parent]),
);
const pages = [
  ...new Set(
    regions.flatMap((region) => {
      const at = region.slug.lastIndexOf("/");
      const fromPath = at === -1 ? `${BASE}index.html` : `${BASE}${region.slug.slice(0, at)}.html`;
      const parent = byParent.get(region.slug);
      const fromParent = parent ? `${BASE}${slugById.get(parent)}.html` : `${BASE}index.html`;

      return [fromPath, fromParent];
    }),
  ),
];

console.error(`reading ${pages.length} listing pages for ${regions.length} extract sizes`);
const probed = new Map();
const stated = new Map();
let done = 0;
let readPages = 0;
let unreachable = 0;
await pooled(pages, PAGES_AT_ONCE, async (url) => {
  const result = await page(url);
  done += 1;
  if (result === null) {
    unreachable += 1;
  } else {
    readPages += 1;
  }
  const html = result?.html ?? null;
  if (html) {
    for (const [slug, name] of namesOn(html, url)) {
      stated.set(slug, name);
    }
  }
  const found = html ? sizesOn(html, url) : new Map();
  for (const [slug, bytes] of found) {
    probed.set(slug, bytes);
  }
  // Reported as each page lands: a run against a host that refuses half its
  // connections is slow enough that silence is indistinguishable from a hang.
  console.error(
    `[${done}/${pages.length}] ${url.slice(BASE.length)} — ${result?.missing ? "no such page" : `${found.size} sizes`}`,
  );
});

if (unreachable > 0) {
  throw new Error(
    `${unreachable} of ${pages.length} listing pages could not be reached, which would leave ` +
      "regions unsized for no better reason than the transport. Nothing written; run it again.",
  );
}

// What is left after every page was read is genuinely unstated rather than
// missed: Geofabrik prints no size for a handful of its regions.
const unsized = regions.filter((region) => !probed.has(region.slug));
for (const region of unsized) {
  console.warn(`no size stated for ${region.slug}`);
}

const entries = regions.map((region) => ({
  slug: region.slug,
  // The page's name where it has one, since the index names some regions after
  // their own path.
  name: stated.get(region.slug) ?? region.name,
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
  `wrote ${entries.length} regions, ${entries.length - unsized.length} of them sized`,
);
process.exitCode = 0;
