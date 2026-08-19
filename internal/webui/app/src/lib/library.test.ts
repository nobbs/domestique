import { describe, expect, it } from "vitest";
import type { Stage } from "../api/types";
import { arrangeStages, matchesQuery, stageCounts } from "./library";

function stage(overrides: Partial<Stage> = {}): Stage {
  return {
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

function named(routeId: number, stageOrder: number, routeName: string, stageName: string): Stage {
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

describe("arrangeStages", () => {
  const library = [
    named(3, 1, "Rhine Traverse", "Valley floor"),
    named(1, 2, "Alpine loop", "Descent"),
    named(1, 1, "Alpine loop", "Climb"),
  ].map((entry, index) =>
    stage({ ...entry, distanceMetres: 10_000 * (index + 1), ascentMetres: 100 * (3 - index) }),
  );

  it("sorts by name, then by the stage's own identity", () => {
    expect(arrangeStages(library, "", "name").map((entry) => entry.title)).toEqual([
      "Alpine loop — Climb",
      "Alpine loop — Descent",
      "Rhine Traverse — Valley floor",
    ]);
  });

  it("sorts by distance, longest first", () => {
    expect(arrangeStages(library, "", "distance").map((entry) => entry.distanceMetres)).toEqual([
      30_000, 20_000, 10_000,
    ]);
  });

  it("sorts by ascent, most climbing first", () => {
    expect(arrangeStages(library, "", "ascent").map((entry) => entry.ascentMetres)).toEqual([
      300, 200, 100,
    ]);
  });

  it("breaks a tie the same way every time, whatever order it was given", () => {
    const tied = [
      named(2, 2, "Second", "Late"),
      named(1, 1, "First", "Early"),
      named(2, 1, "Second", "Early"),
    ].map((entry) => stage({ ...entry, distanceMetres: 5_000 }));
    const keys = (stages: Stage[]) => stages.map((entry) => `${entry.routeId}/${entry.stageOrder}`);

    expect(keys(arrangeStages(tied, "", "distance"))).toEqual(["1/1", "2/1", "2/2"]);
    expect(keys(arrangeStages([...tied].reverse(), "", "distance"))).toEqual(["1/1", "2/1", "2/2"]);
  });

  it("filters and orders together", () => {
    expect(arrangeStages(library, "alpine", "name").map((entry) => entry.title)).toEqual([
      "Alpine loop — Climb",
      "Alpine loop — Descent",
    ]);
  });

  it("leaves the listing it was given untouched", () => {
    const given = [...library];
    arrangeStages(given, "", "name");

    expect(given).toEqual(library);
  });
});

describe("stageCounts", () => {
  it("counts the stages each source route contributes", () => {
    const counts = stageCounts([
      named(1, 1, "Alpine loop", "Climb"),
      named(1, 2, "Alpine loop", "Descent"),
      named(3, 1, "Rhine Traverse", ""),
    ]);

    expect(counts.get(1)).toBe(2);
    expect(counts.get(3)).toBe(1);
  });
});
