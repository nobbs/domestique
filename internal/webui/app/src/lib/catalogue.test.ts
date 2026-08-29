import { describe, expect, it } from "vitest";
import type { Route } from "../api/types";
import { DEFAULT_VIEW, initialDirection, readView, sortRoutes, writeView } from "./catalogue";
import { EMPTY_FILTERS } from "./filters";

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

  it("leaves the library in the order it arrived when sorting by name", () => {
    expect(titles(sortRoutes(library, "title", "asc"))).toEqual(["Alps", "Border", "Coast"]);
    expect(titles(sortRoutes(library, "title", "desc"))).toEqual(["Coast", "Border", "Alps"]);
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

describe("readView", () => {
  it("answers an empty address with the default view", () => {
    expect(readView(new URLSearchParams())).toEqual(DEFAULT_VIEW);
  });

  it("reads a search, an order, and the bounds", () => {
    const view = readView(
      new URLSearchParams("q=rhine&sort=ascent&dir=desc&distanceMin=8000&gradientMax=12"),
    );

    expect(view.query).toBe("rhine");
    expect(view.sort).toBe("ascent");
    expect(view.direction).toBe("desc");
    expect(view.filters.distanceMetres).toEqual({ min: 8_000, max: null });
    expect(view.filters.maxGradientPercent).toEqual({ min: null, max: 12 });
  });

  it("falls back rather than failing on anything it does not recognise", () => {
    const view = readView(new URLSearchParams("sort=colour&dir=sideways&distanceMin=far"));

    expect(view.sort).toBe(DEFAULT_VIEW.sort);
    expect(view.direction).toBe(DEFAULT_VIEW.direction);
    expect(view.filters.distanceMetres.min).toBeNull();
  });

  it("never reads a surface filter, which needs geometry this page has not fetched", () => {
    expect(readView(new URLSearchParams("surfaces=paved")).filters.surfaces).toEqual([]);
  });
});

describe("writeView", () => {
  it("writes nothing at all for an untouched catalogue", () => {
    expect(writeView(DEFAULT_VIEW).toString()).toBe("");
  });

  it("round-trips a view through the address", () => {
    const view = {
      query: "rhine",
      sort: "gradient",
      direction: "asc",
      filters: {
        ...EMPTY_FILTERS,
        distanceMetres: { min: 8_000, max: 120_000 },
        ascentMetres: { min: null, max: 900 },
      },
    } as const;

    expect(readView(writeView(view))).toEqual(view);
  });
});
