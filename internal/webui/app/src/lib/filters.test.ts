import { describe, expect, it } from "vitest";
import type { Route, SurfaceKind } from "../api/types";
import { EMPTY_FILTERS, hasActiveFilters, matchesFilters } from "./filters";

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
    maxGradientPercent: 12,
    pointCount: 400,
    ...overrides,
  };
}

describe("hasActiveFilters", () => {
  it("reads the empty filters as inactive", () => {
    expect(hasActiveFilters(EMPTY_FILTERS)).toBe(false);
  });

  it.each<[string, Partial<import("./filters").LibraryFilters>]>([
    ["a distance minimum", { distanceMetres: { min: 1000, max: null } }],
    ["a distance maximum", { distanceMetres: { min: null, max: 1000 } }],
    ["an ascent bound", { ascentMetres: { min: 100, max: null } }],
    ["a gradient bound", { maxGradientPercent: { min: null, max: 5 } }],
    ["a surface", { surfaces: ["gravel"] }],
  ])("reads %s as active", (_name, overrides) => {
    expect(hasActiveFilters({ ...EMPTY_FILTERS, ...overrides })).toBe(true);
  });
});

describe("matchesFilters", () => {
  it("passes every route when nothing is filtered", () => {
    expect(matchesFilters(route(), EMPTY_FILTERS, undefined)).toBe(true);
  });

  it.each([
    ["at the minimum bound", 50_000, true],
    ["at the maximum bound", 80_000, true],
    ["just below the minimum", 49_999, false],
    ["just above the maximum", 80_001, false],
  ])("treats a distance bound as inclusive: %s", (_name, distanceMetres, want) => {
    const filters = { ...EMPTY_FILTERS, distanceMetres: { min: 50_000, max: 80_000 } };
    expect(matchesFilters(route({ distanceMetres }), filters, undefined)).toBe(want);
  });

  it("leaves a side unbounded when its input is empty", () => {
    const filters = { ...EMPTY_FILTERS, ascentMetres: { min: 500, max: null } };
    expect(matchesFilters(route({ ascentMetres: 100_000 }), filters, undefined)).toBe(true);
    expect(matchesFilters(route({ ascentMetres: 499 }), filters, undefined)).toBe(false);
  });

  // A stage with no elevation data reports zero ascent and zero gradient the
  // same way a genuinely flat one does. The filter has no way to tell them
  // apart, and must not invent one: zero is compared like any other value.
  it("compares zero ascent and gradient the same as any other value", () => {
    const flat = route({ ascentMetres: 0, maxGradientPercent: 0 });
    expect(
      matchesFilters(flat, { ...EMPTY_FILTERS, ascentMetres: { min: 0, max: 0 } }, undefined),
    ).toBe(true);
    expect(
      matchesFilters(flat, { ...EMPTY_FILTERS, ascentMetres: { min: 1, max: null } }, undefined),
    ).toBe(false);
  });

  it("bounds the max gradient the same way as the other numeric fields", () => {
    const filters = { ...EMPTY_FILTERS, maxGradientPercent: { min: null, max: 8 } };
    expect(matchesFilters(route({ maxGradientPercent: 8 }), filters, undefined)).toBe(true);
    expect(matchesFilters(route({ maxGradientPercent: 9 }), filters, undefined)).toBe(false);
  });

  it("requires every active numeric bound to hold at once", () => {
    const filters = {
      ...EMPTY_FILTERS,
      distanceMetres: { min: 10_000, max: 60_000 },
      ascentMetres: { min: 500, max: null },
    };
    // Distance passes, ascent does not.
    expect(
      matchesFilters(route({ distanceMetres: 20_000, ascentMetres: 100 }), filters, undefined),
    ).toBe(false);
    expect(
      matchesFilters(route({ distanceMetres: 20_000, ascentMetres: 600 }), filters, undefined),
    ).toBe(true);
  });

  it("passes a route holding any one of several checked surfaces", () => {
    const filters = { ...EMPTY_FILTERS, surfaces: ["asphalt", "gravel"] as SurfaceKind[] };
    const kinds = new Set<SurfaceKind>(["gravel"]);
    expect(matchesFilters(route(), filters, kinds)).toBe(true);
  });

  it("fails a route whose surfaces are all unchecked", () => {
    const filters = { ...EMPTY_FILTERS, surfaces: ["asphalt"] as SurfaceKind[] };
    const kinds = new Set<SurfaceKind>(["gravel", "ground"]);
    expect(matchesFilters(route(), filters, kinds)).toBe(false);
  });

  // Unclassified geometry — never enriched, or not fetched yet — must never
  // read as though it matched a specific class it has no answer for.
  it.each([
    ["absent geometry", undefined],
    ["an empty classification", new Set<SurfaceKind>()],
  ])("never matches a surface filter against %s", (_name, kinds) => {
    const filters = { ...EMPTY_FILTERS, surfaces: ["asphalt"] as SurfaceKind[] };
    expect(matchesFilters(route(), filters, kinds)).toBe(false);
  });

  it("still applies the numeric bounds alongside an unmatched surface filter", () => {
    const filters = {
      ...EMPTY_FILTERS,
      distanceMetres: { min: 100_000, max: null },
      surfaces: ["asphalt"] as SurfaceKind[],
    };
    const kinds = new Set<SurfaceKind>(["asphalt"]);
    // Surface matches, distance does not.
    expect(matchesFilters(route({ distanceMetres: 1000 }), filters, kinds)).toBe(false);
  });
});
