/**
 * Arranging the stored library for reading: which stages a search leaves, and
 * what order they are shown in.
 *
 * All of it is done here, in the browser, over the listing the page already
 * holds. The library is one operator's own route collection rather than a
 * catalogue, so there is nothing to page through and nothing to ask the service
 * for — a query parameter carrying a route name would be a new server-side
 * surface, and one that put display names in an access log at that.
 *
 * Only the safe display names are ever matched. Geometry is served by its own
 * endpoint and is not part of the listing, so there is nothing here that could
 * leak it into a search.
 */

import type { Stage } from "../api/types";

/** The orders the library can be read in, in the wire names the controls use. */
export const STAGE_SORTS = ["name", "distance", "ascent"] as const;

export type StageSort = (typeof STAGE_SORTS)[number];

/** The ways the library can be presented, in the wire names the control uses. */
export const LIBRARY_VIEWS = ["grid", "table"] as const;

export type LibraryView = (typeof LIBRARY_VIEWS)[number];

/** What each presentation is called in the control that switches between them. */
export const LIBRARY_VIEW_LABELS: Record<LibraryView, string> = {
  grid: "Grid",
  table: "Table",
};

/** What each order is called, and which way it runs, said in the control. */
export const STAGE_SORT_LABELS: Record<StageSort, string> = {
  name: "Name (A–Z)",
  distance: "Distance (longest first)",
  ascent: "Ascent (most climbing first)",
};

/**
 * The text a stage is matched on: everything a reader can see it called.
 *
 * The stage name is matched separately from the composed title so that a search
 * for a stage's own name finds it without the reader having to remember which
 * route it belongs to, and searching the route name still returns every stage of
 * it.
 */
function haystack(stage: Stage): string {
  return `${stage.title} ${stage.routeName} ${stage.stageName}`;
}

/**
 * Folded for comparison: case and accents removed.
 *
 * A library of German and French route names is full of characters a reader will
 * not reach for while typing quickly, and "Kaiserstuhl" not finding
 * "Kaiserstühl" is the kind of miss that reads as a broken search rather than as
 * a precise one.
 */
function fold(value: string): string {
  return value
    .normalize("NFD")
    .replace(/\p{Diacritic}/gu, "")
    .toLowerCase();
}

/**
 * Whether one stage answers a search.
 *
 * Every whitespace-separated word has to appear somewhere in the stage's names,
 * in any order: "rhine forest" finds the forest stage of the Rhine traverse
 * without the reader reproducing the em dash between them.
 */
export function matchesQuery(stage: Stage, query: string): boolean {
  const words = fold(query).split(/\s+/).filter(Boolean);
  if (words.length === 0) {
    return true;
  }
  const text = fold(haystack(stage));

  return words.every((word) => text.includes(word));
}

/**
 * The comparison one order sorts by, before the tie-breaker.
 *
 * Distance and ascent run largest first, which is what their labels say: the
 * question asked of a library sorted by climbing is which ride is the big one.
 */
function ordering(sort: StageSort, left: Stage, right: Stage): number {
  switch (sort) {
    case "name":
      return left.title.localeCompare(right.title);
    case "distance":
      return right.distanceMetres - left.distanceMetres;
    case "ascent":
      return right.ascentMetres - left.ascentMetres;
  }
}

/**
 * The library in one order, filtered to what a search leaves.
 *
 * The order is total: stages that tie on the chosen figure fall back to the
 * stage's own stable identity, so two rides of exactly forty kilometres do not
 * swap places between renders, and a sort never depends on the order the service
 * happened to list them in.
 */
export function arrangeStages(stages: Stage[], query: string, sort: StageSort): Stage[] {
  return stages
    .filter((stage) => matchesQuery(stage, query))
    .sort(
      (left, right) =>
        ordering(sort, left, right) ||
        left.routeId - right.routeId ||
        left.stageOrder - right.stageOrder,
    );
}

/**
 * How many stages each source route contributes to the library.
 *
 * Counted over the whole listing rather than over what is on show, because a
 * search that leaves one stage of a three-stage route must not make that stage
 * look like a route of its own. The count is what tells a card whether it is one
 * of several, and it is a fact about the library, not about the search.
 */
export function stageCounts(stages: Stage[]): Map<number, number> {
  const counts = new Map<number, number>();
  for (const stage of stages) {
    counts.set(stage.routeId, (counts.get(stage.routeId) ?? 0) + 1);
  }

  return counts;
}
