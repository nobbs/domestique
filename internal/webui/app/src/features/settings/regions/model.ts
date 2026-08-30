/**
 * What a set of chosen Geofabrik regions means, apart from how it was chosen.
 *
 * Every picker variant renders the same derived facts from here — what is
 * selected, what it costs, and what is wrong with it — so the variants differ
 * only in the interaction and can be compared on that.
 *
 * The service accepts any well-formed slug, catalogue or not
 * (`runtimeconfig.ValidateSurface`), so nothing here removes a value it does not
 * recognise. A hand-typed region survives being opened in a picker.
 */

import { CATALOGUE, type CatalogueRegion, GERMANY } from "./catalogue.generated";

export type { CatalogueRegion };
export { GERMANY };

const BY_SLUG = new Map(CATALOGUE.map((region) => [region.slug, region]));

/** "europe/germany/bayern" — a country's states, but not their sub-districts. */
const GERMAN_STATE_DEPTH = 3;

const BYTES_PER_GIB = 1024 ** 3;
const BYTES_PER_MIB = 1024 ** 2;

export function region(slug: string): CatalogueRegion | undefined {
  return BY_SLUG.get(slug);
}

/**
 * Whether one region's extract already contains another's. Geofabrik nests its
 * extracts along the slug path, so "europe/germany" holds every
 * "europe/germany/…" and the prefix is the whole test.
 */
export function covers(outer: string, inner: string): boolean {
  return inner.startsWith(`${outer}/`);
}

/**
 * The selected regions that another selected region already contains. Naming
 * both a country and one of its states is the mistake a free-text list invites:
 * the state's data is fetched and indexed twice, for nothing.
 */
export function redundant(slugs: readonly string[]): string[] {
  return slugs.filter((slug) => slugs.some((other) => covers(other, slug)));
}

/** The selected regions the catalogue does not know, which may simply be stale. */
export function unknown(slugs: readonly string[]): string[] {
  return slugs.filter((slug) => !BY_SLUG.has(slug));
}

/** What one rebuild downloads: every extract, every time its checksum has moved. */
export function transferBytes(slugs: readonly string[]): number {
  return slugs.reduce((total, slug) => total + (BY_SLUG.get(slug)?.bytes ?? 0), 0);
}

/**
 * What one rebuild needs on disk at its peak. A build stages one extract, packs
 * it, and deletes it before starting the next (`osmindex.addRegion`), so this is
 * the largest single region rather than the sum.
 */
export function peakStagingBytes(slugs: readonly string[]): number {
  return slugs.reduce((peak, slug) => Math.max(peak, BY_SLUG.get(slug)?.bytes ?? 0), 0);
}

/** A size in the unit it is worth comparing at, or a dash where none is published. */
export function formatBytes(bytes: number | null): string {
  if (bytes === null) {
    return "size unknown";
  }
  if (bytes === 0) {
    return "0 MB";
  }

  return bytes >= BYTES_PER_GIB
    ? `${(bytes / BYTES_PER_GIB).toFixed(1)} GB`
    : `${Math.max(1, Math.round(bytes / BYTES_PER_MIB))} MB`;
}

/**
 * Adds a region and drops whatever it makes redundant, so a selection cannot
 * hold a country and its own states at once however it was clicked together.
 */
export function select(slugs: readonly string[], slug: string): string[] {
  if (slugs.includes(slug)) {
    return [...slugs];
  }
  const kept = slugs.filter((held) => !covers(slug, held) && !covers(held, slug));

  return [...kept, slug].sort((a, b) => a.localeCompare(b));
}

export function deselect(slugs: readonly string[], slug: string): string[] {
  return slugs.filter((held) => held !== slug);
}

export function toggle(slugs: readonly string[], slug: string): string[] {
  return slugs.includes(slug) ? deselect(slugs, slug) : select(slugs, slug);
}

/**
 * Catalogue entries matching what has been typed, by name or by slug, with
 * Germany first while nothing has been. The setting exists here to index
 * Germany; the rest of the world is a search away rather than in the way.
 */
export function search(query: string, limit = 40): CatalogueRegion[] {
  const wanted = query.trim().toLowerCase();
  if (!wanted) {
    // The country and its sixteen states, but not the Regierungsbezirke below
    // them: four of Baden-Württemberg's otherwise sort above Bayern and push
    // the states themselves out of sight. They are still a search away.
    return CATALOGUE.filter(
      (entry) => entry.slug.startsWith(GERMANY) && entry.depth <= GERMAN_STATE_DEPTH,
    ).slice(0, limit);
  }

  return CATALOGUE.filter(
    (entry) => entry.slug.includes(wanted) || entry.name.toLowerCase().includes(wanted),
  ).slice(0, limit);
}

/** Germany and its 16 states, in the order the catalogue holds them. */
export const GERMAN_STATES: readonly CatalogueRegion[] = CATALOGUE.filter(
  (entry) => entry.parent === "germany",
);

/**
 * The regions nested directly inside one slug, or the top level for null. The
 * tree is read off the slug path rather than Geofabrik's parent ids, since the
 * path is what nesting means to the service: a region whose own parent path is
 * not published sits at the top level rather than disappearing.
 */
export function childrenOf(slug: string | null): CatalogueRegion[] {
  return CATALOGUE.filter((entry) => {
    const at = entry.slug.lastIndexOf("/");
    const above = at === -1 ? null : entry.slug.slice(0, at);

    return (above !== null && BY_SLUG.has(above) ? above : null) === slug;
  });
}
