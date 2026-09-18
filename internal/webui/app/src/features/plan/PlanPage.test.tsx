import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { act, fireEvent, render, screen, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { MemoryRouter, Route, Routes } from "react-router";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { haversineMetres } from "../../lib/profile";

const preview = vi.hoisted(() => vi.fn());
const create = vi.hoisted(() => vi.fn());
const replace = vi.hoisted(() => vi.fn());
const remove = vi.hoisted(() => vi.fn());
const openedPlan = vi.hoisted(() => ({ value: {} }));
const mapPoint = vi.hoisted(() => ({ value: { longitude: 8, latitude: 49 } }));
const narrowViewport = vi.hoisted(() => ({ value: false }));
const routeOverlay = vi.hoisted(() => vi.fn());
// Keyed by "longitude,latitude" as last rendered, so a test can drive a
// marker's drag without a real pointer gesture on a mocked map.
const markerDragHandlers = vi.hoisted(
  () => new Map<string, (event: { lngLat: { lng: number; lat: number } }) => void>(),
);
// Answers every waypoint unmoved unless a test says otherwise.
const searchPicks = vi.hoisted(() => ({
  value: [] as Array<{ name: string; longitude: number; latitude: number }>,
}));
const snap = vi.hoisted(() =>
  vi.fn(async (at: { longitude: number; latitude: number }) => ({
    data: { ...at, snapped: false },
  })),
);

vi.mock("../../api/generated", async (importOriginal) => ({
  ...(await importOriginal<typeof import("../../api/generated")>()),
  getGetPlanQueryKey: (id: number) => ["plan", id],
  useCreatePlan: () => ({ isPending: false, mutateAsync: create }),
  useGetPlan: () => openedPlan.value,
  useDeletePlan: () => ({ isPending: false, mutateAsync: remove }),
  usePreviewPlanRoute: () => ({ mutate: preview }),
  useReplacePlan: () => ({ isPending: false, mutateAsync: replace }),
  snapPlace: snap,
  // Rows ask for their names once a geocoder is configured; none is known here.
  getReversePlaceQueryOptions: (
    params: unknown,
    options: { query: { select: (response: unknown) => string } },
  ) => ({
    queryKey: ["place", params],
    queryFn: async () => ({ data: {} }),
    select: options.query.select,
  }),
}));
vi.mock("../../api/queries", () => ({
  webUIConfigQuery: () => ({ queryKey: ["config"], queryFn: vi.fn() }),
}));
vi.mock("../../components/Layout", () => ({
  PageShell: ({ children }: { children: React.ReactNode }) => <main>{children}</main>,
  Layout: ({
    map,
    children,
    dock,
    workspaceLabel,
    workspace = "overlay",
  }: {
    map: React.ReactNode;
    children: React.ReactNode;
    dock: React.ReactNode;
    workspaceLabel: string;
    workspace?: "overlay" | "sidebar";
  }) => (
    <main>
      {workspace === "sidebar" ? <aside aria-label={workspaceLabel}>{children}</aside> : null}
      {map}
      <div className="shell__overlay">
        {workspace === "sidebar" ? null : <aside aria-label={workspaceLabel}>{children}</aside>}
        {workspace === "sidebar" ? null : dock}
      </div>
      {workspace === "sidebar" ? dock : null}
    </main>
  ),
}));
vi.mock("../../lib/mediaQuery", async (importOriginal) => ({
  ...(await importOriginal<typeof import("../../lib/mediaQuery")>()),
  useNarrowViewport: () => narrowViewport.value,
}));
vi.mock("../../components/map/MapWidget", () => ({
  MapWidget: ({
    children,
    furniture,
    onClick,
    onMouseDown,
    onMoveStart,
  }: {
    children: React.ReactNode;
    furniture?: React.ReactNode;
    onClick?: (event: {
      lngLat: { lng: number; lat: number };
      originalEvent: { altKey: boolean };
    }) => void;
    onMouseDown?: (event: {
      lngLat: { lng: number; lat: number };
      originalEvent: { altKey: boolean; button: number; target: EventTarget | null };
    }) => void;
    onMoveStart?: () => void;
  }) => (
    <>
      <button
        type="button"
        aria-label="Plan route map"
        onMouseDown={(event) =>
          onMouseDown?.({
            lngLat: { lng: mapPoint.value.longitude, lat: mapPoint.value.latitude },
            originalEvent: { altKey: event.altKey, button: event.button, target: event.target },
          })
        }
        onClick={(event) =>
          onClick?.({
            lngLat: { lng: mapPoint.value.longitude, lat: mapPoint.value.latitude },
            originalEvent: { altKey: event.altKey },
          })
        }
      >
        {children}
      </button>
      <button type="button" aria-label="Pan map" onClick={() => onMoveStart?.()} />
      {furniture}
    </>
  ),
}));
vi.mock("../../components/map/MapControls", () => ({
  MapControls: ({ children }: { children?: React.ReactNode }) => (
    <div data-testid="plan-map-controls">{children}</div>
  ),
}));
vi.mock("../../components/map/BasemapPicker", () => ({
  BasemapPicker: () => <span data-testid="plan-basemap-picker" />,
}));
vi.mock("../../components/map/MapViewport", () => ({
  MapViewport: ({ bounds, fitRevision }: { bounds: unknown; fitRevision: number }) => (
    <output data-testid="plan-viewport" data-fit-revision={fitRevision}>
      {JSON.stringify(bounds)}
    </output>
  ),
}));
vi.mock("react-map-gl/maplibre", () => ({
  Marker: ({
    children,
    longitude,
    latitude,
    onDragEnd,
  }: {
    children: React.ReactNode;
    longitude: number;
    latitude: number;
    onDragEnd?: (event: { lngLat: { lng: number; lat: number } }) => void;
  }) => {
    if (onDragEnd) {
      markerDragHandlers.set(`${longitude},${latitude}`, onDragEnd);
    }
    return children;
  },
  Source: ({ id, data, children }: { id: string; data: unknown; children: React.ReactNode }) => (
    <div data-testid={id} data-geometry={JSON.stringify(data)}>
      {children}
    </div>
  ),
  Layer: () => null,
  ScaleControl: ({ position, unit }: { position: string; unit: string }) => (
    <output data-testid="plan-scale" data-position={position} data-unit={unit} />
  ),
}));
vi.mock("../routes/RouteOverlay", () => ({
  RouteOverlay: ({
    coordinates,
    showTerminals,
    surface,
  }: {
    coordinates: unknown[];
    showTerminals?: boolean;
    surface?: unknown;
  }) => {
    routeOverlay({ coordinates, showTerminals, surface });
    return <output data-testid="route-line">{coordinates.length}</output>;
  },
}));
vi.mock("../routes/ElevationProfile", () => ({
  ElevationProfile: () => <div>elevation profile</div>,
}));
// Covered on its own in PlaceSearch.test.tsx; here it only hands over what was picked.
vi.mock("./PlaceSearch", () => ({
  PlaceSearch: ({ onAdd }: { onAdd: (places: typeof searchPicks.value) => void }) => (
    <button type="button" onClick={() => onAdd(searchPicks.value)}>
      Search places
    </button>
  ),
}));
// Covered on its own in PlanDelivery.test.tsx; this suite only needs to know
// whether the header renders it, not the delivery query behind it.
vi.mock("./PlanDelivery", () => ({
  PlanDeliveryTrigger: ({ planId }: { planId: number | null }) =>
    planId === null ? null : <span data-testid="plan-delivery-trigger" />,
}));

const { PlanPage } = await import("./PlanPage");

function renderPage(
  path: string | { pathname: string; state: unknown } = "/plan",
  config: { placeNames?: boolean } = {},
) {
  const client = new QueryClient({
    defaultOptions: { queries: { retry: false, staleTime: Number.POSITIVE_INFINITY } },
  });
  client.setQueryData(["config"], {
    basemaps: [
      { name: "Streets", styleUrl: "https://tiles.example/style.json", darkCartography: false },
    ],
    sourceBaseUrls: {},
    timezone: "Europe/Berlin",
    identity: { display: "admin@example.test", admin: true },
    planning: true,
    ...config,
  });

  return render(
    <QueryClientProvider client={client}>
      <MemoryRouter initialEntries={[path]}>
        <Routes>
          <Route path="/plan" element={<PlanPage />} />
          <Route path="/plan/:planId" element={<PlanPage />} />
        </Routes>
      </MemoryRouter>
    </QueryClientProvider>,
  );
}

beforeEach(() => {
  vi.useFakeTimers();
  preview.mockReset();
  create.mockReset();
  replace.mockReset();
  remove.mockReset();
  routeOverlay.mockReset();
  openedPlan.value = {};
  mapPoint.value = { longitude: 8, latitude: 49 };
  markerDragHandlers.clear();
});

afterEach(() => vi.useRealTimers());

/** The waypoint rows as they read, newest markup: one line per stop. */
function waypointRows(): string[] {
  return within(screen.getByRole("list", { name: "Waypoints" }))
    .getAllByRole("listitem")
    .map((row) => row.textContent?.replace(/\s+/g, " ").trim() ?? "");
}

/** The coordinate a row shows, which is what an ungeocoded waypoint reads as. */
function firstWaypointCoordinates(): string {
  return waypointRows()[0] ?? "";
}

describe("edgeSpeed", () => {
  it("scrolls up near the top, down near the bottom, and not in between", async () => {
    const { edgeSpeed } = await import("./PlannerSidebar");

    expect(edgeSpeed(100, 500, 300)).toBe(0);
    expect(edgeSpeed(100, 500, 110)).toBeLessThan(0);
    expect(edgeSpeed(100, 500, 490)).toBeGreaterThan(0);
    // Hardest at the very edge, gentler a row in.
    expect(edgeSpeed(100, 500, 100)).toBeLessThan(edgeSpeed(100, 500, 130));
    expect(edgeSpeed(100, 500, 500)).toBeGreaterThan(edgeSpeed(100, 500, 470));
  });
});

describe("PlanPage", () => {
  it("shows the delivery trigger only once a plan is saved", async () => {
    vi.useRealTimers();
    renderPage();
    expect(screen.queryByTestId("plan-delivery-trigger")).toBeNull();

    openedPlan.value = {
      data: {
        data: {
          id: 4,
          name: "Stored loop",
          profile: "trekking",
          cues: false,
          published: false,
          version: 2,
          waypoints: [
            { longitude: 8, latitude: 49 },
            { longitude: 8.1, latitude: 49.1 },
          ],
          geometry: {
            type: "LineString",
            coordinates: [
              [8, 49],
              [8.1, 49.1],
            ],
          },
          distanceMetres: 10_000,
          ascentMetres: 100,
          createdAt: "2026-09-15T09:00:00Z",
          updatedAt: "2026-09-15T09:00:00Z",
        },
      },
    };
    renderPage("/plan/4");
    await act(async () => {});
    expect(screen.getByTestId("plan-delivery-trigger")).toBeInTheDocument();
  });

  it("renders the planner with a mocked map and labels drafts", async () => {
    vi.useRealTimers();
    renderPage();

    expect(screen.getByRole("button", { name: "Plan route map" })).toBeInTheDocument();
    await userEvent.click(screen.getByRole("button", { name: "Plans" }));
    expect(await screen.findByRole("menuitem", { name: "New plan" })).toHaveAttribute(
      "href",
      "/plan",
    );
    // An unsaved plan has nothing to delete.
    expect(screen.queryByRole("menuitem", { name: /Delete this plan/ })).toBeNull();
    expect(screen.getByRole("button", { name: "Draft" })).toHaveAttribute("aria-pressed", "true");
    expect(screen.getByTestId("plan-map-controls")).toBeInTheDocument();
    expect(screen.getByTestId("plan-basemap-picker")).toBeInTheDocument();
    expect(screen.getByTestId("plan-scale")).toHaveAttribute("data-position", "bottom-left");
    expect(screen.getByTestId("plan-scale")).toHaveAttribute("data-unit", "metric");
  });

  it("traces a copied route round by round, keeping only the ones routing needs", async () => {
    // A router that knows no roads: every leg is the straight line between its waypoints.
    preview.mockImplementation(
      (
        variables: { data: { waypoints: { longitude: number; latitude: number }[] } },
        callbacks: { onSuccess: (value: unknown) => void },
      ) => {
        const coordinates = variables.data.waypoints.map(({ longitude, latitude }) => [
          longitude,
          latitude,
        ]);
        let distance = 0;
        const waypointProgress = coordinates.map((position, index) => {
          const previous = coordinates[index - 1];
          distance += previous
            ? haversineMetres(previous as [number, number], position as [number, number])
            : 0;
          return { distanceMetres: distance };
        });
        callbacks.onSuccess({
          data: {
            geometry: { type: "LineString", coordinates },
            distanceMetres: distance,
            ascentMetres: 0,
            waypointProgress,
          },
        });
      },
    );
    const route = [
      ...Array.from({ length: 40 }, (_, index) => [8 + index / 1000, 49]),
      ...Array.from({ length: 41 }, (_, index) => [8.04, 49 + index / 1000]),
    ];
    renderPage({
      pathname: "/plan",
      state: {
        name: "Corner",
        profile: "trekking",
        waypoints: [
          { longitude: 8, latitude: 49 },
          { longitude: 8.04, latitude: 49.04 },
        ],
        trace: { route, indices: [0, 80] },
      },
    });
    await act(async () => {});
    expect(screen.getByText("Tracing the copied route with 2 waypoints…")).toHaveAttribute(
      "role",
      "status",
    );

    for (let round = 0; round < 6 && screen.queryByText(/Tracing the copied route/); round++) {
      act(() => vi.advanceTimersByTime(300));
    }

    expect(screen.queryByText(/Tracing the copied route/)).not.toBeInTheDocument();
    expect(waypointRows()).toHaveLength(3);
    expect(waypointRows()[1]).toContain("49.0000, 8.0400");
    expect(preview).toHaveBeenCalledTimes(4);
    expect(create).not.toHaveBeenCalled();
  });

  it("stops tracing a copied route when the routing engine refuses, keeping its waypoints", async () => {
    preview.mockImplementation(
      (_variables: unknown, callbacks: { onError: (error: Error) => void }) =>
        callbacks.onError(new Error("unavailable")),
    );
    renderPage({
      pathname: "/plan",
      state: {
        name: "Corner",
        profile: "trekking",
        waypoints: [
          { longitude: 8, latitude: 49 },
          { longitude: 8.04, latitude: 49.04 },
        ],
        trace: {
          route: [
            [8, 49],
            [8.04, 49],
            [8.04, 49.04],
          ],
          indices: [0, 2],
        },
      },
    });
    await act(async () => {});
    expect(screen.getByText("Tracing the copied route with 2 waypoints…")).toBeInTheDocument();

    act(() => vi.advanceTimersByTime(300));

    expect(screen.queryByText(/Tracing the copied route/)).not.toBeInTheDocument();
    expect(screen.getByText("Preview unavailable")).toBeInTheDocument();
    expect(waypointRows()).toHaveLength(2);

    // The copied route stays on the map to compare against, until it is toggled off.
    const toggle = screen.getByRole("button", { name: "Show the copied route" });
    expect(toggle).toHaveAttribute("aria-pressed", "true");
    expect(
      JSON.parse(screen.getByTestId("plan-copied-route").dataset.geometry ?? "{}").geometry
        .coordinates,
    ).toHaveLength(3);
    fireEvent.click(toggle);
    expect(toggle).toHaveAttribute("aria-pressed", "false");
    expect(screen.queryByTestId("plan-copied-route")).not.toBeInTheDocument();
  });

  it("initializes a new draft from a copied route seed without saving it", async () => {
    preview.mockImplementation(
      (
        _variables: unknown,
        callbacks: {
          onSuccess: (value: {
            data: {
              geometry: { type: "LineString"; coordinates: number[][] };
              distanceMetres: number;
              ascentMetres: number;
            };
          }) => void;
        },
      ) =>
        callbacks.onSuccess({
          data: {
            geometry: {
              type: "LineString",
              coordinates: [
                [8, 49, 100],
                [8.1, 49.1, 200],
              ],
            },
            distanceMetres: 10_000,
            ascentMetres: 100,
          },
        }),
    );
    renderPage({
      pathname: "/plan",
      state: {
        name: "Alpine loop — Descent",
        profile: "trekking",
        waypoints: [
          { longitude: 8, latitude: 49 },
          { longitude: 8.1, latitude: 49.1 },
        ],
      },
    });
    await act(async () => {});

    expect(screen.getByLabelText("Plan name")).toHaveValue("Alpine loop — Descent");
    expect(waypointRows()[0]).toContain("49.0000, 8.0000");
    expect(waypointRows()[1]).toContain("49.1000, 8.1000");
    expect(screen.getByTestId("plan-viewport")).toHaveTextContent("[8,49,8.1,49.1]");
    expect(screen.getByTestId("plan-viewport")).toHaveAttribute("data-fit-revision", "1");
    expect(screen.queryByRole("button", { name: "Show the copied route" })).not.toBeInTheDocument();
    expect(create).not.toHaveBeenCalled();

    act(() => vi.advanceTimersByTime(300));
    expect(screen.getByText("elevation profile")).toBeInTheDocument();
    expect(screen.getByTestId("plan-viewport")).toHaveAttribute("data-fit-revision", "2");
  });

  it("frames a blank draft on the rider's own position", async () => {
    vi.stubGlobal("navigator", {
      geolocation: {
        getCurrentPosition: (
          found: (position: { coords: { latitude: number; longitude: number } }) => void,
        ) => found({ coords: { latitude: 49, longitude: 8 } }),
      },
    });
    try {
      renderPage();
      await act(async () => {});

      expect(screen.getByTestId("plan-viewport")).toHaveTextContent("[7.99,48.99,8.01,49.01]");
      fireEvent.click(screen.getByRole("button", { name: "Plan route map" }));

      expect(screen.getByTestId("plan-viewport")).toHaveTextContent("null");
    } finally {
      vi.unstubAllGlobals();
    }
  });

  it("folds and reopens the elevation panel", () => {
    renderPage();

    expect(screen.getByText("elevation profile")).toBeInTheDocument();
    expect(screen.getByTestId("plan-viewport")).toHaveAttribute("data-fit-revision", "1");
    fireEvent.click(screen.getByRole("button", { name: "Hide the route detail" }));
    expect(screen.getByRole("button", { name: "Show the route detail" })).toBeInTheDocument();
    expect(screen.queryByText("elevation profile")).toBeNull();
    expect(screen.getByTestId("plan-viewport")).toHaveAttribute("data-fit-revision", "0");
    fireEvent.click(screen.getByRole("button", { name: "Show the route detail" }));
    expect(screen.getByRole("button", { name: "Hide the route detail" })).toBeInTheDocument();
    expect(screen.getByText("elevation profile")).toBeInTheDocument();
    expect(screen.getByTestId("plan-viewport")).toHaveAttribute("data-fit-revision", "1");
  });

  it("does not re-frame for the elevation panel while it lives in the Drawer", () => {
    narrowViewport.value = true;
    try {
      renderPage();

      expect(screen.getByTestId("plan-viewport")).toHaveAttribute("data-fit-revision", "0");
      fireEvent.click(screen.getByRole("button", { name: "Hide the route detail" }));
      expect(screen.getByTestId("plan-viewport")).toHaveAttribute("data-fit-revision", "0");
    } finally {
      narrowViewport.value = false;
    }
  });

  it("keeps history controls on the map beside the planner and dispatches their actions", () => {
    renderPage();
    const history = screen.getByRole("group", { name: "Planner history" });
    const undo = screen.getByRole("button", { name: "Undo" });
    const redo = screen.getByRole("button", { name: "Redo" });
    const reverse = screen.getByRole("button", { name: "Reverse" });

    expect(history).toHaveAttribute("data-orientation", "horizontal");
    expect(history.parentElement).toHaveClass("absolute");
    expect(document.querySelector(".shell__overlay")).not.toContainElement(
      screen.getByRole("complementary", { name: "Route planner controls" }),
    );
    expect(document.querySelector(".shell__overlay")).not.toContainElement(
      screen.getByRole("region", { name: "Planned route" }),
    );
    expect(
      screen.getByRole("complementary", { name: "Route planner controls" }),
    ).not.toContainElement(history);
    expect(undo).toBeDisabled();
    expect(redo).toBeDisabled();
    expect(reverse).toBeDisabled();
    expect(undo).toHaveAttribute("title", "Undo");
    expect(redo).toHaveAttribute("title", "Redo");
    expect(reverse).toHaveAttribute("title", "Reverse");

    const map = screen.getByRole("button", { name: "Plan route map" });
    fireEvent.click(map);
    mapPoint.value = { longitude: 8.1, latitude: 49.1 };
    fireEvent.click(map);
    expect(undo).toBeEnabled();
    expect(reverse).toBeEnabled();

    fireEvent.click(reverse);
    expect(firstWaypointCoordinates()).toContain("49.1000, 8.1000");
    fireEvent.click(undo);
    expect(firstWaypointCoordinates()).toContain("49.0000, 8.0000");
    fireEvent.click(redo);
    expect(firstWaypointCoordinates()).toContain("49.1000, 8.1000");
  });

  it("routes one preview after a burst and leaves the last good line up after a failure", () => {
    let failed = false;
    preview.mockImplementation(
      (
        _variables: unknown,
        callbacks: {
          onSuccess: (value: {
            data: {
              geometry: { type: "LineString"; coordinates: number[][] };
              distanceMetres: number;
              ascentMetres: number;
              waypointProgress: { distanceMetres: number }[];
            };
          }) => void;
          onError: (error: Error) => void;
        },
      ) => {
        if (failed) {
          callbacks.onError(new Error("Routing unavailable"));
        } else {
          callbacks.onSuccess({
            data: {
              geometry: {
                type: "LineString",
                coordinates: [
                  [8, 49],
                  [8.1, 49.1],
                ],
              },
              distanceMetres: 10_000,
              ascentMetres: 100,
              waypointProgress: [{ distanceMetres: 0 }, { distanceMetres: 13_400 }],
            },
          });
        }
      },
    );
    renderPage();

    fireEvent.click(screen.getByRole("button", { name: "Plan route map" }));
    fireEvent.click(screen.getByRole("button", { name: "Plan route map" }));
    act(() => vi.advanceTimersByTime(299));
    expect(preview).not.toHaveBeenCalled();
    act(() => vi.advanceTimersByTime(1));
    // The route shows once its legs have morphed out of their straight provisional lines.
    act(() => vi.advanceTimersByTime(500));

    expect(preview).toHaveBeenCalledOnce();
    expect(screen.getByTestId("route-line")).toHaveTextContent("2");
    expect(screen.getByTestId("plan-viewport")).toHaveTextContent("null");
    expect(routeOverlay).toHaveBeenCalledWith(expect.objectContaining({ showTerminals: false }));
    expect(routeOverlay).toHaveBeenLastCalledWith(expect.objectContaining({ surface: undefined }));
    expect(screen.getByLabelText("Planned route summary")).toHaveTextContent("10.0 km");
    expect(screen.getByLabelText("Planned route summary")).toHaveTextContent("100 m");
    failed = true;
    mapPoint.value = { longitude: 8.2, latitude: 49.2 };
    fireEvent.click(screen.getByRole("button", { name: "Plan route map" }), { altKey: true });
    act(() => vi.advanceTimersByTime(300));

    expect(screen.getByText("Routing unavailable")).toBeInTheDocument();
    // The routed leg keeps its shape; the unroutable one stays a straight provisional line.
    const drawn = JSON.parse(screen.getByTestId("plan-transition").dataset.geometry ?? "{}");
    expect(
      drawn.features.map((leg: { properties: { provisional: boolean } }) => leg.properties),
    ).toEqual([{ provisional: false }, { provisional: true }]);
  });

  it("passes classified preview ranges to the route overlay", () => {
    const ranges = [{ kind: "gravel" as const, startIndex: 0, endIndex: 1 }];
    preview.mockImplementation(
      (
        _variables: unknown,
        callbacks: {
          onSuccess: (value: {
            data: {
              geometry: { type: "LineString"; coordinates: number[][] };
              distanceMetres: number;
              ascentMetres: number;
              surface: { ranges: typeof ranges; matchedMetres: number };
            };
          }) => void;
        },
      ) =>
        callbacks.onSuccess({
          data: {
            geometry: {
              type: "LineString",
              coordinates: [
                [8, 49],
                [8.1, 49.1],
              ],
            },
            distanceMetres: 10_000,
            ascentMetres: 100,
            surface: { ranges, matchedMetres: 10_000 },
          },
        }),
    );
    renderPage();

    fireEvent.click(screen.getByRole("button", { name: "Plan route map" }));
    fireEvent.click(screen.getByRole("button", { name: "Plan route map" }));
    act(() => vi.advanceTimersByTime(300));
    // The route shows once its legs have morphed out of their straight provisional lines.
    act(() => vi.advanceTimersByTime(500));

    expect(routeOverlay).toHaveBeenLastCalledWith(expect.objectContaining({ surface: ranges }));
  });

  it("offers the ground stop only where the plan's surface was classified", () => {
    const ranges = [{ kind: "gravel" as const, startIndex: 0, endIndex: 1 }];
    const answer = (surface: { ranges: typeof ranges; matchedMetres: number } | undefined) =>
      preview.mockImplementation(
        (_variables: unknown, callbacks: { onSuccess: (value: unknown) => void }) =>
          callbacks.onSuccess({
            data: {
              geometry: {
                type: "LineString",
                coordinates: [
                  [8, 49, 100],
                  [8.1, 49.1, 140],
                ],
              },
              distanceMetres: 10_000,
              ascentMetres: 100,
              ...(surface === undefined ? {} : { surface }),
            },
          }),
      );

    answer(undefined);
    const { unmount } = renderPage();
    fireEvent.click(screen.getByRole("button", { name: "Plan route map" }));
    fireEvent.click(screen.getByRole("button", { name: "Plan route map" }));
    act(() => vi.advanceTimersByTime(300));
    // One reading is not a choice, so the switcher stays away.
    expect(screen.queryByRole("tab", { name: "Ground" })).toBeNull();
    unmount();

    answer({ ranges, matchedMetres: 10_000 });
    renderPage();
    fireEvent.click(screen.getByRole("button", { name: "Plan route map" }));
    fireEvent.click(screen.getByRole("button", { name: "Plan route map" }));
    act(() => vi.advanceTimersByTime(300));

    expect(screen.getByRole("tab", { name: "Profile" })).toBeInTheDocument();
    fireEvent.click(screen.getByRole("tab", { name: "Ground" }));
    // The ribbon labels the stretch and the table measures it.
    expect(screen.getAllByText("Gravel").length).toBeGreaterThan(1);
  });

  it("marks where the rider walks, on the strip and dashed over the route", () => {
    preview.mockImplementation(
      (_variables: unknown, callbacks: { onSuccess: (value: unknown) => void }) =>
        callbacks.onSuccess({
          data: {
            geometry: {
              type: "LineString",
              coordinates: [
                [8, 49, 100],
                [8.001, 49, 100],
                [8.002, 49, 100],
                [8.003, 49, 100],
              ],
            },
            distanceMetres: 219,
            ascentMetres: 0,
            // The two middle vertices sit about 73 m and 146 m along.
            pushing: [{ startMetres: 73, endMetres: 145 }],
          },
        }),
    );
    renderPage();

    fireEvent.click(screen.getByRole("button", { name: "Plan route map" }));
    fireEvent.click(screen.getByRole("button", { name: "Plan route map" }));
    act(() => vi.advanceTimersByTime(300));
    // The route shows once its legs have morphed out of their straight provisional lines.
    act(() => vi.advanceTimersByTime(500));

    expect(screen.getByLabelText("Planned route summary")).toHaveTextContent("72 m");
    const walked = JSON.parse(
      screen.getByTestId("plan-pushing").getAttribute("data-geometry") ?? "{}",
    ) as { geometry: { coordinates: number[][][] } };
    // The middle third of the line, and only that.
    expect(walked.geometry.coordinates).toEqual([
      [
        [8.001, 49, 100],
        [8.002, 49, 100],
      ],
    ]);
  });

  it("draws no walked line on a route ridden throughout", () => {
    preview.mockImplementation(
      (_variables: unknown, callbacks: { onSuccess: (value: unknown) => void }) =>
        callbacks.onSuccess({
          data: {
            geometry: {
              type: "LineString",
              coordinates: [
                [8, 49],
                [8.1, 49.1],
              ],
            },
            distanceMetres: 10_000,
            ascentMetres: 100,
          },
        }),
    );
    renderPage();

    fireEvent.click(screen.getByRole("button", { name: "Plan route map" }));
    fireEvent.click(screen.getByRole("button", { name: "Plan route map" }));
    act(() => vi.advanceTimersByTime(300));

    expect(screen.queryByTestId("plan-pushing")).toBeNull();
    expect(screen.queryByLabelText("Walked")).toBeNull();
  });

  it("leaves the route unpainted when the classification matched nothing", () => {
    preview.mockImplementation(
      (
        _variables: unknown,
        callbacks: {
          onSuccess: (value: {
            data: {
              geometry: { type: "LineString"; coordinates: number[][] };
              distanceMetres: number;
              ascentMetres: number;
              surface: { ranges: unknown[]; matchedMetres: number };
            };
          }) => void;
        },
      ) =>
        callbacks.onSuccess({
          data: {
            geometry: {
              type: "LineString",
              coordinates: [
                [8, 49],
                [8.1, 49.1],
              ],
            },
            distanceMetres: 10_000,
            ascentMetres: 100,
            surface: {
              ranges: [{ kind: "unknown", startIndex: 0, endIndex: 1 }],
              matchedMetres: 0,
            },
          },
        }),
    );
    renderPage();

    fireEvent.click(screen.getByRole("button", { name: "Plan route map" }));
    fireEvent.click(screen.getByRole("button", { name: "Plan route map" }));
    act(() => vi.advanceTimersByTime(300));
    // The route shows once its legs have morphed out of their straight provisional lines.
    act(() => vi.advanceTimersByTime(500));

    expect(routeOverlay).toHaveBeenLastCalledWith(expect.objectContaining({ surface: undefined }));
  });

  it("shows a save failure instead of leaving a rejected action behind", async () => {
    create.mockRejectedValue(new Error("Save unavailable"));
    renderPage();

    fireEvent.change(screen.getByLabelText("Plan name"), { target: { value: "Draft" } });
    fireEvent.click(screen.getByRole("button", { name: "Plan route map" }));
    fireEvent.click(screen.getByRole("button", { name: "Plan route map" }));
    fireEvent.click(screen.getByRole("button", { name: "Save" }));

    await act(async () => {});
    expect(screen.getByRole("alert")).toHaveTextContent("Save unavailable");
  });

  it("starts a new draft after leaving an opened plan", async () => {
    const savedSurface = [{ kind: "asphalt" as const, startIndex: 0, endIndex: 1 }];
    openedPlan.value = {
      data: {
        data: {
          id: 4,
          name: "Stored loop",
          profile: "trekking",
          cues: false,
          published: false,
          version: 2,
          waypoints: [
            { longitude: 8, latitude: 49 },
            { longitude: 8.1, latitude: 49.1 },
          ],
          geometry: {
            type: "LineString",
            coordinates: [
              [8, 49],
              [8.1, 49.1],
            ],
          },
          distanceMetres: 10_000,
          ascentMetres: 100,
          surface: { ranges: savedSurface, matchedMetres: 10_000 },
          createdAt: "2026-09-15T09:00:00Z",
          updatedAt: "2026-09-15T09:00:00Z",
        },
      },
    };
    preview.mockImplementation(
      (_variables: unknown, callbacks: { onError: (error: Error) => void }) =>
        callbacks.onError(new Error("Stored route could not refresh")),
    );
    renderPage({
      pathname: "/plan/4",
      state: {
        name: "Copied route",
        profile: "trekking",
        waypoints: [
          { longitude: 8.5, latitude: 49.5 },
          { longitude: 8.6, latitude: 49.6 },
        ],
      },
    });
    await act(async () => {});
    act(() => vi.advanceTimersByTime(300));

    expect(screen.getByDisplayValue("Stored loop")).toBeInTheDocument();
    expect(screen.getByLabelText("Planned route summary")).toHaveTextContent("10.0 km");
    expect(screen.getByLabelText("Planned route summary")).toHaveTextContent("100 m");
    expect(screen.getByText("Stored route could not refresh")).toBeInTheDocument();
    expect(screen.getByTestId("route-line")).toHaveTextContent("2");
    expect(routeOverlay).toHaveBeenLastCalledWith(
      expect.objectContaining({ surface: savedSurface }),
    );
    expect(screen.getByTestId("plan-viewport")).toHaveTextContent("[8,49,8.1,49.1]");
    mapPoint.value = { longitude: 8.05, latitude: 49.05 };
    fireEvent.click(screen.getByRole("button", { name: "Plan route map" }));
    act(() => vi.advanceTimersByTime(300));

    expect(screen.getByTestId("plan-viewport")).toHaveTextContent("[8,49,8.1,49.1]");
    vi.stubGlobal("navigator", {
      geolocation: {
        getCurrentPosition: (
          found: (position: { coords: { latitude: number; longitude: number } }) => void,
        ) => found({ coords: { latitude: 49, longitude: 8 } }),
      },
    });
    try {
      fireEvent.click(screen.getByRole("button", { name: "Plans" }));
      await act(async () => {});
      fireEvent.click(screen.getByRole("menuitem", { name: "New plan" }));
      await act(async () => {});

      expect(screen.getByDisplayValue("")).toBeInTheDocument();
      expect(screen.queryByLabelText("Planned route summary")).toBeNull();
      expect(screen.getByTestId("plan-viewport")).toHaveTextContent("[7.99,48.99,8.01,49.01]");
    } finally {
      vi.unstubAllGlobals();
    }
  });

  it("waits for an opened plan and keeps its controls out of a failed load", () => {
    openedPlan.value = { isPending: true };
    const { rerender } = renderPage("/plan/4");

    expect(screen.getByText("Loading plan…")).toBeInTheDocument();
    expect(screen.queryByRole("button", { name: "Save changes" })).toBeNull();

    openedPlan.value = { isError: true, error: new Error("Plan not found") };
    rerender(
      <QueryClientProvider client={new QueryClient()}>
        <MemoryRouter initialEntries={["/plan/4"]}>
          <Routes>
            <Route path="/plan/:planId" element={<PlanPage />} />
          </Routes>
        </MemoryRouter>
      </QueryClientProvider>,
    );
    expect(screen.getByText("Plan not found")).toBeInTheDocument();
    expect(screen.queryByRole("button", { name: "Save changes" })).toBeNull();
  });

  it("uses saved plan versions for subsequent replacements", async () => {
    const storedPlan = {
      id: 4,
      name: "Stored loop",
      profile: "trekking" as const,
      cues: false,
      published: false,
      version: 2,
      waypoints: [
        { longitude: 8, latitude: 49 },
        { longitude: 8.1, latitude: 49.1 },
      ],
      geometry: {
        type: "LineString" as const,
        coordinates: [
          [8, 49],
          [8.1, 49.1],
        ],
      },
      distanceMetres: 10_000,
      ascentMetres: 100,
      createdAt: "2026-09-15T09:00:00Z",
      updatedAt: "2026-09-15T09:00:00Z",
    };
    openedPlan.value = {
      data: {
        data: storedPlan,
      },
    };
    replace
      .mockResolvedValueOnce({ data: { ...storedPlan, version: 3 } })
      .mockResolvedValueOnce({ data: { ...storedPlan, version: 4, published: true } });
    renderPage("/plan/4");
    await act(async () => {});

    expect(screen.getByRole("button", { name: "Save" })).toBeDisabled();
    fireEvent.change(screen.getByLabelText("Plan name"), { target: { value: "Renamed" } });
    fireEvent.click(screen.getByRole("button", { name: "Save" }));
    await act(async () => {});
    // Publishing takes effect only on the save that follows it.
    fireEvent.click(screen.getByRole("button", { name: "Published" }));
    expect(replace).toHaveBeenCalledOnce();
    fireEvent.click(screen.getByRole("button", { name: "Save" }));
    await act(async () => {});

    expect(replace.mock.calls[0]?.[0]).toMatchObject({ headers: { "If-Match": "2" } });
    expect(replace.mock.calls[1]?.[0]).toMatchObject({
      headers: { "If-Match": "3" },
      data: expect.objectContaining({ published: true }),
    });
  });

  it("shows the turn cues switch with the loaded plan's turn count and sends it on save", async () => {
    const storedPlan = {
      id: 4,
      name: "Stored loop",
      profile: "trekking" as const,
      cues: false,
      published: false,
      version: 2,
      waypoints: [
        { longitude: 8, latitude: 49 },
        { longitude: 8.1, latitude: 49.1 },
      ],
      geometry: {
        type: "LineString" as const,
        coordinates: [
          [8, 49],
          [8.1, 49.1],
        ],
      },
      distanceMetres: 10_000,
      ascentMetres: 100,
      turnCount: 12,
      createdAt: "2026-09-15T09:00:00Z",
      updatedAt: "2026-09-15T09:00:00Z",
    };
    openedPlan.value = { data: { data: storedPlan } };
    replace.mockResolvedValueOnce({ data: { ...storedPlan, version: 3, cues: true } });
    renderPage("/plan/4");
    await act(async () => {});

    const toggle = screen.getByRole("button", { name: "Turn cues" });
    expect(toggle).toHaveAttribute("aria-pressed", "false");
    expect(screen.getByRole("button", { name: "Save" })).toBeDisabled();

    fireEvent.click(toggle);
    expect(toggle).toHaveAttribute("aria-pressed", "true");
    expect(toggle).toHaveTextContent("Turn cues · 12");
    expect(screen.getByRole("button", { name: "Save" })).toBeEnabled();

    fireEvent.click(screen.getByRole("button", { name: "Save" }));
    await act(async () => {});

    expect(replace.mock.calls[0]?.[0]).toMatchObject({
      data: expect.objectContaining({ cues: true }),
    });
  });

  it("does not reroute after changing only the plan name", () => {
    renderPage();
    fireEvent.click(screen.getByRole("button", { name: "Plan route map" }));
    fireEvent.click(screen.getByRole("button", { name: "Plan route map" }));
    act(() => vi.advanceTimersByTime(300));
    expect(preview).toHaveBeenCalledOnce();

    fireEvent.change(screen.getByLabelText("Plan name"), { target: { value: "No detour" } });
    act(() => vi.advanceTimersByTime(300));

    expect(preview).toHaveBeenCalledOnce();
  });

  it("routes with the profile chosen from the route type control", () => {
    renderPage();
    fireEvent.click(screen.getByRole("button", { name: "Route type" }));
    act(() => vi.advanceTimersByTime(0));
    fireEvent.click(screen.getByRole("menuitem", { name: "Gravel" }));
    fireEvent.click(screen.getByRole("button", { name: "Plan route map" }));
    fireEvent.click(screen.getByRole("button", { name: "Plan route map" }));
    act(() => vi.advanceTimersByTime(300));

    expect(preview).toHaveBeenCalledWith(
      expect.objectContaining({ data: expect.objectContaining({ profile: "gravel" }) }),
      expect.anything(),
    );
  });

  it("marks the first waypoint as start, the last as finish, and the middle ones by number", () => {
    renderPage();
    const map = screen.getByRole("button", { name: "Plan route map" });

    fireEvent.click(map);
    expect(
      screen.getByRole("group", { name: "Drag Start waypoint to reorder" }),
    ).not.toHaveAttribute("title");
    expect(screen.getByRole("img", { name: "Start waypoint" })).toBeInTheDocument();
    expect(screen.queryByRole("img", { name: "Finish waypoint" })).toBeNull();

    mapPoint.value = { longitude: 8.1, latitude: 49 };
    fireEvent.click(map);
    expect(screen.getByRole("img", { name: "Finish waypoint" })).toBeInTheDocument();
    expect(
      screen.getByRole("group", { name: "Drag Finish waypoint to reorder" }),
    ).toBeInTheDocument();

    mapPoint.value = { longitude: 8.2, latitude: 49 };
    fireEvent.click(map);
    expect(screen.getByRole("img", { name: "Waypoint 2" })).toHaveTextContent("2");
    expect(screen.getByRole("group", { name: "Drag Waypoint 2 to reorder" })).toHaveTextContent(
      "2",
    );
    expect(screen.getByRole("img", { name: "Finish waypoint" })).toBeInTheDocument();
  });

  it("settles a clicked waypoint onto the road beside it, as one step to undo", async () => {
    snap.mockImplementationOnce(async () => ({
      data: { longitude: 8.0004, latitude: 49.0003, snapped: true },
    }));
    renderPage();

    fireEvent.click(screen.getByRole("button", { name: "Plan route map" }));
    await act(async () => {});

    expect(snap).toHaveBeenCalledWith({ longitude: 8, latitude: 49 });
    expect(waypointRows()[0]).toContain("49.0003, 8.0004");
    fireEvent.click(screen.getByRole("button", { name: "Undo" }));
    expect(screen.queryAllByRole("listitem")).toHaveLength(0);
  });

  it("asks to settle a click on a wrapped world at the longitude it stands for", async () => {
    renderPage();

    mapPoint.value = { longitude: 368, latitude: 49 };
    fireEvent.click(screen.getByRole("button", { name: "Plan route map" }));
    await act(async () => {});

    expect(snap).toHaveBeenCalledWith({ longitude: 8, latitude: 49 });
  });

  it("inserts map clicks into the nearest leg and appends at the final endpoint, with Alt, or before two waypoints", () => {
    renderPage();
    const map = screen.getByRole("button", { name: "Plan route map" });

    mapPoint.value = { longitude: 8, latitude: 49 };
    fireEvent.click(map);
    mapPoint.value = { longitude: 8.2, latitude: 49 };
    fireEvent.click(map);
    expect(waypointRows()[1]).toContain("49.0000, 8.2000");

    mapPoint.value = { longitude: 8.1, latitude: 49.01 };
    fireEvent.click(map);
    expect(waypointRows()[1]).toContain("49.0100, 8.1000");
    expect(waypointRows()[2]).toContain("49.0000, 8.2000");

    mapPoint.value = { longitude: 9, latitude: 50 };
    fireEvent.click(map);
    expect(waypointRows()[3]).toContain("50.0000, 9.0000");

    mapPoint.value = { longitude: 10, latitude: 50 };
    fireEvent.click(map, { altKey: true });
    expect(waypointRows()[4]).toContain("50.0000, 10.0000");
    act(() => vi.advanceTimersByTime(300));

    expect(preview).toHaveBeenCalledWith(
      expect.objectContaining({
        data: expect.objectContaining({
          waypoints: [
            { longitude: 8, latitude: 49 },
            { longitude: 8.1, latitude: 49.01 },
            { longitude: 8.2, latitude: 49 },
            { longitude: 9, latitude: 50 },
            { longitude: 10, latitude: 50 },
          ],
        }),
      }),
      expect.anything(),
    );
  });

  it("offers a place search only where a geocoder is configured", () => {
    renderPage();
    expect(screen.queryByRole("button", { name: "Search places" })).toBeNull();
  });

  it("adds searched places along the route, each settled onto the road, as one step", async () => {
    renderPage("/plan", { placeNames: true });
    const map = screen.getByRole("button", { name: "Plan route map" });
    mapPoint.value = { longitude: 8, latitude: 49 };
    fireEvent.click(map);
    mapPoint.value = { longitude: 8, latitude: 49.3 };
    fireEvent.click(map);
    snap.mockClear();

    searchPicks.value = [
      { name: "Late", longitude: 8.01, latitude: 49.25 },
      { name: "Early", longitude: 8.01, latitude: 49.05 },
    ];
    fireEvent.click(screen.getByRole("button", { name: "Search places" }));
    await act(async () => {});

    const rows = waypointRows();
    expect(rows).toHaveLength(4);
    expect(rows[1]).toContain("49.0500, 8.0100");
    expect(rows[2]).toContain("49.2500, 8.0100");
    expect(rows[3]).toContain("49.3000, 8.0000");
    expect(snap).toHaveBeenCalledTimes(2);
    expect(snap).toHaveBeenCalledWith({ longitude: 8.01, latitude: 49.05 });

    fireEvent.click(screen.getByRole("button", { name: "Undo" }));
    expect(waypointRows()).toHaveLength(2);
  });

  it("previews, commits, and cancels full-row waypoint reordering from its grip affordance", async () => {
    openedPlan.value = {
      data: {
        data: {
          id: 4,
          name: "Stored loop",
          profile: "trekking",
          cues: false,
          published: false,
          version: 2,
          waypoints: [
            { longitude: 8, latitude: 49 },
            { longitude: 8.1, latitude: 49.1 },
            { longitude: 8.2, latitude: 49.2 },
          ],
          geometry: {
            type: "LineString",
            coordinates: [
              [8, 49],
              [8.1, 49.1],
              [8.2, 49.2],
            ],
          },
          distanceMetres: 10_000,
          ascentMetres: 100,
          createdAt: "2026-09-15T09:00:00Z",
          updatedAt: "2026-09-15T09:00:00Z",
        },
      },
    };
    renderPage("/plan/4");
    await act(async () => {});
    act(() => vi.advanceTimersByTime(300));
    preview.mockClear();

    const first = screen.getByRole("group", { name: "Drag Start waypoint to reorder" });
    const third = screen.getByRole("group", { name: "Drag Finish waypoint to reorder" });
    const rows = () =>
      Array.from(screen.getByRole("list", { name: "Waypoints" }).querySelectorAll("li"));
    const rowIDs = () => rows().map((row) => row.getAttribute("data-waypoint-id"));
    expect(first).not.toHaveAttribute("title", "the row carries no tooltip over the map");
    expect(first).toHaveAttribute("draggable", "true");
    expect(first).toHaveClass("flex", "min-w-0", "flex-1", "items-center");
    expect(first.querySelector(".tabler-icon-grip-vertical")).toBeInTheDocument();
    expect(screen.getByRole("button", { name: "Delete waypoint 1" }).previousElementSibling).toBe(
      first,
    );
    expect(screen.getByRole("button", { name: "Move Waypoint 2 up" })).toBeInTheDocument();

    const list = screen.getByRole("list", { name: "Waypoints" });
    expect(list).not.toHaveAttribute("data-dragging");

    const dataTransfer = { effectAllowed: "", setData: vi.fn(), setDragImage: vi.fn() };
    fireEvent.dragStart(first, { dataTransfer });
    // Snapping fights the browser's own scrolling while a row is in hand.
    expect(list).toHaveAttribute("data-dragging");
    expect(dataTransfer.setDragImage).toHaveBeenCalledWith(
      first,
      expect.any(Number),
      expect.any(Number),
    );
    fireEvent.dragOver(third);
    expect(rowIDs()).toEqual(["1", "2", "0"]);
    act(() => vi.advanceTimersByTime(300));
    expect(preview).not.toHaveBeenCalled();
    fireEvent.dragEnd(first);
    expect(rowIDs()).toEqual(["0", "1", "2"]);
    fireEvent.dragStart(screen.getByRole("button", { name: "Delete waypoint 1" }));
    expect(rowIDs()).toEqual(["0", "1", "2"]);

    fireEvent.dragStart(first);
    fireEvent.dragOver(third);
    fireEvent.drop(third);
    act(() => vi.advanceTimersByTime(300));

    expect(rowIDs()).toEqual(["1", "2", "0"]);
    expect(firstWaypointCoordinates()).toContain("49.1000, 8.1000");
    expect(preview).toHaveBeenCalledWith(
      expect.objectContaining({
        data: expect.objectContaining({
          waypoints: [
            { longitude: 8.1, latitude: 49.1 },
            { longitude: 8.2, latitude: 49.2 },
            { longitude: 8, latitude: 49 },
          ],
        }),
      }),
      expect.anything(),
    );
  });

  it("arms avoid mode from the map control and adds an area instead of a waypoint", () => {
    renderPage();
    const map = screen.getByRole("button", { name: "Plan route map" });

    fireEvent.click(map);
    mapPoint.value = { longitude: 8.2, latitude: 49.2 };
    fireEvent.click(map);
    expect(waypointRows()).toHaveLength(2);

    mapPoint.value = { longitude: 8.5, latitude: 49.5 };
    fireEvent.click(screen.getByRole("button", { name: "Avoid an area" }));
    fireEvent.click(map);

    expect(waypointRows()).toHaveLength(2);
    expect(screen.getByText("1 area")).toBeInTheDocument();
    expect(screen.getByRole("button", { name: "Avoid an area" })).toBeInTheDocument();

    act(() => vi.advanceTimersByTime(300));
    expect(preview).toHaveBeenCalledWith(
      expect.objectContaining({
        data: expect.objectContaining({
          avoid: [{ longitude: 8.5, latitude: 49.5, radiusMetres: 250 }],
        }),
      }),
      expect.anything(),
    );
  });

  it("disarms avoid mode on Escape without adding an area", () => {
    renderPage();
    fireEvent.click(screen.getByRole("button", { name: "Avoid an area" }));

    fireEvent.keyDown(document, { key: "Escape" });
    fireEvent.click(screen.getByRole("button", { name: "Plan route map" }));

    expect(screen.queryByRole("button", { name: "Delete avoided area" })).toBeNull();
    expect(waypointRows()).toHaveLength(1);
  });

  it("sends a waypoint's straight flag and skips settling a straight waypoint dragged along the map", async () => {
    openedPlan.value = {
      data: {
        data: {
          id: 4,
          name: "Stored loop",
          profile: "trekking",
          cues: false,
          published: false,
          version: 2,
          waypoints: [
            { longitude: 8, latitude: 49 },
            { longitude: 8.1, latitude: 49.1, straight: true },
          ],
          geometry: {
            type: "LineString",
            coordinates: [
              [8, 49],
              [8.1, 49.1],
            ],
          },
          distanceMetres: 10_000,
          ascentMetres: 100,
          createdAt: "2026-09-15T09:00:00Z",
          updatedAt: "2026-09-15T09:00:00Z",
        },
      },
    };
    renderPage("/plan/4");
    await act(async () => {});
    act(() => vi.advanceTimersByTime(300));

    expect(
      screen.getByRole("button", { name: "Route to Finish waypoint normally" }),
    ).toBeInTheDocument();
    expect(preview).toHaveBeenCalledWith(
      expect.objectContaining({
        data: expect.objectContaining({
          waypoints: [
            { longitude: 8, latitude: 49 },
            { longitude: 8.1, latitude: 49.1, straight: true },
          ],
        }),
      }),
      expect.anything(),
    );

    snap.mockClear();
    act(() => markerDragHandlers.get("8.1,49.1")?.({ lngLat: { lng: 8.1005, lat: 49.1005 } }));
    expect(snap).not.toHaveBeenCalled();
    expect(waypointRows()[1]).toContain("49.1005, 8.1005");
  });

  it("toggles a waypoint's straight leg from its row, never offering it on the first waypoint", () => {
    renderPage();
    const map = screen.getByRole("button", { name: "Plan route map" });
    fireEvent.click(map);
    mapPoint.value = { longitude: 8.1, latitude: 49.1 };
    fireEvent.click(map);

    expect(screen.queryByRole("button", { name: /Straight line to Start waypoint/ })).toBeNull();
    const toggle = screen.getByRole("button", { name: "Straight line to Finish waypoint" });
    fireEvent.click(toggle);
    expect(
      screen.getByRole("button", { name: "Route to Finish waypoint normally" }),
    ).toBeInTheDocument();
    expect(
      screen.getByRole("button", { name: "Route to Finish waypoint normally" }),
    ).toHaveAttribute("aria-pressed", "true");
  });

  it("folds a long plan to its ends and the waypoint in focus, lighting a folded run on hover", async () => {
    const count = 10;
    const stops = Array.from({ length: count }, (_, index) => ({
      longitude: 8 + index * 0.01,
      latitude: 49,
    }));
    openedPlan.value = {
      data: {
        data: {
          id: 4,
          name: "Long loop",
          profile: "trekking",
          cues: false,
          published: false,
          version: 2,
          waypoints: stops,
          geometry: {
            type: "LineString",
            coordinates: stops.map(({ longitude, latitude }) => [longitude, latitude]),
          },
          distanceMetres: 6_600,
          ascentMetres: 10,
          waypointProgress: stops.map((_, index) => ({ distanceMetres: index * 730 })),
          createdAt: "2026-09-15T09:00:00Z",
          updatedAt: "2026-09-15T09:00:00Z",
        },
      },
    };
    renderPage("/plan/4");
    await act(async () => {});
    const rowIDs = () =>
      Array.from(screen.getByRole("list", { name: "Waypoints" }).querySelectorAll("li")).map(
        (row) => row.getAttribute("data-waypoint-id") ?? "gap",
      );
    const litLine = () =>
      JSON.parse(screen.getByTestId("plan-hidden-run").dataset.geometry ?? "{}").features[0]
        ?.geometry.coordinates ?? [];

    expect(rowIDs()).toEqual(["0", "gap", "9"]);
    const run = screen.getByRole("button", { name: /8 more waypoints/ });
    fireEvent.mouseEnter(run);
    expect(litLine().length).toBeGreaterThan(1);
    fireEvent.mouseLeave(run);
    expect(litLine()).toEqual([]);

    fireEvent.mouseEnter(screen.getByRole("img", { name: "Waypoint 5" }));
    expect(rowIDs()).toEqual(["0", "gap", "3", "4", "5", "gap", "9"]);

    fireEvent.click(screen.getByRole("button", { name: /2 more waypoints/ }));
    expect(rowIDs()).toEqual(["0", "1", "2", "3", "4", "5", "gap", "9"]);
    // A new focus folds what was opened again.
    fireEvent.mouseEnter(screen.getByRole("img", { name: "Waypoint 8" }));
    expect(rowIDs()).toEqual(["0", "gap", "6", "7", "8", "9"]);
  });

  it("deletes a saved plan after confirming, then starts a new one", async () => {
    vi.useRealTimers();
    openedPlan.value = {
      data: {
        data: {
          id: 4,
          name: "Stored loop",
          profile: "trekking",
          cues: false,
          published: true,
          version: 2,
          waypoints: [
            { longitude: 8, latitude: 49 },
            { longitude: 8.1, latitude: 49.1 },
          ],
          geometry: { type: "LineString", coordinates: [] },
          distanceMetres: 10_000,
          ascentMetres: 100,
          createdAt: "2026-09-15T09:00:00Z",
          updatedAt: "2026-09-15T09:00:00Z",
        },
      },
    };
    remove
      .mockRejectedValueOnce(new Error("Plan changed since it was read"))
      .mockResolvedValue({ status: 204 });
    renderPage("/plan/4");
    await act(async () => {});

    await userEvent.click(screen.getByRole("button", { name: "Plans" }));
    await userEvent.click(await screen.findByRole("menuitem", { name: "Delete this plan…" }));
    const dialog = await screen.findByRole("alertdialog");
    expect(dialog).toHaveTextContent(
      "It is removed from every rider's Wahoo. This cannot be undone.",
    );
    await userEvent.click(within(dialog).getByRole("button", { name: "Delete plan" }));
    // A refused delete says why where the reader is looking: in the dialog, still open.
    expect(await within(dialog).findByRole("alert")).toHaveTextContent(
      "Could not delete plan: Plan changed since it was read",
    );
    await userEvent.click(within(dialog).getByRole("button", { name: "Delete plan" }));

    expect(remove).toHaveBeenCalledWith({ planId: 4, headers: { "If-Match": "2" } });
    expect(await screen.findByDisplayValue("")).toBeInTheDocument();
  });

  it("publishes a new plan on its first save when Published is chosen", async () => {
    create.mockResolvedValue({ data: { id: 7, version: 1 } });
    replace.mockResolvedValue({ data: { id: 7, version: 2, published: true } });
    renderPage();

    fireEvent.change(screen.getByLabelText("Plan name"), { target: { value: "Fresh" } });
    fireEvent.click(screen.getByRole("button", { name: "Plan route map" }));
    fireEvent.click(screen.getByRole("button", { name: "Plan route map" }));
    fireEvent.click(screen.getByRole("button", { name: "Published" }));
    fireEvent.click(screen.getByRole("button", { name: "Save" }));
    await act(async () => {});

    expect(replace).toHaveBeenCalledWith({
      planId: 7,
      data: expect.objectContaining({ name: "Fresh", published: true }),
      headers: { "If-Match": "1" },
    });
  });

  it("says so on the saved draft when publishing a new plan fails", async () => {
    create.mockResolvedValue({ data: { id: 7, version: 1 } });
    replace.mockRejectedValue(new Error("Routing unavailable"));
    openedPlan.value = {
      data: {
        data: {
          id: 7,
          name: "Fresh",
          profile: "trekking",
          cues: false,
          published: false,
          version: 1,
          waypoints: [
            { longitude: 8, latitude: 49 },
            { longitude: 8, latitude: 49 },
          ],
          geometry: { type: "LineString", coordinates: [] },
          distanceMetres: 0,
          ascentMetres: 0,
          createdAt: "2026-09-15T09:00:00Z",
          updatedAt: "2026-09-15T09:00:00Z",
        },
      },
    };
    renderPage();

    fireEvent.change(screen.getByLabelText("Plan name"), { target: { value: "Fresh" } });
    fireEvent.click(screen.getByRole("button", { name: "Plan route map" }));
    fireEvent.click(screen.getByRole("button", { name: "Plan route map" }));
    fireEvent.click(screen.getByRole("button", { name: "Published" }));
    fireEvent.click(screen.getByRole("button", { name: "Save" }));
    await act(async () => {});

    expect(screen.getByDisplayValue("Fresh")).toBeInTheDocument();
    expect(screen.getByRole("alert")).toHaveTextContent(
      "Saved as a draft, but publishing failed: Routing unavailable",
    );
  });

  it("draws a pressed waypoint and its legs straight before the click lands, and drops it on a pan", () => {
    renderPage();
    const map = screen.getByRole("button", { name: "Plan route map" });
    const drawn = () =>
      JSON.parse(screen.getByTestId("plan-transition").dataset.geometry ?? "{}").features.map(
        (leg: { properties: { provisional: boolean } }) => leg.properties.provisional,
      );
    fireEvent.click(map);
    mapPoint.value = { longitude: 8.1, latitude: 49.1 };
    fireEvent.mouseDown(map);

    expect(drawn()).toEqual([true]);
    fireEvent.click(screen.getByRole("button", { name: "Pan map" }));
    expect(drawn()).toEqual([]);

    fireEvent.mouseDown(map);
    fireEvent.click(map);
    // Placed but not yet routed: still straight, and its row claims no distance.
    expect(drawn()).toEqual([true]);
    expect(waypointRows()[1]).not.toMatch(/km/);
  });
});

describe("avoidRadiusTo", () => {
  it("measures from the centre to the dragged edge and keeps to the service's bounds", async () => {
    const { avoidRadiusTo } = await import("./PlanPage");
    const centre = { longitude: 8, latitude: 0 };

    expect(avoidRadiusTo(centre, { longitude: 8 + 1000 / 111_320, latitude: 0 })).toBe(1000);
    expect(avoidRadiusTo(centre, centre)).toBe(10);
    expect(avoidRadiusTo(centre, { longitude: 9, latitude: 0 })).toBe(5000);
  });
});
