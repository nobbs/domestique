/**
 * The catalogue, as a reader drives it.
 *
 * What is tested here is the agreement the page exists to keep: the order the
 * sort control promises is the order the rows are in, the address carries that
 * order across a visit to the atlas, and a row leads to the route it names.
 */

import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { fireEvent, render, screen, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { MemoryRouter, Route, Routes, useLocation } from "react-router";
import { beforeEach, describe, expect, it, vi } from "vitest";
import { routeGeometryQuery, routesQuery, statusQuery, webUIConfigQuery } from "../../api/queries";
import type { Route as LibraryRoute, RouteGeometry, Status, WebUIConfig } from "../../api/types";
import { stubPendingFetch } from "../../test/network";
import { IDLE_STATUS } from "../../test/status";
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
    descentMetres: 80,
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

const STATUS: Status = {
  ...IDLE_STATUS,
  sync: {
    ...IDLE_STATUS.sync,
    phases: {
      source: {
        lastCompletedAt: "2026-08-29T07:00:00Z",
        lastResult: "succeeded",
        sourceRoutes: 3,
        created: 0,
        updated: 0,
        deleted: 0,
      },
    },
  },
};

const CONFIG: WebUIConfig = {
  basemaps: [],
  sourceBaseUrls: {},
  timezone: "Europe/Berlin",
  identity: { display: "rider@example.test", admin: false },
};

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

function show(
  library: LibraryRoute[] = LIBRARY,
  entry = "/catalogue",
  {
    geometry = true,
    nothingToDivide = false,
  }: { geometry?: boolean; nothingToDivide?: boolean } = {},
) {
  const client = new QueryClient({
    defaultOptions: { queries: { retry: false, staleTime: Number.POSITIVE_INFINITY } },
  });
  client.setQueryData(routesQuery().queryKey, library);
  client.setQueryData(statusQuery().queryKey, STATUS);
  client.setQueryData(webUIConfigQuery().queryKey, CONFIG);
  // Seeded rather than fetched: the glyphs and the mix bars both read this,
  // under the same keys the atlas caches it with.
  if (!geometry) {
    stubPendingFetch();
  }
  if (geometry) {
    library.forEach((route, index) => {
      const seeded = geometryFor(index);
      client.setQueryData(
        routeGeometryQuery(route.provider, route.sourceRouteId, route.stageOrder).queryKey,
        // A geometry that arrived and holds one point: nothing to classify and
        // nothing to band, which is a different answer from not having arrived.
        nothingToDivide
          ? { ...seeded, coordinates: seeded.coordinates.slice(0, 1), surface: undefined }
          : seeded,
      );
    });
  }

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
 * The "Library" card, scoped so a route also listed under "Recently updated"
 * is not confused with it. jsdom does not resolve `<section>` to the "region"
 * role, so the card is found by its own heading and read from there.
 */
function libraryRegion(): HTMLElement {
  // The heading's accessible name also carries the count subtitle, so this
  // matches on the leading word rather than the whole thing.
  return screen.getByRole("heading", { name: /^Library/ }).closest("section") as HTMLElement;
}

/** The route names in the order the ledger has them, read off each row's own link. */
function shownTitles(): string[] {
  return within(libraryRegion())
    .getAllByRole("link")
    .map((link) => link.textContent ?? "");
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

  it("ranks by a measure when it is chosen, and turns it around on a second press", async () => {
    const user = userEvent.setup();
    show();

    await user.click(screen.getByRole("button", { name: "Distance" }));
    expect(shownTitles()).toEqual([
      expect.stringContaining("Alpine loop"),
      expect.stringContaining("Coast ride"),
      expect.stringContaining("Border run"),
    ]);
    expect(screen.getByRole("button", { name: "Distance", pressed: true })).toBeInTheDocument();
    expect(screen.getByRole("button", { name: "Descending" })).toBeInTheDocument();

    await user.click(screen.getByRole("button", { name: "Distance" }));
    expect(shownTitles()).toEqual([
      expect.stringContaining("Border run"),
      expect.stringContaining("Coast ride"),
      expect.stringContaining("Alpine loop"),
    ]);
    expect(screen.getByRole("button", { name: "Ascending" })).toBeInTheDocument();
  });

  it("keeps the order and the search in the address, so leaving and returning restores them", async () => {
    const user = userEvent.setup();
    show();

    await user.click(screen.getByRole("button", { name: "Ascent" }));
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

  it("stays lit while it holds text, and a clear button empties it", async () => {
    const user = userEvent.setup();
    show();

    const field = screen.getByRole("searchbox");
    expect(field.closest("label")).not.toHaveAttribute("data-lit");

    await user.type(field, "coast");
    expect(field.closest("label")).toHaveAttribute("data-lit");

    await user.click(screen.getByRole("button", { name: "Clear search" }));
    expect(field).toHaveValue("");
    expect(field.closest("label")).not.toHaveAttribute("data-lit");
  });

  it("says a search matched nothing rather than showing an empty ledger", async () => {
    const user = userEvent.setup();
    show();

    await user.type(screen.getByRole("searchbox"), "montreal");

    expect(screen.getByText("Nothing here is called that.")).toBeInTheDocument();
    expect(within(libraryRegion()).queryAllByRole("link")).toHaveLength(0);
  });

  it("hands a route to the atlas rather than opening it here", async () => {
    const user = userEvent.setup();
    show();

    await user.click(within(libraryRegion()).getByRole("link", { name: /Coast ride/ }));

    expect(screen.getByText("the atlas")).toBeInTheDocument();
    expect(screen.getByTestId("address")).toHaveTextContent("route=veloplanner%2F3%2F1");
  });

  it("draws each route's shape from the geometry the atlas caches", () => {
    show();

    expect(
      within(libraryRegion()).getByRole("img", { name: "Shape of Alpine loop" }),
    ).toBeInTheDocument();
    expect(within(libraryRegion()).getAllByRole("img", { name: /^Shape of / })).toHaveLength(3);
  });

  it("divides each route by surface and by gradient, from that same geometry", () => {
    show();

    const [first] = within(libraryRegion()).getAllByRole("listitem");
    // The seeded geometry makes the first route wholly gravel and, with every
    // point at one elevation, wholly flat.
    expect(
      within(first as HTMLElement).getByRole("img", { name: /Surface: Gravel 100%/ }),
    ).toBeInTheDocument();
    expect(
      within(first as HTMLElement).getByRole("img", { name: /Gradient: flat 100%/ }),
    ).toBeInTheDocument();
  });

  it("shows no bars for a route nothing has measured", () => {
    // Geometry present and flat throughout, with no surface on it: there is
    // nothing to divide, so the row shows plain figures and no mix bars.
    show([libraryRoute("Unmeasured", { sourceRouteId: 9 })], "/catalogue", {
      nothingToDivide: true,
    });

    expect(within(libraryRegion()).getByText("Unmeasured")).toBeInTheDocument();
    expect(
      within(libraryRegion()).queryByRole("img", { name: /^Surface:/ }),
    ).not.toBeInTheDocument();
    expect(
      within(libraryRegion()).queryByRole("img", { name: /^Gradient:/ }),
    ).not.toBeInTheDocument();
  });

  it("claims nothing about a route whose geometry has not arrived", () => {
    // Nothing seeded, so the row shows no bars for want of an answer rather
    // than because there is none.
    show([libraryRoute("Pending", { sourceRouteId: 8 })], "/catalogue", { geometry: false });

    expect(within(libraryRegion()).getByText("Pending")).toBeInTheDocument();
    expect(
      within(libraryRegion()).queryByRole("img", { name: /^Surface:/ }),
    ).not.toBeInTheDocument();
    expect(
      within(libraryRegion()).queryByRole("img", { name: /^Gradient:/ }),
    ).not.toBeInTheDocument();
  });

  it("marks a route that is new or updated beside its name", () => {
    show();

    expect(within(libraryRegion()).getAllByText("New")).toHaveLength(3);
  });

  it("narrows by a slider bound and writes it to the address", async () => {
    show();

    // The filters card sits open on a wide screen; ascents of 900, 300 and
    // 100 m give a track to 900 m by 20 m, and the thumb is a native range
    // input, so one change event reaches it.
    screen.getByRole("slider", { name: "Ascent min" }).focus();
    fireEvent.change(document.activeElement as HTMLInputElement, { target: { value: "400" } });

    expect(shownTitles()).toEqual([expect.stringContaining("Alpine loop")]);
    expect(screen.getByTestId("address")).toHaveTextContent("ascentMin=400");
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
    const client = new QueryClient({
      defaultOptions: { queries: { retry: false, staleTime: Number.POSITIVE_INFINITY } },
    });
    client.setQueryData(statusQuery().queryKey, STATUS);
    client.setQueryData(webUIConfigQuery().queryKey, CONFIG);

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

  describe("the filters toggle", () => {
    it("hides and shows the filters card, and counts what is active while closed", async () => {
      const user = userEvent.setup();
      show(LIBRARY, "/catalogue?ascentMin=400");

      const toggle = screen.getByRole("button", { name: /Filters/ });
      expect(toggle).toHaveAttribute("aria-expanded", "true");

      await user.click(toggle);

      expect(toggle).toHaveAttribute("aria-expanded", "false");
      expect(toggle).toHaveTextContent("1");
      expect(screen.queryByRole("slider", { name: "Ascent min" })).not.toBeInTheDocument();

      await user.click(toggle);
      expect(screen.getByRole("slider", { name: "Ascent min" })).toBeInTheDocument();
    });
  });

  describe("the library card", () => {
    it("states routes, distance, the longest ride and the most climbing", () => {
      show();

      const totals = screen
        .getByRole("heading", { name: "The library" })
        .closest("section") as HTMLElement;
      expect(within(totals).getByText("3")).toBeInTheDocument();
      expect(within(totals).getByText("60.0 km")).toBeInTheDocument();
      expect(within(totals).getByText(/Alpine loop · 30.0 km/)).toBeInTheDocument();
      expect(within(totals).getByText(/Alpine loop · 900 m/)).toBeInTheDocument();
    });

    it("does not show when the library is empty", () => {
      show([]);

      expect(screen.queryByRole("heading", { name: "The library" })).not.toBeInTheDocument();
    });
  });

  describe("recently updated", () => {
    it("lists the routes with the newest parseable revision, newest first", () => {
      show();

      const recent = screen
        .getByRole("heading", { name: "Recently updated" })
        .closest("section") as HTMLElement;
      expect(
        within(recent)
          .getAllByRole("link")
          .map((link) => link.textContent),
      ).toEqual(["Alpine loop", "Border run", "Coast ride"]);
      expect(within(recent).getAllByText("New")).toHaveLength(3);
    });

    it("hides when no route has a revision that parses as a date", () => {
      show([libraryRoute("Unversioned", { sourceRouteId: 5, sourceRevision: "not-a-date" })]);

      expect(screen.queryByRole("heading", { name: "Recently updated" })).not.toBeInTheDocument();
    });
  });

  describe("where a ledger will not fit", () => {
    beforeEach(() => {
      stubViewport(true);
    });

    it("stacks the same routes as cards, each leading to the atlas", async () => {
      const user = userEvent.setup();
      show();

      expect(within(libraryRegion()).getAllByRole("listitem")).toHaveLength(3);

      await user.click(within(libraryRegion()).getByRole("link", { name: /Coast ride/ }));

      expect(screen.getByText("the atlas")).toBeInTheDocument();
    });

    it("still ranks, since the order is in the address rather than in the headings", () => {
      show(LIBRARY, "/catalogue?sort=ascent&dir=asc");

      expect(shownTitles()).toEqual([
        expect.stringContaining("Coast ride"),
        expect.stringContaining("Border run"),
        expect.stringContaining("Alpine loop"),
      ]);
    });

    it("keeps the filters closed until asked, above the ledger when opened", async () => {
      const user = userEvent.setup();
      show();

      expect(screen.queryByRole("slider", { name: "Ascent min" })).not.toBeInTheDocument();

      await user.click(screen.getByRole("button", { name: /Filters/ }));

      expect(screen.getByRole("slider", { name: "Ascent min" })).toBeInTheDocument();
    });
  });
});
