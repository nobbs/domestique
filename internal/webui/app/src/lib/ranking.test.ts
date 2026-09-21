import { describe, expect, it } from "vitest";
import type { Route } from "../api/types";
import { initialDirection, sortRoutes } from "./ranking";

function route(title: string, overrides: Partial<Route> = {}): Route {
  return {
    provider: "veloplanner",
    sourceRouteId: 1,
    stageOrder: 1,
    title,
    sourceRouteName: title,
    routeName: title,
    sourceRevision: "2026-08-17",
    contentHash: `hash-${title}`,
    distanceMetres: 10_000,
    ascentMetres: 100,
    descentMetres: 80,
    maxGradientPercent: 5,
    pointCount: 10,
    movingSeconds: 3_600,
    ...overrides,
  };
}

const titles = (routes: Route[]) => routes.map((entry) => entry.title);

/** A route nothing has predicted a moving time for: the field is absent, not zero. */
function unpredicted(base: Route): Route {
  const { movingSeconds: _unpredicted, ...rest } = base;

  return rest;
}

describe("sortRoutes", () => {
  const library = [
    route("Alps", { distanceMetres: 30_000, ascentMetres: 900, maxGradientPercent: 12 }),
    route("Border", { distanceMetres: 10_000, ascentMetres: 300, maxGradientPercent: 4 }),
    route("Coast", { distanceMetres: 20_000, ascentMetres: 100, maxGradientPercent: 8 }),
  ];

  it("orders by name whatever order the library arrived in", () => {
    const shuffled = [library[2], library[0], library[1]].filter((entry) => entry !== undefined);

    expect(titles(sortRoutes(shuffled, "title", "asc"))).toEqual(["Alps", "Border", "Coast"]);
    expect(titles(sortRoutes(shuffled, "title", "desc"))).toEqual(["Coast", "Border", "Alps"]);
  });

  it("keeps routes of one name in the order they arrived, either way, so a later key can break the tie", () => {
    const named = [
      route("Loop", { distanceMetres: 10_000 }),
      route("Alps"),
      route("Loop", { distanceMetres: 30_000 }),
    ];
    // Least significant first, as the palette applies several `by`s.
    const byDistance = sortRoutes(named, "distance", "asc");
    const byNameThenDistance = sortRoutes(byDistance, "title", "desc");

    expect(byNameThenDistance.map((entry) => `${entry.title} ${entry.distanceMetres}`)).toEqual([
      "Loop 10000",
      "Loop 30000",
      "Alps 10000",
    ]);
  });

  it("ranks by each measured column in both directions", () => {
    expect(titles(sortRoutes(library, "distance", "desc"))).toEqual(["Alps", "Coast", "Border"]);
    expect(titles(sortRoutes(library, "ascent", "asc"))).toEqual(["Coast", "Border", "Alps"]);
    expect(titles(sortRoutes(library, "gradient", "desc"))).toEqual(["Alps", "Coast", "Border"]);
  });

  it("holds tied routes in the order they arrived, whichever way it is sorted", () => {
    const tied = [
      route("Alps", { distanceMetres: 10_000 }),
      route("Border", { distanceMetres: 10_000 }),
      route("Coast", { distanceMetres: 30_000 }),
    ];

    expect(titles(sortRoutes(tied, "distance", "asc"))).toEqual(["Alps", "Border", "Coast"]);
    expect(titles(sortRoutes(tied, "distance", "desc"))).toEqual(["Coast", "Alps", "Border"]);
  });

  it("sorts a route nothing predicted a moving time for last, either way", () => {
    const partial = [
      route("Alps", { movingSeconds: 7_200 }),
      unpredicted(route("Border")),
      route("Coast", { movingSeconds: 1_800 }),
    ];

    expect(titles(sortRoutes(partial, "movingTime", "asc"))).toEqual(["Coast", "Alps", "Border"]);
    expect(titles(sortRoutes(partial, "movingTime", "desc"))).toEqual(["Alps", "Coast", "Border"]);
  });

  it("leaves two routes nothing predicted in the order they arrived", () => {
    const neither = [unpredicted(route("Alps")), unpredicted(route("Border"))];

    expect(titles(sortRoutes(neither, "movingTime", "desc"))).toEqual(["Alps", "Border"]);
  });

  it("ranks by distance to start nearest first, unknown starts last", () => {
    const start: Record<string, number | undefined> = { Alps: 5_000, Coast: 1_000 };
    const startOf = (entry: Route) => start[entry.title];

    expect(titles(sortRoutes(library, "start", "asc", startOf))).toEqual([
      "Coast",
      "Alps",
      "Border",
    ]);
    expect(initialDirection("start")).toBe("asc");
  });

  it("does not disturb the array it was given", () => {
    const original = [...library];
    sortRoutes(library, "distance", "desc");

    expect(library).toEqual(original);
  });
});

describe("initialDirection", () => {
  it("opens names alphabetically and measurements at the largest", () => {
    expect(initialDirection("title")).toBe("asc");
    expect(initialDirection("distance")).toBe("desc");
    expect(initialDirection("movingTime")).toBe("desc");
  });
});
