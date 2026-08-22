/**
 * Narrowing the stored library to what a search leaves.
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

import type { Route } from "../api/types";
import { providerLabel } from "./provider";

/**
 * The text a route is matched on: everything a reader can see it called.
 *
 * All three names are matched, not just the composed title, so a search finds a
 * route by whichever of its names the reader happens to remember. The source
 * label rides along too, which is the one filter this reuses rather than
 * building a picker of its own: typing "komoot" is how a reader narrows the
 * library to one source.
 */
function haystack(route: Route): string {
  return `${route.title} ${route.routeName} ${route.stageName} ${providerLabel(route.provider)}`;
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
 * Whether one route answers a search.
 *
 * Every whitespace-separated word has to appear somewhere in the route's names,
 * in any order: "rhine forest" finds the forest ride of the Rhine traverse
 * without the reader reproducing the em dash between them.
 */
export function matchesQuery(route: Route, query: string): boolean {
  const words = fold(query).split(/\s+/).filter(Boolean);
  if (words.length === 0) {
    return true;
  }
  const text = fold(haystack(route));

  return words.every((word) => text.includes(word));
}

/**
 * What a search leaves, by name.
 *
 * There is one order and no control to change it. The results column is read by
 * eye, from the top, while the map beside it answers where each route goes; a
 * sort by distance would be a second way to ask a question the figures in each
 * row already answer.
 *
 * The order is total: two routes that share a name fall back to their own stable
 * identity, so nothing swaps places between renders.
 */
export function matchingRoutes(routes: Route[], query: string): Route[] {
  return routes
    .filter((route) => matchesQuery(route, query))
    .sort(
      (left, right) =>
        left.title.localeCompare(right.title) ||
        left.routeId - right.routeId ||
        left.stageOrder - right.stageOrder,
    );
}
