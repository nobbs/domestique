import { describe, expect, it } from "vitest";
import type { Route } from "../api/types";
import { matchesQuery, matchingRoutes } from "./library";

function stage(overrides: Partial<Route> = {}): Route {
  return {
    provider: "veloplanner",
    routeId: 12,
    stageOrder: 1,
    title: "Alpine loop — Descent",
    routeName: "Alpine loop",
    stageName: "Descent",
    sourceRevision: "2026-08-17",
    contentHash: "hash",
    distanceMetres: 42_500,
    ascentMetres: 620,
    maxGradientPercent: 11.4,
    pointCount: 1200,
    ...overrides,
  };
}

function named(routeId: number, stageOrder: number, routeName: string, stageName: string): Route {
  return stage({
    routeId,
    stageOrder,
    routeName,
    stageName,
    title: stageName ? `${routeName} — ${stageName}` : routeName,
  });
}

describe("matchesQuery", () => {
  it("keeps every stage for an empty or blank query", () => {
    expect(matchesQuery(stage(), "")).toBe(true);
    expect(matchesQuery(stage(), "   ")).toBe(true);
  });

  it("matches the route name and the stage name alike", () => {
    expect(matchesQuery(stage(), "alpine")).toBe(true);
    expect(matchesQuery(stage(), "descent")).toBe(true);
    expect(matchesQuery(stage(), "kaiserstuhl")).toBe(false);
  });

  it("ignores case and accents", () => {
    expect(matchesQuery(named(1, 1, "Kaiserstühl Loop", ""), "kaiserstuhl")).toBe(true);
    expect(matchesQuery(named(1, 1, "Kaiserstuhl Loop", ""), "KAISERSTÜHL")).toBe(true);
  });

  it("requires every word, in any order, across both names", () => {
    expect(matchesQuery(stage(), "descent alpine")).toBe(true);
    expect(matchesQuery(stage(), "alpine summit")).toBe(false);
  });
});

describe("matchingRoutes", () => {
  const library = [
    named(3, 1, "Rhine Traverse", "Valley floor"),
    named(1, 2, "Alpine loop", "Descent"),
    named(1, 1, "Alpine loop", "Climb"),
  ];

  it("puts what a search leaves in one order, by name", () => {
    expect(matchingRoutes(library, "").map((entry) => entry.title)).toEqual([
      "Alpine loop — Climb",
      "Alpine loop — Descent",
      "Rhine Traverse — Valley floor",
    ]);
  });

  it("breaks a tie the same way every time, whatever order it was given", () => {
    const tied = [
      named(2, 2, "Second", "Late"),
      named(1, 1, "First", "Early"),
      named(2, 1, "Second", "Early"),
    ].map((entry) => stage({ ...entry, title: "Same" }));
    const keys = (routes: Route[]) => routes.map((entry) => `${entry.routeId}/${entry.stageOrder}`);

    expect(keys(matchingRoutes(tied, ""))).toEqual(["1/1", "2/1", "2/2"]);
    expect(keys(matchingRoutes([...tied].reverse(), ""))).toEqual(["1/1", "2/1", "2/2"]);
  });

  it("filters and orders together", () => {
    expect(matchingRoutes(library, "alpine").map((entry) => entry.title)).toEqual([
      "Alpine loop — Climb",
      "Alpine loop — Descent",
    ]);
  });

  it("leaves the listing it was given untouched", () => {
    const given = [...library];
    matchingRoutes(given, "");

    expect(given).toEqual(library);
  });
});
