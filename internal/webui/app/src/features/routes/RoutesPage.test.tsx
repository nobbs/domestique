/**
 * The entry page, without a canvas.
 *
 * The map needs WebGL, so it is stood in for by a fake that records what it was
 * asked to draw. What is tested here is the agreement the page exists to keep:
 * the map, the column, and the selection are three views of one state, and they
 * cannot disagree about what the library is or which route is open.
 */

import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { MemoryRouter } from "react-router";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import type { BoundingBox, Position, Route, RouteGeometry } from "../../api/types";

interface Drawing {
  keys: string[];
  selectedKey: string | null;
  bounds: BoundingBox | null;
  /** Whether the selected route's own layers were handed to the map. */
  overlaid: boolean;
}

const drawn = vi.hoisted(() => ({ maps: [] as Drawing[] }));

vi.mock("./LibraryMap", () => ({
  LibraryMap: (props: {
    lines: Array<{ key: string }>;
    selectedKey: string | null;
    bounds: BoundingBox | null;
    overlay?: unknown;
  }) => {
    drawn.maps.push({
      keys: props.lines.map((line) => line.key),
      selectedKey: props.selectedKey,
      bounds: props.bounds,
      overlaid: Boolean(props.overlay),
    });

    return <div data-testid="library-map" />;
  },
}));

const { RoutesPage } = await import("./RoutesPage");

function route(routeId: number, stageOrder: number, routeName: string, stageName: string): Route {
  return {
    routeId,
    stageOrder,
    routeName,
    stageName,
    title: stageName ? `${routeName} — ${stageName}` : routeName,
    sourceRevision: "2026-08-17",
    contentHash: `hash-${routeId}-${stageOrder}`,
    distanceMetres: 10_000 * stageOrder + routeId,
    ascentMetres: 100 * stageOrder,
    maxGradientPercent: 8,
    pointCount: 100,
  };
}

const LIBRARY = [
  route(1, 1, "Rhine Traverse", "Valley floor"),
  route(1, 2, "Rhine Traverse", "Forest ramps"),
  route(2, 1, "Kaiserstuhl Loop", ""),
];

function geometry(stage: Route, offset: number): RouteGeometry {
  const coordinates: Position[] = [
    [8 + offset, 49, 100],
    [8.05 + offset, 49.05, 140],
    [8.1 + offset, 49.1, 180],
  ];

  return {
    stage,
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
  options: { geometryFor?: Route[]; readAt?: string; at?: string } = {},
) {
  const client = new QueryClient({
    defaultOptions: { queries: { retry: false, staleTime: Number.POSITIVE_INFINITY } },
  });
  client.setQueryData(["stages"], library);
  client.setQueryData(["webui-config"], { tileStyleUrl: "https://tiles.example/style.json" });
  client.setQueryData(["status"], {
    ready: true,
    converged: true,
    targets: [],
    sync: {
      state: "idle",
      sourceStages: library.length,
      created: 0,
      updated: 0,
      deleted: 0,
      schedule: { source: true, targets: true },
      phases: options.readAt
        ? {
            source: {
              lastCompletedAt: options.readAt,
              lastResult: "succeeded",
              sourceStages: library.length,
              created: 0,
              updated: 0,
              deleted: 0,
            },
          }
        : {},
      surface: { classified: 0, total: 0 },
    },
  });
  (options.geometryFor ?? library).forEach((entry, index) => {
    client.setQueryData(
      ["stage-geometry", entry.routeId, entry.stageOrder],
      geometry(entry, index * 0.4),
    );
  });

  return render(
    <QueryClientProvider client={client}>
      <MemoryRouter initialEntries={[options.at ?? "/"]}>
        <RoutesPage />
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

describe("RoutesPage", () => {
  it("draws every route in the library on one map", () => {
    renderPage();

    expect(lastDrawing().keys).toEqual(["1/1", "1/2", "2/1"]);
    // Framed around the whole library, which is the union of what was drawn.
    expect(lastDrawing().bounds).toEqual([8, 49, 8.9, 49.1]);
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

  it("counts the whole library in the search pill, however narrow the result", async () => {
    renderPage();

    expect(screen.getByRole("searchbox")).toHaveAttribute("placeholder", "Search 3 routes");
    await userEvent.type(screen.getByRole("searchbox"), "rhine");

    expect(screen.getByText("2 of 3")).toBeInTheDocument();
  });

  // The map and the column are two views of one state. A search that narrows
  // one and not the other would be two answers to the same question.
  it("keeps the map whole while the column narrows", async () => {
    renderPage();
    await userEvent.type(screen.getByRole("searchbox"), "kaiserstuhl");

    expect(screen.getByRole("button", { name: /Kaiserstuhl Loop/ })).toBeInTheDocument();
    expect(screen.queryByRole("button", { name: /Rhine Traverse/ })).toBeNull();
    expect(lastDrawing().keys).toEqual(["1/1", "1/2", "2/1"]);
  });

  /*
   * A route picked out of the column is a route the reader now wants to see the
   * shape of, so the camera follows the selection rather than staying on the
   * library.
   */
  it("picks a route out on the map and flies to it", async () => {
    renderPage();
    await userEvent.type(screen.getByRole("searchbox"), "kaiserstuhl");
    await userEvent.click(screen.getByRole("button", { name: /Kaiserstuhl Loop/ }));

    expect(lastDrawing().selectedKey).toBe("2/1");
    expect(lastDrawing().bounds).toEqual([8.8, 49, 8.9, 49.1]);
    expect(screen.getByRole("button", { name: "Open route" })).toBeInTheDocument();
  });

  // A search that no longer holds the open route would leave its card expanded
  // in a column it is not in.
  it("closes the open route when the search moves on", async () => {
    renderPage();
    await userEvent.type(screen.getByRole("searchbox"), "kaiserstuhl");
    await userEvent.click(screen.getByRole("button", { name: /Kaiserstuhl Loop/ }));
    await userEvent.clear(screen.getByRole("searchbox"));

    expect(lastDrawing().selectedKey).toBeNull();
    expect(screen.queryByRole("button", { name: "Open route" })).toBeNull();
  });

  // One timestamp for the whole library: the service reads it in a single pass,
  // so a per-route time would be the same time repeated.
  it("says when the library was read, from the read half's own last run", async () => {
    renderPage(LIBRARY, { readAt: "2026-08-18T06:30:00Z" });
    await userEvent.type(screen.getByRole("searchbox"), "kaiserstuhl");
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

    expect(lastDrawing().keys).toEqual(["1/1"]);
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

    await userEvent.type(screen.getByRole("searchbox"), "rhine");

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
    await userEvent.type(screen.getByRole("searchbox"), "kaiserstuhl");
    await userEvent.click(screen.getByRole("button", { name: /Kaiserstuhl Loop/ }));

    expect(lastDrawing().selectedKey).toBe("2/1");
    expect(lastDrawing().bounds).toEqual([8.4, 49, 8.5, 49.1]);
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
    await userEvent.type(screen.getByRole("searchbox"), "kaiserstuhl");
    await userEvent.click(screen.getByRole("button", { name: /Kaiserstuhl Loop/ }));
    await userEvent.click(screen.getByRole("button", { name: "Open route" }));

    expect(screen.getByRole("region", { name: "Kaiserstuhl Loop" })).toBeInTheDocument();
    expect(screen.queryByRole("searchbox")).toBeNull();
    expect(lastDrawing().overlaid).toBe(true);
  });

  // The open route is in the address rather than in component state, so the view
  // a reader is looking at is a view they can send to someone else.
  it("opens the route the address names", () => {
    renderPage(LIBRARY, { at: "/?route=2%2F1" });

    expect(screen.getByRole("region", { name: "Kaiserstuhl Loop" })).toBeInTheDocument();
    expect(lastDrawing().overlaid).toBe(true);
  });

  it("says so when the address names a route the library does not have", () => {
    renderPage(LIBRARY, { at: "/?route=99%2F1" });

    expect(screen.getByText("No route at that address.")).toBeInTheDocument();
  });

  it("goes back to the search it came from", async () => {
    renderPage(LIBRARY, { at: "/?route=2%2F1" });
    await userEvent.click(screen.getByRole("button", { name: /^← Search \d+ routes?$/ }));

    expect(screen.getByRole("searchbox")).toBeInTheDocument();
    expect(lastDrawing().overlaid).toBe(false);
  });

  /*
   * The profile is a panel across the bottom of the map, and the map is the
   * point: a reader who wants the ground back can put the chart away without
   * closing the route.
   */
  it("puts the profile away and leaves the route open", async () => {
    renderPage(LIBRARY, { at: "/?route=2%2F1" });
    await userEvent.click(screen.getByRole("button", { name: "Hide the profile" }));

    expect(screen.getByRole("button", { name: "Show the profile" })).toBeInTheDocument();
    expect(screen.getByRole("region", { name: "Kaiserstuhl Loop" })).toBeInTheDocument();
  });
});
