import { describe, expect, it } from "vitest";
import type { Route } from "../api/types";
import { EMPTY_FILTERS, hasActiveFilters, type LibraryFilters, matchesFilters } from "./filters";

function route(overrides: Partial<Route> = {}): Route {
  return {
    provider: "veloplanner",
    sourceRouteId: 1,
    stageOrder: 1,
    title: "Kaiserstuhl Loop",
    sourceRouteName: "Kaiserstuhl Loop",
    routeName: "",
    sourceRevision: "1",
    contentHash: "hash",
    distanceMetres: 50_000,
    ascentMetres: 800,
    descentMetres: 700,
    maxGradientPercent: 12,
    movingSeconds: 7200,
    pointCount: 400,
    ...overrides,
  };
}

describe("hasActiveFilters", () => {
  it("reads the empty filters as inactive", () => {
    expect(hasActiveFilters(EMPTY_FILTERS)).toBe(false);
  });

  it.each<[string, Partial<LibraryFilters>]>([
    ["a distance minimum", { distanceMetres: { min: 1000, max: null } }],
    ["a distance maximum", { distanceMetres: { min: null, max: 1000 } }],
    ["an ascent bound", { ascentMetres: { min: 100, max: null } }],
    ["a duration bound", { movingSeconds: { min: null, max: 3600 } }],
  ])("reads %s as active", (_name, overrides) => {
    expect(hasActiveFilters({ ...EMPTY_FILTERS, ...overrides })).toBe(true);
  });
});

describe("matchesFilters", () => {
  it("passes every route when nothing is filtered", () => {
    expect(matchesFilters(route(), EMPTY_FILTERS)).toBe(true);
  });

  it.each([
    ["at the minimum bound", 50_000, true],
    ["at the maximum bound", 80_000, true],
    ["just below the minimum", 49_999, false],
    ["just above the maximum", 80_001, false],
  ])("treats a distance bound as inclusive: %s", (_name, distanceMetres, want) => {
    const filters = { ...EMPTY_FILTERS, distanceMetres: { min: 50_000, max: 80_000 } };
    expect(matchesFilters(route({ distanceMetres }), filters)).toBe(want);
  });

  it("leaves a side unbounded when it is null", () => {
    const filters = { ...EMPTY_FILTERS, ascentMetres: { min: 500, max: null } };
    expect(matchesFilters(route({ ascentMetres: 100_000 }), filters)).toBe(true);
    expect(matchesFilters(route({ ascentMetres: 499 }), filters)).toBe(false);
  });

  // A stage with no elevation data reports zero ascent the same way a flat
  // one does. The filter has no way to tell them apart, and must not invent one.
  it("compares zero ascent the same as any other value", () => {
    const flat = route({ ascentMetres: 0 });
    expect(matchesFilters(flat, { ...EMPTY_FILTERS, ascentMetres: { min: 0, max: 0 } })).toBe(true);
    expect(matchesFilters(flat, { ...EMPTY_FILTERS, ascentMetres: { min: 1, max: null } })).toBe(
      false,
    );
  });

  it("bounds the moving time, reading an unpredicted one as zero", () => {
    const filters = { ...EMPTY_FILTERS, movingSeconds: { min: 3600, max: 7200 } };
    expect(matchesFilters(route({ movingSeconds: 7200 }), filters)).toBe(true);
    expect(matchesFilters(route({ movingSeconds: 7201 }), filters)).toBe(false);
    const { movingSeconds: _unpredicted, ...rest } = route();
    expect(matchesFilters(rest, filters)).toBe(false);
  });

  it("requires every active bound to hold at once", () => {
    const filters = {
      ...EMPTY_FILTERS,
      distanceMetres: { min: 10_000, max: 60_000 },
      ascentMetres: { min: 500, max: null },
    };
    expect(matchesFilters(route({ distanceMetres: 20_000, ascentMetres: 100 }), filters)).toBe(
      false,
    );
    expect(matchesFilters(route({ distanceMetres: 20_000, ascentMetres: 600 }), filters)).toBe(
      true,
    );
  });
});
