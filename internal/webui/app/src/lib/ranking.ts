/** Ordering the library by what a route measures, for the search palette's `by` order. */

import type { Route } from "../api/types";

/** Which measure the list is ranked by. */
export type SortColumn = "title" | "distance" | "ascent" | "gradient" | "movingTime" | "start";

export type SortDirection = "asc" | "desc";

/**
 * What each measured column reads off a route.
 *
 * Title is absent because it is not a measurement; `sortRoutes` compares it as text.
 *
 * `undefined` is a real answer for moving time — nothing has predicted this
 * stage — rather than a small number, so it is kept out of the comparison
 * rather than coerced to zero.
 */
const MEASURES: Partial<Record<SortColumn, (route: Route) => number | undefined>> = {
  distance: (route) => route.distanceMetres,
  ascent: (route) => route.ascentMetres,
  gradient: (route) => route.maxGradientPercent,
  movingTime: (route) => route.movingSeconds,
};

/**
 * Which way a column reads first when it is picked.
 *
 * A reader sorting by name wants A before Z, and a reader sorting by anything
 * measured is asking which is the longest, the steepest, the hardest — so the
 * numeric columns open descending and only reverse when asked again. Distance
 * to start is the exception: the question there is which is nearest.
 */
export function initialDirection(column: SortColumn): SortDirection {
  return column === "title" || column === "start" ? "asc" : "desc";
}

/**
 * The library in the order the reader asked for.
 *
 * `Array.prototype.sort` is stable, so whatever order `routes` arrives in is the
 * tiebreak: two routes of the same length keep their places, in either direction.
 * That is what lets several keys be applied one after another, least significant
 * first — which is also why no direction is a reversal of the whole list.
 *
 * A route with no predicted moving time sorts last in both directions. It is
 * not the shortest ride in the library; it is one the model has nothing to say
 * about, and burying it under the answers is closer to the truth than ranking
 * it as zero. A start distance not yet known — no position, no geometry — sorts
 * last the same way.
 */
export function sortRoutes(
  routes: Route[],
  sort: SortColumn,
  direction: SortDirection,
  startMetres: (route: Route) => number | undefined = () => undefined,
): Route[] {
  const sign = direction === "desc" ? -1 : 1;
  const measure = sort === "start" ? startMetres : MEASURES[sort];
  if (!measure) {
    return [...routes].sort((left, right) => left.title.localeCompare(right.title) * sign);
  }

  return [...routes].sort((left, right) => {
    const leftValue = measure(left);
    const rightValue = measure(right);
    if (leftValue === undefined || rightValue === undefined) {
      if (leftValue === rightValue) {
        return 0;
      }

      return leftValue === undefined ? 1 : -1;
    }

    return (leftValue - rightValue) * sign;
  });
}
