/**
 * The entry page, without a canvas.
 *
 * The map needs WebGL, so it is stood in for by a fake that records what it was
 * asked to draw. What is tested here is the agreement the page exists to keep:
 * the map, the column, and the selection are three views of one state, and they
 * cannot disagree about what the library is or which route is open.
 */

import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { render, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { MemoryRouter } from "react-router";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { routeGeometryQuery, routesQuery, statusQuery, webUIConfigQuery } from "../../api/queries";
import type {
  BoundingBox,
  Position,
  Route,
  RouteGeometry,
  RouteSurface,
  WebUIConfig,
} from "../../api/types";
import { routeKey } from "../../api/types";
import type { ThemeChoice } from "../../lib/theme";
import { focusThumb } from "../../test/filterPanel";

interface Drawing {
  keys: string[];
  pickedKey: string | null;
  bounds: BoundingBox | null;
  maxZoom: number | undefined;
  /** Whether the selected route's own layers were handed to the map. */
  overlaid: boolean;
  /** The line the map was told a pick would do nothing to. */
  inertKey: string | null;
  /** The basemap style the map was handed, and whether it counts as dark. */
  styleUrl: string;
  darkBasemap: boolean;
}

const drawn = vi.hoisted(() => ({ maps: [] as Drawing[] }));

vi.mock("./LibraryMap", () => ({
  LibraryMap: (props: {
    lines: Array<{ key: string }>;
    pickedKey: string | null;
    bounds: BoundingBox | null;
    maxZoom?: number;
    children?: unknown;
    onPick?: (key: string) => void;
    inertKey?: string | null;
    styleUrl: string;
    darkBasemap?: boolean;
  }) => {
    drawn.maps.push({
      keys: props.lines.map((line) => line.key),
      pickedKey: props.pickedKey,
      bounds: props.bounds,
      maxZoom: props.maxZoom,
      overlaid: Boolean(props.children),
      inertKey: props.inertKey ?? null,
      styleUrl: props.styleUrl,
      darkBasemap: props.darkBasemap ?? false,
    });

    // Pointing at a line, without a line to point at: one control per route,
    // calling exactly what the real map calls when a pointer lands on it.
    return (
      <div data-testid="library-map">
        {props.lines.map((line) => (
          <button key={line.key} type="button" onClick={() => props.onPick?.(line.key)}>
            {`point at ${line.key}`}
          </button>
        ))}
      </div>
    );
  },
}));

const { AtlasPage } = await import("./AtlasPage");

function route(
  sourceRouteId: number,
  stageOrder: number,
  sourceRouteName: string,
  routeName: string,
): Route {
  return {
    provider: "veloplanner",
    sourceRouteId,
    stageOrder,
    sourceRouteName,
    routeName,
    title: routeName ? `${sourceRouteName} — ${routeName}` : sourceRouteName,
    sourceRevision: "2026-08-17",
    contentHash: `hash-${sourceRouteId}-${stageOrder}`,
    distanceMetres: 10_000 * stageOrder + sourceRouteId,
    ascentMetres: 100 * stageOrder,
    descentMetres: 80 * stageOrder,
    maxGradientPercent: 8,
    pointCount: 100,
  };
}

const LIBRARY = [
  route(1, 1, "Rhine Traverse", "Valley floor"),
  route(1, 2, "Rhine Traverse", "Forest ramps"),
  route(2, 1, "Kaiserstuhl Loop", ""),
];

function geometry(_stage: Route, offset: number): RouteGeometry {
  const coordinates: Position[] = [
    [8 + offset, 49, 100],
    [8.05 + offset, 49.05, 140],
    [8.1 + offset, 49.1, 180],
  ];

  return {
    bbox: [8 + offset, 49, 8.1 + offset, 49.1],
    coordinates,
  };
}

/**
 * The listing, the map configuration and the geometry, all seeded. The page
 * fetches geometry per route, and a test that let those requests fail would be
 * asking what the page draws with nothing to draw.
 */
function renderPage(
  library: Route[] = LIBRARY,
  options: {
    geometryFor?: Route[];
    readAt?: string;
    at?: string;
    /** Surface classification for a route's geometry, by `routeKey`. */
    surfaceFor?: Record<string, RouteSurface>;
    basemaps?: WebUIConfig["basemaps"];
    themeChoice?: ThemeChoice;
  } = {},
) {
  const client = new QueryClient({
    defaultOptions: { queries: { retry: false, staleTime: Number.POSITIVE_INFINITY } },
  });
  client.setQueryData(routesQuery().queryKey, library);
  client.setQueryData(webUIConfigQuery().queryKey, {
    basemaps: options.basemaps ?? [
      { name: "Streets", styleUrl: "https://tiles.example/style.json", darkCartography: false },
    ],
    sourceBaseUrls: {},
    timezone: "Europe/Berlin",
    identity: { display: "rider@example.test", admin: false },
  });
  client.setQueryData(statusQuery().queryKey, {
    ready: true,
    converged: true,
    targets: [],
    sync: {
      state: "idle",
      sourceRoutes: library.length,
      created: 0,
      updated: 0,
      deleted: 0,
      phases: options.readAt
        ? {
            source: {
              lastCompletedAt: options.readAt,
              lastResult: "succeeded",
              sourceRoutes: library.length,
              created: 0,
              updated: 0,
              deleted: 0,
            },
          }
        : {},
      surface: { classified: 0, total: 0, incomplete: 0, enrichmentFailures: 0 },
    },
  });
  (options.geometryFor ?? library).forEach((entry, index) => {
    client.setQueryData(
      routeGeometryQuery(entry.provider, entry.sourceRouteId, entry.stageOrder).queryKey,
      {
        ...geometry(entry, index * 0.4),
        surface: options.surfaceFor?.[routeKey(entry)],
      },
    );
  });

  return render(
    <QueryClientProvider client={client}>
      <MemoryRouter initialEntries={[options.at ?? "/"]}>
        <AtlasPage themeChoice={options.themeChoice ?? "system"} />
      </MemoryRouter>
    </QueryClientProvider>,
  );
}

/** What the map was asked to draw on its most recent render. */
function lastDrawing(): Drawing {
  const last = drawn.maps.at(-1);
  if (!last) {
    throw new Error("expected the map to have been drawn");
  }

  return last;
}

async function searchBox() {
  const existing = screen.queryByRole("searchbox", { name: "Search the route library" });
  if (existing) {
    return existing;
  }

  await userEvent.click(screen.getByRole("button", { name: "Search the route library" }));

  return screen.getByRole("searchbox", { name: "Search the route library" });
}

/**
 * A `localStorage` for jsdom, which has none. See `basemap.test.ts` for why a
 * `Map` behind the two methods the hook uses is enough.
 */
function stubStorage(): void {
  const entries = new Map<string, string>();
  vi.stubGlobal("localStorage", {
    getItem: (key: string) => entries.get(key) ?? null,
    setItem: (key: string, value: string) => {
      entries.set(key, value);
    },
  });
}

/** A `navigator.geolocation` for jsdom, which has none. */
function stubGeolocation(outcome: "granted" | "denied"): {
  getCurrentPosition: ReturnType<typeof vi.fn>;
} {
  const getCurrentPosition = vi.fn(
    (
      found: (position: { coords: { latitude: number; longitude: number } }) => void,
      failed?: (error: unknown) => void,
    ) => {
      if (outcome === "granted") {
        found({ coords: { latitude: 49, longitude: 8 } });
      } else {
        failed?.(new Error("denied"));
      }
    },
  );
  vi.stubGlobal("navigator", { geolocation: { getCurrentPosition } });

  return { getCurrentPosition };
}

beforeEach(() => {
  drawn.maps = [];
  vi.stubGlobal(
    "fetch",
    vi.fn(async () => new Response("{}", { status: 404 })),
  );
});

afterEach(() => {
  vi.unstubAllGlobals();
});

describe("AtlasPage", () => {
  it("draws every route in the library on one map", () => {
    renderPage();

    expect(lastDrawing().keys).toEqual(["veloplanner/1/1", "veloplanner/1/2", "veloplanner/2/1"]);
    // Framed around the whole library, which is the union of what was drawn.
    expect(lastDrawing().bounds).toEqual([8, 49, 8.9, 49.1]);
  });

  // The default stub in src/test/setup.ts answers every media query "no", so
  // the system here prefers light without anything further being stubbed.
  const BASEMAP_WITH_DARK_STYLE = [
    {
      name: "Streets",
      styleUrl: "https://tiles.example/light.json",
      styleUrlDark: "https://tiles.example/dark.json",
      darkCartography: false,
    },
  ];

  it("picks the dark basemap for an explicit dark choice, system or not", () => {
    renderPage(LIBRARY, { basemaps: BASEMAP_WITH_DARK_STYLE, themeChoice: "dark" });

    expect(lastDrawing().styleUrl).toBe("https://tiles.example/dark.json");
    expect(lastDrawing().darkBasemap).toBe(true);
  });

  it("keeps the light basemap for an explicit light choice under a dark system", () => {
    vi.stubGlobal(
      "matchMedia",
      vi.fn((query: string) => ({
        matches: query.includes("dark"),
        media: query,
        addEventListener: () => {},
        removeEventListener: () => {},
      })),
    );

    renderPage(LIBRARY, { basemaps: BASEMAP_WITH_DARK_STYLE, themeChoice: "light" });

    expect(lastDrawing().styleUrl).toBe("https://tiles.example/light.json");
    expect(lastDrawing().darkBasemap).toBe(false);
  });

  /*
   * The page has a name a reader navigating by heading can land on, and it is
   * not drawn: the map is the title, and a bar across the top of a map is a bar
   * across the map.
   */
  it("names itself without putting a header over the cartography", () => {
    renderPage();

    const heading = screen.getByRole("heading", { level: 1, name: "Route library" });
    expect(heading).toHaveClass("visually-hidden");
  });

  it("keeps the library size in the expanded search prompt", async () => {
    renderPage();

    const search = await searchBox();
    expect(search).toHaveAttribute("placeholder", "Search 3 routes");
    await userEvent.type(search, "rhine");

    expect(search).toHaveValue("rhine");
  });

  // The map and the column are two views of one state. A search that narrows
  // one and not the other would be two answers to the same question.
  it("keeps the map whole while the column narrows", async () => {
    renderPage();
    await userEvent.type(await searchBox(), "kaiserstuhl");

    expect(screen.getByRole("button", { name: /Kaiserstuhl Loop/ })).toBeInTheDocument();
    expect(screen.queryByRole("button", { name: /Rhine Traverse/ })).toBeNull();
    expect(lastDrawing().keys).toEqual(["veloplanner/1/1", "veloplanner/1/2", "veloplanner/2/1"]);
  });

  // A numeric filter narrows the column exactly as a name does, without a
  // query having to be typed, and the map keeps drawing the whole library.
  it("narrows the column by a slider bound read off the listing", async () => {
    renderPage();
    await userEvent.click(screen.getByRole("button", { name: "Show the library filters" }));
    // The library ascends 100, 200 and 100 m, so the track runs to 200 m by 10 m.
    await focusThumb("Ascent min");
    await userEvent.keyboard("{ArrowRight}".repeat(11));

    expect(await screen.findByRole("button", { name: /Forest ramps/ })).toBeInTheDocument();
    expect(screen.queryByRole("button", { name: /Valley floor/ })).toBeNull();
    expect(screen.queryByRole("button", { name: /Kaiserstuhl Loop/ })).toBeNull();
    expect(lastDrawing().keys).toEqual(["veloplanner/1/1", "veloplanner/1/2", "veloplanner/2/1"]);
  });

  it("says so when a filter leaves nothing", async () => {
    renderPage();
    await userEvent.click(screen.getByRole("button", { name: "Show the library filters" }));
    await focusThumb("Ascent max");
    await userEvent.keyboard("{Home}");

    expect(await screen.findByText("Nothing here matches these filters.")).toBeInTheDocument();
  });

  /*
   * A route picked out of the column is a route the reader now wants to see the
   * shape of, so the camera follows the selection rather than staying on the
   * library.
   */
  it("picks a route out on the map and flies to it", async () => {
    renderPage();
    await userEvent.type(await searchBox(), "kaiserstuhl");
    await userEvent.click(screen.getByRole("button", { name: /Kaiserstuhl Loop/ }));

    expect(lastDrawing().pickedKey).toBe("veloplanner/2/1");
    expect(lastDrawing().bounds).toEqual([8.8, 49, 8.9, 49.1]);
    expect(screen.getByRole("button", { name: "Open route" })).toBeInTheDocument();
  });

  // A search that no longer holds the open route would leave its card expanded
  // in a column it is not in.
  it("closes the open route when the search moves on", async () => {
    renderPage();
    await userEvent.type(await searchBox(), "kaiserstuhl");
    await userEvent.click(screen.getByRole("button", { name: /Kaiserstuhl Loop/ }));
    await userEvent.clear(await searchBox());

    expect(lastDrawing().pickedKey).toBeNull();
    expect(screen.queryByRole("button", { name: "Open route" })).toBeNull();
  });

  // One timestamp for the whole library: the service reads it in a single pass,
  // so a per-route time would be the same time repeated.
  it("says when the library was read, from the read half's own last run", async () => {
    renderPage(LIBRARY, { readAt: "2026-08-18T06:30:00Z" });
    await userEvent.type(await searchBox(), "kaiserstuhl");
    await userEvent.click(screen.getByRole("button", { name: /Kaiserstuhl Loop/ }));

    expect(screen.getByText(/read /)).toBeInTheDocument();
  });

  /*
   * Geometry arrives per route and after the listing, so the map draws what has
   * come back and frames what it has. A route still in flight is simply not on
   * it yet.
   */
  it("draws the geometry that has arrived rather than waiting for all of it", () => {
    renderPage(LIBRARY, { geometryFor: [LIBRARY[0] as Route] });

    expect(lastDrawing().keys).toEqual(["veloplanner/1/1"]);
    expect(lastDrawing().bounds).toEqual([8, 49, 8.1, 49.1]);
  });

  /*
   * Framing is an imperative call on the map, so the camera moves whenever the
   * bounds it is given are a different object. A union rebuilt on every render
   * would snap the map back from wherever the reader had panned it, once per
   * keystroke.
   */
  it("hands the camera the same framing until the framing changes", async () => {
    renderPage();
    const before = lastDrawing().bounds;

    await userEvent.type(await searchBox(), "rhine");

    expect(drawn.maps.length).toBeGreaterThan(1);
    expect(lastDrawing().bounds).toBe(before);
  });

  /*
   * Geometry arrives one request at a time, so what has come back is a shorter
   * list than the library. The camera has to find the selected route's box by
   * its key: by position it would frame whichever route happened to be there.
   */
  it("frames the selected route even when routes above it are still in flight", async () => {
    renderPage(LIBRARY, { geometryFor: [LIBRARY[1] as Route, LIBRARY[2] as Route] });
    await userEvent.type(await searchBox(), "kaiserstuhl");
    await userEvent.click(screen.getByRole("button", { name: /Kaiserstuhl Loop/ }));

    expect(lastDrawing().pickedKey).toBe("veloplanner/2/1");
    expect(lastDrawing().bounds).toEqual([8.4, 49, 8.5, 49.1]);
  });

  /*
   * The map is the library, so a line on it is the route itself: pointing at
   * where a ride goes asks about that ride. It is the same two steps the column
   * has — the card first, the route second — because the lines cross and the
   * reader is panning across them.
   */
  it("shows the card of a route pointed at on the map", async () => {
    renderPage();
    await userEvent.click(screen.getByRole("button", { name: "point at veloplanner/2/1" }));

    expect(lastDrawing().pickedKey).toBe("veloplanner/2/1");
    expect(lastDrawing().bounds).toEqual([8.8, 49, 8.9, 49.1]);
    expect(screen.getByRole("button", { name: "Open route" })).toBeInTheDocument();
    expect(screen.getByRole("searchbox")).toBeInTheDocument();
  });

  // The second step, on the same line: the card said which route was hit, and
  // pointing at it again is the map's own way of saying yes.
  it("opens a route pointed at a second time", async () => {
    renderPage();
    await userEvent.click(screen.getByRole("button", { name: "point at veloplanner/2/1" }));

    expect(screen.queryByRole("region", { name: "Kaiserstuhl Loop" })).toBeNull();

    await userEvent.click(screen.getByRole("button", { name: "point at veloplanner/2/1" }));

    expect(screen.getByRole("region", { name: "Kaiserstuhl Loop" })).toBeInTheDocument();
    expect(lastDrawing().overlaid).toBe(true);
  });

  /*
   * The search is one way to a route and the map is another. A card that stayed
   * hidden behind a query it does not match would be a selection the reader can
   * see on the ground and nowhere else.
   */
  it("clears a search the route pointed at is not in", async () => {
    renderPage();
    await userEvent.type(await searchBox(), "kaiserstuhl");
    await userEvent.click(screen.getByRole("button", { name: "point at veloplanner/1/2" }));

    expect(screen.getByRole("searchbox")).toHaveValue("");
    expect(lastDrawing().pickedKey).toBe("veloplanner/1/2");
    expect(screen.getByRole("button", { name: "Open route" })).toBeInTheDocument();
  });

  // A search the route is already in is left alone: it is how the reader got
  // here, and clearing it would throw away the column they were comparing in.
  it("keeps a search the route pointed at is in", async () => {
    renderPage();
    await userEvent.type(await searchBox(), "rhine");
    await userEvent.click(screen.getByRole("button", { name: "point at veloplanner/1/2" }));

    expect(screen.getByRole("searchbox")).toHaveValue("rhine");
    expect(lastDrawing().pickedKey).toBe("veloplanner/1/2");
  });

  // The library goes away under an opened route, and the window before its
  // overlay is up is not a hole in that: a line hit there is not a way out.
  it("keeps the open route when another line is pointed at", async () => {
    renderPage(LIBRARY, { at: "/?route=veloplanner%2F2%2F1" });
    await userEvent.click(screen.getByRole("button", { name: "point at veloplanner/1/1" }));

    expect(screen.getByRole("region", { name: "Kaiserstuhl Loop" })).toBeInTheDocument();
    expect(screen.queryByRole("region", { name: "Rhine Traverse — Valley floor" })).toBeNull();
  });

  /*
   * The open route is already the answer, and reopening it would throw away
   * everything asked of it since — the stretch the chart is zoomed into most of
   * all, which is picked by dragging along that very line.
   */
  it("leaves the open route alone when its own line is pointed at", async () => {
    renderPage(LIBRARY, { at: "/?route=veloplanner%2F2%2F1" });
    const before = drawn.maps.length;
    await userEvent.click(screen.getByRole("button", { name: "point at veloplanner/2/1" }));

    expect(screen.getByRole("region", { name: "Kaiserstuhl Loop" })).toBeInTheDocument();
    expect(drawn.maps.length).toBe(before);
    // And the map says as much before it is clicked, by giving that one line no
    // pointer cursor.
    expect(lastDrawing().inertKey).toBe("veloplanner/2/1");
  });

  // The map with nothing on it is the loading state; a panel saying so would
  // cover the ground it is waiting to draw.
  it("frames nothing and says nothing while the library is on its way", () => {
    renderPage(LIBRARY, { geometryFor: [] });

    expect(lastDrawing().bounds).toBeNull();
    expect(screen.queryByText("No routes yet.")).toBeNull();
  });

  it("says an empty library is empty rather than showing a search over nothing", () => {
    renderPage([]);

    expect(screen.getByText("No routes yet.")).toBeInTheDocument();
    expect(screen.queryByRole("searchbox")).toBeNull();
  });

  /*
   * The route is not a page of its own: it takes over the column the search was
   * in, over the map the reader was already looking at, and the map keeps its
   * camera and gains the route's own layers rather than being mounted again.
   */
  it("swaps the search for the route in the same column", async () => {
    renderPage();
    await userEvent.type(await searchBox(), "kaiserstuhl");
    await userEvent.click(screen.getByRole("button", { name: /Kaiserstuhl Loop/ }));
    await userEvent.click(screen.getByRole("button", { name: "Open route" }));

    expect(screen.getByRole("region", { name: "Kaiserstuhl Loop" })).toBeInTheDocument();
    expect(screen.queryByRole("searchbox")).toBeNull();
    expect(lastDrawing().overlaid).toBe(true);
  });

  // The open route is in the address rather than in component state, so the view
  // a reader is looking at is a view they can send to someone else.
  it("opens the route the address names", () => {
    renderPage(LIBRARY, { at: "/?route=veloplanner%2F2%2F1" });

    expect(screen.getByRole("region", { name: "Kaiserstuhl Loop" })).toBeInTheDocument();
    expect(lastDrawing().overlaid).toBe(true);
  });

  // The address the app handed out before a second provider existed. A link
  // bookmarked or shared then names the provider it always meant, and still
  // opens the route it always did.
  it("opens the route a two-part address names", () => {
    renderPage(LIBRARY, { at: "/?route=2%2F1" });

    expect(screen.getByRole("region", { name: "Kaiserstuhl Loop" })).toBeInTheDocument();
    expect(lastDrawing().overlaid).toBe(true);
  });

  it("says so when the address names a route the library does not have", () => {
    renderPage(LIBRARY, { at: "/?route=veloplanner%2F99%2F1" });

    expect(screen.getByText("No route at that address.")).toBeInTheDocument();
  });

  // An identifier made of digits that still numbers nothing. Reading it as a
  // route would send the page looking for route zero of source route zero; it
  // is not a route the library is missing, it is not an address at all.
  it("ignores an address whose numbers name no route", () => {
    renderPage(LIBRARY, { at: "/?route=veloplanner%2F0%2F1" });

    expect(screen.queryByText("No route at that address.")).not.toBeInTheDocument();
    expect(screen.getByRole("button", { name: "Search the route library" })).toBeInTheDocument();
  });

  /*
   * The listing has the route and the geometry endpoint does not. A panel built
   * from no points is a title over an empty page, so the page says what happened
   * and leaves the search standing rather than drawing a route it cannot draw.
   */
  it("says so when the open route's geometry never arrives", async () => {
    renderPage(LIBRARY, {
      geometryFor: [LIBRARY[0] as Route, LIBRARY[1] as Route],
      at: "/?route=veloplanner%2F2%2F1",
    });

    expect(await screen.findByText("Could not load that route's geometry.")).toBeInTheDocument();
    expect(screen.queryByRole("region", { name: "Kaiserstuhl Loop" })).toBeNull();
    expect(screen.getByRole("button", { name: "Search the route library" })).toBeInTheDocument();
    expect(lastDrawing().overlaid).toBe(false);
  });

  it("goes back to the search it came from", async () => {
    renderPage(LIBRARY, { at: "/?route=veloplanner%2F2%2F1" });
    await userEvent.click(
      screen.getByRole("button", { name: /^Close the route and go back to \d+ routes?$/ }),
    );

    expect(screen.getByRole("button", { name: "Search the route library" })).toBeInTheDocument();
    expect(lastDrawing().overlaid).toBe(false);
  });

  /*
   * The profile is a panel across the bottom of the map, and the map is the
   * point: a reader who wants the ground back can put the chart away without
   * closing the route.
   */
  it("puts the detail dock away and leaves the route open", async () => {
    renderPage(LIBRARY, { at: "/?route=veloplanner%2F2%2F1" });
    await userEvent.click(screen.getByRole("button", { name: "Hide the route detail" }));

    expect(screen.getByRole("button", { name: "Show the profile" })).toBeInTheDocument();
    expect(screen.getByRole("region", { name: "Kaiserstuhl Loop" })).toBeInTheDocument();
  });

  // A route nobody has opened yet is new, in the reader's own browser only —
  // nothing about this reaches the service.
  it("marks a route new until it has been opened", async () => {
    stubStorage();
    renderPage();
    await userEvent.type(await searchBox(), "kaiserstuhl");

    expect(screen.getByText("New")).toBeInTheDocument();
  });

  it("stops marking a route new once its own panel has been opened", async () => {
    stubStorage();
    renderPage(LIBRARY, { at: "/?route=veloplanner%2F2%2F1" });
    await userEvent.click(
      screen.getByRole("button", { name: /^Close the route and go back to \d+ routes?$/ }),
    );
    await userEvent.type(await searchBox(), "kaiserstuhl");

    expect(screen.queryByText("New")).toBeNull();
  });

  // Opening it by pressing "Open route" is the same trigger as opening it by
  // address, so it must leave the same mark behind. The search query outlives
  // the round trip, so it is not retyped on the way back.
  it("stops marking a route new once it is opened by hand", async () => {
    stubStorage();
    renderPage();
    await userEvent.type(await searchBox(), "kaiserstuhl");
    await userEvent.click(screen.getByRole("button", { name: /Kaiserstuhl Loop/ }));
    await userEvent.click(screen.getByRole("button", { name: "Open route" }));
    await userEvent.click(
      screen.getByRole("button", { name: /^Close the route and go back to \d+ routes?$/ }),
    );

    expect(screen.queryByText("New")).toBeNull();
  });
});

describe("AtlasPage startup location", () => {
  it("frames the rider's own position, at no closer than the location zoom, when nothing else was asked for", async () => {
    stubGeolocation("granted");
    renderPage();

    await waitFor(() => expect(lastDrawing().bounds).toEqual([7.99, 48.99, 8.01, 49.01]));
    expect(lastDrawing().maxZoom).toBe(12);
  });

  it("falls back to the library's own bounds when geolocation fails", async () => {
    stubGeolocation("denied");
    renderPage();

    await waitFor(() => expect(lastDrawing().bounds).toEqual([8, 49, 8.9, 49.1]));
  });

  it("never asks for the rider's position when a deep link already names a route", async () => {
    const { getCurrentPosition } = stubGeolocation("granted");
    renderPage(LIBRARY, { at: "/?route=veloplanner%2F2%2F1" });

    expect(screen.getByRole("region", { name: "Kaiserstuhl Loop" })).toBeInTheDocument();
    expect(getCurrentPosition).not.toHaveBeenCalled();
  });
});
