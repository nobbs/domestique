/**
 * The catalogue, as a reader drives it.
 *
 * What is tested here is the agreement the page exists to keep: the order the
 * headings promise is the order the rows are in, the address carries that order
 * across a visit to the atlas, and a row leads to the route it names.
 */

import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { render, screen, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { MemoryRouter, Route, Routes, useLocation } from "react-router";
import { beforeEach, describe, expect, it, vi } from "vitest";
import { routeGeometryQuery, routesQuery, statusQuery } from "../../api/queries";
import type { Route as LibraryRoute, RouteGeometry, Status } from "../../api/types";
import { CataloguePage } from "./CataloguePage";

function libraryRoute(
  title: string,
  overrides: Partial<LibraryRoute> & { sourceRouteId: number },
): LibraryRoute {
  return {
    provider: "veloplanner",
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

const LIBRARY: LibraryRoute[] = [
  libraryRoute("Alpine loop", { sourceRouteId: 1, distanceMetres: 30_000, ascentMetres: 900 }),
  libraryRoute("Border run", { sourceRouteId: 2, distanceMetres: 10_000, ascentMetres: 300 }),
  libraryRoute("Coast ride", { sourceRouteId: 3, distanceMetres: 20_000, ascentMetres: 100 }),
];

const STATUS = {
  sync: { phases: { source: { lastCompletedAt: "2026-08-29T07:00:00Z" } } },
} as unknown as Status;

/** Reports the address the page is on, so a link can be followed and read back. */
function Address() {
  const location = useLocation();

  return <span data-testid="address">{`${location.pathname}${location.search}`}</span>;
}

/** A short line, enough for a glyph to have a shape and a surface to classify. */
function geometryFor(index: number): RouteGeometry {
  const coordinates = Array.from({ length: 8 }, (_, step): [number, number, number] => [
    8 + step * 0.01,
    49 + index * 0.1 + step * 0.005,
    100,
  ]);

  return {
    bbox: [8, 49, 8.07, 49.5],
    coordinates,
    surface: {
      matchedMetres: 1_000,
      ranges: [{ kind: index === 0 ? "gravel" : "asphalt", startIndex: 0, endIndex: 7 }],
    },
  };
}

function show(library: LibraryRoute[] = LIBRARY, entry = "/catalogue") {
  const client = new QueryClient({ defaultOptions: { queries: { retry: false } } });
  client.setQueryData(routesQuery().queryKey, library);
  client.setQueryData(statusQuery().queryKey, STATUS);
  // Seeded rather than fetched: the glyphs and the surface filter both read
  // this, under the same keys the atlas caches it with.
  library.forEach((route, index) => {
    client.setQueryData(
      routeGeometryQuery(route.provider, route.sourceRouteId, route.stageOrder).queryKey,
      geometryFor(index),
    );
  });

  return render(
    <QueryClientProvider client={client}>
      <MemoryRouter initialEntries={[entry]}>
        <Address />
        <Routes>
          <Route path="/catalogue" element={<CataloguePage />} />
          <Route path="/" element={<span>the atlas</span>} />
        </Routes>
      </MemoryRouter>
    </QueryClientProvider>,
  );
}

/**
 * The route names in the order the table has them, read off each row's own
 * link rather than off a column index — the shape column has no name in it.
 */
function shownTitles(): string[] {
  return screen
    .getAllByRole("row")
    .slice(1)
    .map((row) => within(row).getByRole("link").textContent ?? "");
}

/**
 * The one breakpoint, answered. jsdom implements no media query at all, so
 * without this the page would read every layout question as false by accident
 * rather than by arrangement.
 */
function stubViewport(narrow: boolean) {
  vi.stubGlobal(
    "matchMedia",
    vi.fn((query: string) => ({
      matches: narrow && query.includes("max-width"),
      media: query,
      addEventListener: vi.fn(),
      removeEventListener: vi.fn(),
    })),
  );
}

beforeEach(() => {
  stubViewport(false);
});

describe("CataloguePage", () => {
  it("lists the whole library by name, and says how much of it is shown", () => {
    show();

    expect(shownTitles()).toEqual([
      expect.stringContaining("Alpine loop"),
      expect.stringContaining("Border run"),
      expect.stringContaining("Coast ride"),
    ]);
    expect(screen.getByText(/3 routes/)).toBeInTheDocument();
  });

  it("ranks by a column when its heading is pressed, and turns it around on a second press", async () => {
    const user = userEvent.setup();
    show();

    await user.click(screen.getByRole("button", { name: "Distance" }));
    expect(shownTitles()).toEqual([
      expect.stringContaining("Alpine loop"),
      expect.stringContaining("Coast ride"),
      expect.stringContaining("Border run"),
    ]);
    expect(screen.getByRole("columnheader", { name: /Distance/ })).toHaveAttribute(
      "aria-sort",
      "descending",
    );

    await user.click(screen.getByRole("button", { name: "Distance" }));
    expect(shownTitles()).toEqual([
      expect.stringContaining("Border run"),
      expect.stringContaining("Coast ride"),
      expect.stringContaining("Alpine loop"),
    ]);
    expect(screen.getByRole("columnheader", { name: /Distance/ })).toHaveAttribute(
      "aria-sort",
      "ascending",
    );
  });

  it("keeps the order and the search in the address, so leaving and returning restores them", async () => {
    const user = userEvent.setup();
    show();

    await user.click(screen.getByRole("button", { name: "Climbing" }));
    await user.type(screen.getByRole("searchbox"), "r");

    expect(screen.getByTestId("address")).toHaveTextContent("sort=ascent");
    expect(screen.getByTestId("address")).toHaveTextContent("q=r");
  });

  it("reads an address it was linked to rather than starting over", () => {
    show(LIBRARY, "/catalogue?sort=ascent&dir=asc");

    expect(shownTitles()).toEqual([
      expect.stringContaining("Coast ride"),
      expect.stringContaining("Border run"),
      expect.stringContaining("Alpine loop"),
    ]);
  });

  it("narrows to what was searched for, and counts against the whole library", async () => {
    const user = userEvent.setup();
    show();

    await user.type(screen.getByRole("searchbox"), "coast");

    expect(shownTitles()).toEqual([expect.stringContaining("Coast ride")]);
    expect(screen.getByText(/1 of 3 routes/)).toBeInTheDocument();
  });

  it("keeps every character of a search", async () => {
    /*
     * This states the outcome; it cannot catch the bug that made it worth
     * stating. The field lost every letter but the last when typing outran the
     * router, and jsdom flushes between keystrokes, so the race has no room to
     * happen here — every suite in this repository was green while a real
     * browser showed "l" for "montreal". The story that guards it is
     * `NothingMatches`, which types with no delay in a real browser.
     */
    const user = userEvent.setup({ delay: null });
    show();

    await user.type(screen.getByRole("searchbox"), "coast");

    expect(screen.getByRole("searchbox")).toHaveValue("coast");
    expect(screen.getByTestId("address")).toHaveTextContent("q=coast");
  });

  it("says a search matched nothing rather than showing an empty table", async () => {
    const user = userEvent.setup();
    show();

    await user.type(screen.getByRole("searchbox"), "montreal");

    expect(screen.getByText("Nothing here is called that.")).toBeInTheDocument();
    expect(screen.queryByRole("table")).not.toBeInTheDocument();
  });

  it("hands a route to the atlas rather than opening it here", async () => {
    const user = userEvent.setup();
    show();

    await user.click(screen.getByRole("link", { name: "Coast ride" }));

    expect(screen.getByText("the atlas")).toBeInTheDocument();
    expect(screen.getByTestId("address")).toHaveTextContent("route=veloplanner%2F3%2F1");
  });

  it("draws each route's shape from the geometry the atlas caches", () => {
    show();

    expect(screen.getByRole("img", { name: "Shape of Alpine loop" })).toBeInTheDocument();
    expect(screen.getAllByRole("img", { name: /^Shape of / })).toHaveLength(3);
  });

  it("narrows by surface, which the same geometry classifies", async () => {
    const user = userEvent.setup();
    show();

    await user.click(screen.getByRole("button", { name: /Show the library filters/ }));
    await user.click(screen.getByRole("checkbox", { name: /gravel/i }));

    expect(shownTitles()).toEqual([expect.stringContaining("Alpine loop")]);
    expect(screen.getByTestId("address")).toHaveTextContent("surface=gravel");
  });

  it("says so when the library is empty", () => {
    show([]);

    expect(screen.getByText("No routes yet.")).toBeInTheDocument();
  });

  it("says a library that would not load did not, with what went wrong", async () => {
    // The listing left unseeded and the transport refusing it, which is the
    // one way this page can fail that is not the reader's own address.
    vi.stubGlobal(
      "fetch",
      vi.fn(() => Promise.reject(new Error("the listener refused the connection"))),
    );
    const client = new QueryClient({ defaultOptions: { queries: { retry: false } } });
    client.setQueryData(statusQuery().queryKey, STATUS);

    render(
      <QueryClientProvider client={client}>
        <MemoryRouter initialEntries={["/catalogue"]}>
          <CataloguePage />
        </MemoryRouter>
      </QueryClientProvider>,
    );

    expect(await screen.findByText("Could not load the route library.")).toBeInTheDocument();
    expect(screen.getByText("the listener refused the connection")).toBeInTheDocument();
  });

  describe("where a table will not fit", () => {
    beforeEach(() => {
      stubViewport(true);
    });

    it("stacks the same routes as cards, each leading to the atlas", async () => {
      const user = userEvent.setup();
      show();

      expect(screen.queryByRole("table")).not.toBeInTheDocument();
      expect(screen.getAllByRole("listitem")).toHaveLength(3);

      await user.click(screen.getByRole("link", { name: /Coast ride/ }));

      expect(screen.getByText("the atlas")).toBeInTheDocument();
    });

    it("still ranks, since the order is in the address rather than in the headings", () => {
      show(LIBRARY, "/catalogue?sort=ascent&dir=asc");

      expect(screen.getAllByRole("listitem").map((item) => item.textContent ?? "")).toEqual([
        expect.stringContaining("Coast ride"),
        expect.stringContaining("Border run"),
        expect.stringContaining("Alpine loop"),
      ]);
    });
  });
});
