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
  return `${route.title} ${route.sourceRouteName} ${route.routeName} ${providerLabel(route.provider)}`;
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
  return matchesText(haystack(route), query);
}

/** The same test over any name: every word of the query, accents folded, in any order. */
export function matchesText(name: string, query: string): boolean {
  const words = fold(query).split(/\s+/).filter(Boolean);
  const text = fold(name);

  return words.every((word) => text.includes(word));
}

/**
 * What a search leaves, by name.
 *
 * The search palette ranks what this returns rather than ordering the library
 * itself; see `lib/ranking.ts`, which relies on the order below being total and
 * on `sort` being stable to inherit it as a tiebreak.
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
        left.sourceRouteId - right.sourceRouteId ||
        left.stageOrder - right.stageOrder,
    );
}

/** The page a route is read on. */
export function routePath(route: Pick<Route, "provider" | "sourceRouteId" | "stageOrder">): string {
  return `/routes/${encodeURIComponent(route.provider)}/${route.sourceRouteId}/${route.stageOrder}`;
}

/**
 * The route a `routeKey` names, or null for anything that is not one.
 *
 * Still read for the `/?route=` links the entry page handed out before a route
 * had a page of its own. The two-part form predates a second provider and means
 * VeloPlanner, as the Go handler assumes for the same paths.
 */
export function parseRouteKey(
  value: string | null,
): { provider: string; sourceRouteId: number; stageOrder: number } | null {
  const parts = (value ?? "").split("/");
  const [provider, left, right] = parts.length === 2 ? ["veloplanner", ...parts] : parts;
  if (
    parts.length > 3 ||
    !provider ||
    !left ||
    !right ||
    !/^\d+$/.test(left) ||
    !/^\d+$/.test(right)
  ) {
    return null;
  }
  const sourceRouteId = Number.parseInt(left, 10);
  const stageOrder = Number.parseInt(right, 10);

  return sourceRouteId > 0 && stageOrder > 0 ? { provider, sourceRouteId, stageOrder } : null;
}
