/**
 * Narrowing the library by what a route measures, beside searching it by name.
 * Every bound reads straight off the safe listing each route already carries.
 */

import type { Route } from "../api/types";

/** An inclusive bound on either side; `null` means unbounded on that side. */
export interface NumericRange {
  min: number | null;
  max: number | null;
}

export interface LibraryFilters {
  distanceMetres: NumericRange;
  ascentMetres: NumericRange;
  movingSeconds: NumericRange;
  /** The source providers to keep; empty keeps every source. */
  providers: string[];
}

// Three separate objects, deliberately: every edit path replaces a range with
// a new one, and distinct objects keep that a guarantee rather than a rule.
export const EMPTY_FILTERS: LibraryFilters = {
  distanceMetres: { min: null, max: null },
  ascentMetres: { min: null, max: null },
  movingSeconds: { min: null, max: null },
  providers: [],
};

function isActive(range: NumericRange): boolean {
  return range.min !== null || range.max !== null;
}

export function hasActiveFilters(filters: LibraryFilters): boolean {
  return (
    isActive(filters.distanceMetres) ||
    isActive(filters.ascentMetres) ||
    isActive(filters.movingSeconds) ||
    filters.providers.length > 0
  );
}

/** How many filters narrow the library, for a toggle's count badge. */
export function activeFilterCount(filters: LibraryFilters): number {
  return (
    [filters.distanceMetres, filters.ascentMetres, filters.movingSeconds].filter(isActive).length +
    (filters.providers.length > 0 ? 1 : 0)
  );
}

function inRange(value: number, range: NumericRange): boolean {
  // A crossed range, which only a hand-edited address can produce, reads as
  // the span between its two bounds rather than as nothing at all.
  const crossed = range.min !== null && range.max !== null && range.min > range.max;
  const min = crossed ? range.max : range.min;
  const max = crossed ? range.min : range.max;
  if (min !== null && value < min) {
    return false;
  }

  return !(max !== null && value > max);
}

/**
 * Whether one route passes the filters.
 *
 * A bound is compared exactly as the field is stored, including zero: a route
 * with no elevation data reports zero ascent the same way a flat one does, and
 * a route nothing has predicted a moving time for reports zero seconds.
 */
export function matchesFilters(route: Route, filters: LibraryFilters): boolean {
  return (
    inRange(route.distanceMetres, filters.distanceMetres) &&
    inRange(route.ascentMetres, filters.ascentMetres) &&
    inRange(route.movingSeconds ?? 0, filters.movingSeconds) &&
    (filters.providers.length === 0 || filters.providers.includes(route.provider))
  );
}
