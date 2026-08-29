/**
 * Ordering the library by what a route measures, and keeping that order in the
 * address.
 *
 * This is the catalogue's whole difference from the atlas. The atlas has one
 * fixed order and argues against a control to change it — see `library.ts` —
 * because a column beside a map is read from the top while the map answers
 * where each route goes. The catalogue has no map to answer that, and ranking
 * the library by a number is the reason it exists, so here the order is the
 * reader's to choose.
 *
 * The choice lives in the query string rather than in component state because
 * opening a route leaves this page for the atlas: without it, coming back would
 * land on an unsorted, unsearched table every time. A sorted and narrowed
 * catalogue is also the one view on this UI worth sending to somebody.
 */

import type { Route, SurfaceKind } from "../api/types";
import { SURFACE_KINDS } from "../api/types";
import type { LibraryFilters, NumericRange } from "./filters";
import { EMPTY_FILTERS } from "./filters";

/** Which measure the table is ranked by. */
export type SortColumn = "title" | "distance" | "ascent" | "gradient" | "movingTime";

export type SortDirection = "asc" | "desc";

export interface CatalogueView {
  query: string;
  sort: SortColumn;
  direction: SortDirection;
  filters: LibraryFilters;
}

/**
 * The measures the table ranks by, in the order it draws them.
 *
 * Two names each, because they are read in two places. `label` is the measure
 * spelled out, which is what a sentence about the order needs — "ranked by
 * moving time" rather than "ranked by time". `short` is what fits in a header
 * cell that holds two controls side by side. Both live here so the control and
 * the sentence describing it cannot drift apart.
 */
export const SORT_COLUMNS: ReadonlyArray<{
  readonly column: SortColumn;
  readonly label: string;
  readonly short: string;
}> = [
  { column: "title", label: "Route", short: "Route" },
  { column: "distance", label: "Distance", short: "Distance" },
  { column: "ascent", label: "Climbing", short: "Ascent" },
  { column: "gradient", label: "Max gradient", short: "Max" },
  { column: "movingTime", label: "Moving time", short: "Time" },
];

/** One measure's names, or undefined for a column that is not ranked by. */
export function sortColumn(column: SortColumn) {
  return SORT_COLUMNS.find((entry) => entry.column === column);
}

/**
 * What each measured column reads off a route.
 *
 * Title is absent because it is not a measurement: it is the order the library
 * already arrives in, which `sortRoutes` reverses rather than recomputes.
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

const SORT_COLUMN_NAMES = new Set<string>(SORT_COLUMNS.map((entry) => entry.column));

/**
 * Which way a column reads first when it is picked.
 *
 * A reader sorting by name wants A before Z, and a reader sorting by anything
 * measured is asking which is the longest, the steepest, the hardest — so the
 * numeric columns open descending and only reverse when asked again.
 */
export function initialDirection(column: SortColumn): SortDirection {
  return column === "title" ? "asc" : "desc";
}

export const DEFAULT_VIEW: CatalogueView = {
  query: "",
  sort: "title",
  direction: "asc",
  filters: EMPTY_FILTERS,
};

/**
 * The library in the order the reader asked for.
 *
 * `routes` arrives in `matchingRoutes`' total order — title, then the route's
 * own identity — and `Array.prototype.sort` is stable, so a single-key
 * comparator here inherits that as its tiebreak for free: two routes of the
 * same length stay in alphabetical order, and reversing the direction does not
 * shuffle them against each other.
 *
 * A route with no predicted moving time sorts last in both directions. It is
 * not the shortest ride in the library; it is one the model has nothing to say
 * about, and burying it under the answers is closer to the truth than ranking
 * it as zero.
 */
export function sortRoutes(routes: Route[], sort: SortColumn, direction: SortDirection): Route[] {
  const measure = MEASURES[sort];
  // The name column is the order the library already came in, so descending is
  // that order backwards rather than a comparison of its own.
  if (!measure) {
    return direction === "desc" ? [...routes].reverse() : [...routes];
  }
  const sign = direction === "desc" ? -1 : 1;

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

function isSortColumn(value: string | null): value is SortColumn {
  return value !== null && SORT_COLUMN_NAMES.has(value);
}

function isSurfaceKind(value: string): value is SurfaceKind {
  return (SURFACE_KINDS as readonly string[]).includes(value);
}

/** A bound, or null for anything that is not a finite number. */
function readBound(value: string | null): number | null {
  if (value === null || value.trim() === "") {
    return null;
  }
  const parsed = Number(value);

  return Number.isFinite(parsed) ? parsed : null;
}

function readRange(params: URLSearchParams, prefix: string): NumericRange {
  return {
    min: readBound(params.get(`${prefix}Min`)),
    max: readBound(params.get(`${prefix}Max`)),
  };
}

function writeRange(params: URLSearchParams, prefix: string, range: NumericRange): void {
  if (range.min !== null) {
    params.set(`${prefix}Min`, String(range.min));
  }
  if (range.max !== null) {
    params.set(`${prefix}Max`, String(range.max));
  }
}

/**
 * The view an address describes.
 *
 * Every part of it falls back rather than failing: a hand-edited or outdated
 * link should land on the catalogue showing something, not on an error. The
 * bounds are read in the units they are stored in — metres, and whole percent
 * — because this address is a bookmark rather than a document, and a
 * kilometre figure here would have to round twice to survive the round trip.
 *
 * Surfaces are named rather than numbered, and anything the build does not know
 * is dropped: a link written by a later version must narrow this one by the
 * classes it does understand rather than by none.
 */
export function readView(params: URLSearchParams): CatalogueView {
  const sort = params.get("sort");
  const direction = params.get("dir");

  return {
    query: params.get("q") ?? "",
    sort: isSortColumn(sort) ? sort : DEFAULT_VIEW.sort,
    direction: direction === "asc" || direction === "desc" ? direction : DEFAULT_VIEW.direction,
    filters: {
      distanceMetres: readRange(params, "distance"),
      ascentMetres: readRange(params, "ascent"),
      maxGradientPercent: readRange(params, "gradient"),
      surfaces: params.getAll("surface").filter(isSurfaceKind),
    },
  };
}

/**
 * The address a view is written to, carrying only what differs from the
 * default: an untouched catalogue has a bare `/catalogue` for its address
 * rather than six parameters restating that nothing was asked.
 */
export function writeView(view: CatalogueView): URLSearchParams {
  const params = new URLSearchParams();
  if (view.query.trim() !== "") {
    params.set("q", view.query);
  }
  if (view.sort !== DEFAULT_VIEW.sort) {
    params.set("sort", view.sort);
  }
  if (view.direction !== DEFAULT_VIEW.direction) {
    params.set("dir", view.direction);
  }
  writeRange(params, "distance", view.filters.distanceMetres);
  writeRange(params, "ascent", view.filters.ascentMetres);
  writeRange(params, "gradient", view.filters.maxGradientPercent);
  // One parameter per class rather than one joined list, so a reader editing
  // the address by hand drops a class by deleting its own `&surface=`.
  for (const kind of view.filters.surfaces) {
    params.append("surface", kind);
  }

  return params;
}
