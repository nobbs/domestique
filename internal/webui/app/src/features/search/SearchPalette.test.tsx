/**
 * The search palette, wherever it is opened from.
 *
 * What is tested here is the agreement it exists to keep: it is a way to any
 * route or draft from any page, it narrows and ranks the library the way its
 * own controls promise, it previews the highlighted row on a wide screen, and
 * picking one is a normal navigation.
 */

import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { render, screen, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { MemoryRouter, useLocation } from "react-router";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { getListPlansQueryKey } from "../../api/generated";
import { routeGeometryQuery, routesQuery, webUIConfigQuery } from "../../api/queries";
import type { Route as LibraryRoute, Position, RouteGeometry, WebUIConfig } from "../../api/types";
import { SearchButton } from "../../components/SearchButton";
import { SearchPaletteProvider } from "../../lib/searchPalette";
import { stubPendingFetch } from "../../test/network";
import { SearchPalette } from "./SearchPalette";

const drawn = vi.hoisted(() => ({ keys: [] as string[], mounts: 0 }));

// jsdom has no WebGL: the preview map only records which route it was asked to draw.
vi.mock("../routes/LibraryMap", async () => {
  const { useEffect } = await import("react");
  return {
    LibraryMap: (props: { lines: Array<{ key: string }> }) => {
      drawn.keys = props.lines.map((line) => line.key);
      useEffect(() => {
        drawn.mounts += 1;
      }, []);
      return <div data-testid="library-map" />;
    },
  };
});

function route(sourceRouteId: number, title: string): LibraryRoute {
  return {
    provider: "veloplanner",
    sourceRouteId,
    stageOrder: 1,
    sourceRouteName: title,
    routeName: "",
    title,
    sourceRevision: "2026-08-17",
    contentHash: `hash-${sourceRouteId}`,
    distanceMetres: 10_000 * sourceRouteId,
    ascentMetres: 100,
    descentMetres: 80,
    maxGradientPercent: 6,
    pointCount: 10,
  };
}

const RHINE = route(1, "Rhine Traverse");
const KAISERSTUHL = route(2, "Kaiserstuhl Loop");
const LIBRARY = [RHINE, KAISERSTUHL];

/** A short line whose first point moves further from `[8, 49]` as `index` grows. */
function geometryFor(index: number): RouteGeometry {
  return {
    bbox: [8, 49, 8.1, 49.5],
    coordinates: [
      [8, 49 + index * 0.5, 100],
      [8.05, 49.05 + index * 0.5, 140],
    ] as Position[],
  };
}

const PLANS = [
  {
    id: 7,
    name: "Saturday gravel",
    profile: "gravel" as const,
    published: false,
    version: 2,
    distanceMetres: 42_000,
    ascentMetres: 610,
    waypointCount: 6,
    updatedAt: "2026-09-20T09:00:00Z",
  },
  {
    id: 8,
    name: "Weekday loop",
    profile: "fastbike" as const,
    published: true,
    version: 1,
    distanceMetres: 18_000,
    ascentMetres: 120,
    waypointCount: 3,
    updatedAt: "2026-09-15T09:00:00Z",
  },
];

function config(planner: boolean): WebUIConfig {
  return {
    basemaps: [],
    sourceBaseUrls: {},
    timezone: "Europe/Berlin",
    planning: planner,
    identity: { display: "someone@example.test", admin: planner },
  };
}

/** Reports the address the palette navigated to, for a route or draft that was picked. */
function Landed() {
  const location = useLocation();

  return <span data-testid="landed">{`${location.pathname}${location.search}`}</span>;
}

function show(
  at: string,
  {
    planner = false,
    geometry = false,
    listing = "seeded",
  }: {
    planner?: boolean;
    /** Seeds every route's line, or only the first listed one's, for the preview and the Nearest sort. */
    geometry?: boolean | "first";
    /** The route listing in the cache, still on its way, or refused. */
    listing?: "seeded" | "pending" | "failed";
  } = {},
) {
  const client = new QueryClient({
    defaultOptions: { queries: { retry: false, staleTime: Number.POSITIVE_INFINITY } },
  });
  if (listing === "seeded") {
    client.setQueryData(routesQuery().queryKey, LIBRARY);
  } else if (listing === "pending") {
    stubPendingFetch();
  } else {
    vi.stubGlobal(
      "fetch",
      vi.fn(async () => new Response("{}", { status: 500 })),
    );
  }
  client.setQueryData(webUIConfigQuery().queryKey, config(planner));
  if (planner) {
    client.setQueryData(getListPlansQueryKey(), { data: { plans: PLANS } });
  }
  if (geometry) {
    LIBRARY.forEach((entry, index) => {
      if (geometry === "first" && entry !== KAISERSTUHL) {
        return;
      }
      client.setQueryData(
        routeGeometryQuery(entry.provider, entry.sourceRouteId, entry.stageOrder).queryKey,
        geometryFor(index),
      );
    });
  }

  return render(
    <QueryClientProvider client={client}>
      <MemoryRouter initialEntries={[at]}>
        <Landed />
        <SearchPaletteProvider>
          <SearchPalette themeChoice="system" />
          <SearchButton />
        </SearchPaletteProvider>
      </MemoryRouter>
    </QueryClientProvider>,
  );
}

/** The one breakpoint the palette's preview reads; jsdom implements no media query at all. */
function stubViewport(narrow: boolean) {
  vi.stubGlobal(
    "matchMedia",
    vi.fn((query: string) => ({
      matches: query.includes(narrow ? "max-width" : "min-width"),
      media: query,
      addEventListener: vi.fn(),
      removeEventListener: vi.fn(),
    })),
  );
}

/** A position near the first geometry's start, `geometryFor(0)`'s nearest. */
/** A position for jsdom, which has no geolocation; the rest of `navigator` stays, as userEvent reads it. */
function stubPosition(latitude = 49) {
  Object.defineProperty(navigator, "geolocation", {
    configurable: true,
    value: {
      getCurrentPosition: (found: (position: { coords: object }) => void) =>
        found({ coords: { latitude, longitude: 8 } }),
    },
  });
}

function titles(): string[] {
  return screen.getAllByRole("option").map((option) => option.textContent ?? "");
}

function searchbox(): HTMLInputElement {
  return screen.getByRole("searchbox", { name: "Search the route library" });
}

beforeEach(() => {
  // No preview and no Nearest ranking by default, so a test that says nothing
  // about the viewport asks for no route's geometry — the suite's refusing
  // fetch would otherwise fail it.
  stubViewport(true);
  drawn.keys = [];
  drawn.mounts = 0;
});

afterEach(() => {
  vi.unstubAllGlobals();
  Reflect.deleteProperty(navigator, "geolocation");
});

describe("SearchPalette", () => {
  it("says the library is loading, not that nothing matches, while it is on its way", async () => {
    show("/activities", { listing: "pending" });
    await userEvent.click(screen.getByRole("button", { name: "Search" }));

    expect(screen.getByText("Loading the route library…")).toBeInTheDocument();
    expect(screen.queryByText("Nothing here is called that.")).toBeNull();
  });

  it("says the library could not be loaded when the listing fails", async () => {
    show("/activities", { listing: "failed" });
    await userEvent.click(screen.getByRole("button", { name: "Search" }));

    expect(await screen.findByText("Could not load the route library.")).toBeInTheDocument();
  });

  it("opens from the menu bar's Search button", async () => {
    show("/activities");

    await userEvent.click(screen.getByRole("button", { name: "Search" }));

    expect(screen.getByRole("dialog", { name: "Search" })).toBeInTheDocument();
  });

  it("opens on ⌘K from a page with no search of its own", async () => {
    show("/activities");

    await userEvent.keyboard("{Meta>}k{/Meta}");

    expect(screen.getByRole("searchbox", { name: "Search the route library" })).toBeVisible();
  });

  it("does not bind ⌘K on the planner, which has its own search", async () => {
    show("/plan");

    await userEvent.keyboard("{Meta>}k{/Meta}");

    expect(screen.queryByRole("dialog")).toBeNull();
  });

  it("narrows the list as the query is typed", async () => {
    show("/activities");
    await userEvent.click(screen.getByRole("button", { name: "Search" }));

    await userEvent.type(screen.getByRole("searchbox"), "kaiserstuhl");

    expect(screen.getByRole("option", { name: /Kaiserstuhl Loop/ })).toBeInTheDocument();
    expect(screen.queryByRole("option", { name: /Rhine Traverse/ })).toBeNull();
  });

  it("opens the route named by Enter", async () => {
    show("/activities");
    await userEvent.click(screen.getByRole("button", { name: "Search" }));
    await userEvent.type(screen.getByRole("searchbox"), "kaiserstuhl");
    await userEvent.keyboard("{Enter}");

    expect(screen.getByTestId("landed")).toHaveTextContent("/routes/veloplanner/2/1");
  });

  it("lists an admin's drafts first, marked as drafts, until a filter narrows the search", async () => {
    show("/activities", { planner: true });
    await userEvent.click(screen.getByRole("button", { name: "Search" }));

    const options = screen.getAllByRole("option");
    expect(options[0]).toHaveTextContent("Saturday gravel");
    expect(within(options[0] as HTMLElement).getByText("Draft")).toBeInTheDocument();
    // A published plan is in the library, not among the drafts.
    expect(screen.queryByRole("option", { name: /Weekday loop/ })).toBeNull();

    await userEvent.type(searchbox(), "dist:>15");

    expect(screen.queryByText("Draft")).toBeNull();
  });

  it("shows a rider no drafts", async () => {
    show("/activities");
    await userEvent.click(screen.getByRole("button", { name: "Search" }));

    expect(screen.queryByText("Draft")).toBeNull();
    expect(screen.getAllByRole("option")).toHaveLength(LIBRARY.length);
  });

  it("orders by a measure in its natural direction, or the other way with asc", async () => {
    show("/activities");
    await userEvent.click(screen.getByRole("button", { name: "Search" }));

    await userEvent.type(searchbox(), "by distance");
    expect(titles()).toEqual([
      expect.stringContaining("Kaiserstuhl Loop"),
      expect.stringContaining("Rhine Traverse"),
    ]);

    await userEvent.type(searchbox(), " asc");
    expect(titles()).toEqual([
      expect.stringContaining("Rhine Traverse"),
      expect.stringContaining("Kaiserstuhl Loop"),
    ]);
  });

  it("orders by the first by, breaking its ties with each further one", async () => {
    show("/activities");
    await userEvent.click(screen.getByRole("button", { name: "Search" }));

    // By name alone Kaiserstuhl leads; the first key must win over the second.
    await userEvent.type(searchbox(), "by distance asc by name");
    expect(titles()).toEqual([
      expect.stringContaining("Rhine Traverse"),
      expect.stringContaining("Kaiserstuhl Loop"),
    ]);

    // Both routes climb the same, so here the second key decides.
    await userEvent.clear(searchbox());
    await userEvent.type(searchbox(), "by ascent by distance asc");
    expect(titles()).toEqual([
      expect.stringContaining("Rhine Traverse"),
      expect.stringContaining("Kaiserstuhl Loop"),
    ]);
    expect(screen.getAllByRole("button", { name: /^Remove by / })).toHaveLength(2);
  });

  it("narrows and orders by tokens typed into the query, each shown as a removable chip", async () => {
    show("/activities");
    await userEvent.click(screen.getByRole("button", { name: "Search" }));

    await userEvent.type(searchbox(), "dist:>15 by distance");
    expect(titles()).toEqual([expect.stringContaining("Kaiserstuhl Loop")]);

    await userEvent.click(screen.getByRole("button", { name: "Remove dist:>15" }));
    expect(searchbox()).toHaveValue("by distance");
    expect(titles()).toHaveLength(LIBRARY.length);
  });

  it("offers the filters and by before anything is typed", async () => {
    show("/activities");
    await userEvent.click(screen.getByRole("button", { name: "Search" }));

    const offered = within(screen.getByRole("group", { name: "Completions" }))
      .getAllByRole("button")
      .map((button) => button.textContent);
    expect(offered).toEqual([
      expect.stringMatching(/^dist:/),
      expect.stringMatching(/^up:/),
      expect.stringMatching(/^time:/),
      expect.stringMatching(/^src:/),
      expect.stringMatching(/^by/),
    ]);
  });

  it("chooses between completions with Right and Left at the end of the query", async () => {
    show("/activities", { planner: true });
    await userEvent.click(screen.getByRole("button", { name: "Search" }));
    await userEvent.type(searchbox(), "d");

    await userEvent.keyboard("{ArrowRight}");
    expect(screen.getByRole("button", { name: /^draft/, pressed: true })).toBeInTheDocument();
    await userEvent.keyboard("{ArrowLeft}");
    expect(screen.getByRole("button", { name: /^dist:/, pressed: true })).toBeInTheDocument();

    await userEvent.keyboard("{ArrowRight}{Tab}");
    // Finished, the token leaves the text for a chip in the field.
    expect(searchbox()).toHaveValue("");
    expect(screen.getByRole("button", { name: "Remove draft" })).toBeInTheDocument();
  });

  it("takes the last chip back into the text on Backspace in an empty field", async () => {
    show("/activities");
    await userEvent.click(screen.getByRole("button", { name: "Search" }));
    await userEvent.type(searchbox(), "dist:>15 ");
    expect(searchbox()).toHaveValue("");

    await userEvent.keyboard("{Backspace}");
    expect(searchbox()).toHaveValue("dist:>15");
    expect(screen.queryByRole("button", { name: "Remove dist:>15" })).toBeNull();
  });

  it("completes a started key with Tab", async () => {
    show("/activities");
    await userEvent.click(screen.getByRole("button", { name: "Search" }));

    await userEvent.type(searchbox(), "di");
    await userEvent.keyboard("{Tab}");
    expect(searchbox()).toHaveValue("dist:");
    // A half-typed token narrows nothing by name while it is being written.
    expect(titles()).toHaveLength(LIBRARY.length);
  });

  it("ranks nearest first once the reader's position is known, asking for every route's line", async () => {
    stubPosition();
    show("/activities", { geometry: true });
    await userEvent.click(screen.getByRole("button", { name: "Search" }));

    await userEvent.type(searchbox(), "by near");

    expect(titles()).toEqual([
      expect.stringContaining("Rhine Traverse"),
      expect.stringContaining("Kaiserstuhl Loop"),
    ]);
  });

  it("previews only the highlighted route, and only on a wide screen", async () => {
    stubViewport(false);
    show("/activities", { geometry: true });
    await userEvent.click(screen.getByRole("button", { name: "Search" }));

    expect(screen.getByTestId("library-map")).toBeInTheDocument();
    expect(drawn.keys).toEqual(["veloplanner/2/1"]);
  });

  it("keeps the preview map, and the last line, while the next route's line loads", async () => {
    stubViewport(false);
    stubPendingFetch();
    show("/activities", { geometry: "first" });
    await userEvent.click(screen.getByRole("button", { name: "Search" }));
    expect(drawn.keys).toEqual(["veloplanner/2/1"]);

    // The next row's line never arrives; a fresh map would start from the whole world.
    await userEvent.keyboard("{ArrowDown}");
    expect(screen.getByTestId("library-map")).toBeInTheDocument();
    expect(drawn.keys).toEqual(["veloplanner/2/1"]);
    expect(drawn.mounts).toBe(1);
  });

  it("mounts no preview map on a narrow screen", async () => {
    show("/activities", { geometry: true });
    await userEvent.click(screen.getByRole("button", { name: "Search" }));

    expect(screen.queryByTestId("library-map")).not.toBeInTheDocument();
  });

  it("keeps the query after the palette is closed and reopened", async () => {
    show("/activities");
    await userEvent.click(screen.getByRole("button", { name: "Search" }));
    await userEvent.type(screen.getByRole("searchbox"), "kaiserstuhl");

    await userEvent.keyboard("{Escape}");
    expect(screen.queryByRole("dialog")).toBeNull();

    await userEvent.click(screen.getByRole("button", { name: "Search" }));
    expect(screen.getByRole("searchbox")).toHaveValue("kaiserstuhl");
    expect(screen.queryByRole("option", { name: /Rhine Traverse/ })).toBeNull();
  });
});
