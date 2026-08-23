/**
 * Narrowing the library by what a route measures, beside searching it by name.
 *
 * Distance, ascent, and steepest gradient come straight off the safe listing
 * every route already carries. Surface composition has no field of its own
 * there — it rides along with the geometry the map already fetches for every
 * route to draw the whole library, so filtering by it costs no request this
 * page was not already making.
 */

import type { Route, SurfaceKind } from "../api/types";

/** An inclusive bound on either side; `null` means unbounded on that side. */
export interface NumericRange {
  min: number | null;
  max: number | null;
}

export interface LibraryFilters {
  distanceMetres: NumericRange;
  ascentMetres: NumericRange;
  maxGradientPercent: NumericRange;
  /** Empty means every surface passes — the filter is not narrowing on it. */
  surfaces: SurfaceKind[];
}

const UNBOUNDED: NumericRange = { min: null, max: null };

export const EMPTY_FILTERS: LibraryFilters = {
  distanceMetres: UNBOUNDED,
  ascentMetres: UNBOUNDED,
  maxGradientPercent: UNBOUNDED,
  surfaces: [],
};

export function hasActiveFilters(filters: LibraryFilters): boolean {
  return (
    filters.distanceMetres.min !== null ||
    filters.distanceMetres.max !== null ||
    filters.ascentMetres.min !== null ||
    filters.ascentMetres.max !== null ||
    filters.maxGradientPercent.min !== null ||
    filters.maxGradientPercent.max !== null ||
    filters.surfaces.length > 0
  );
}

function inRange(value: number, range: NumericRange): boolean {
  if (range.min !== null && value < range.min) {
    return false;
  }

  return !(range.max !== null && value > range.max);
}

/**
 * Whether one route passes the filters.
 *
 * `surfaceKinds` is the set of ground classes actually present on this
 * route's geometry, once fetched and classified — absent or empty for a
 * route the enrichment pass has not reached yet, or whose geometry has not
 * arrived. That must never read as though it matched a surface filter it has
 * no answer for: a surface filter only narrows to routes with a confirmed
 * class among the ones checked.
 *
 * A numeric bound is compared exactly as the field is stored, including
 * zero: a stage with no elevation data reports zero ascent and zero max
 * gradient the same way a genuinely flat one does, and a filter that
 * excluded zero specially would need information neither this page nor the
 * service has.
 */
export function matchesFilters(
  route: Route,
  filters: LibraryFilters,
  surfaceKinds: ReadonlySet<SurfaceKind> | undefined,
): boolean {
  if (!inRange(route.distanceMetres, filters.distanceMetres)) {
    return false;
  }
  if (!inRange(route.ascentMetres, filters.ascentMetres)) {
    return false;
  }
  if (!inRange(route.maxGradientPercent, filters.maxGradientPercent)) {
    return false;
  }
  if (filters.surfaces.length === 0) {
    return true;
  }

  return filters.surfaces.some((kind) => surfaceKinds?.has(kind) ?? false);
}
