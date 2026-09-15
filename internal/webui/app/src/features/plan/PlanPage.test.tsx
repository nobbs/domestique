import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { act, fireEvent, render, screen } from "@testing-library/react";
import { MemoryRouter, Route, Routes } from "react-router";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

const preview = vi.hoisted(() => vi.fn());
const create = vi.hoisted(() => vi.fn());
const replace = vi.hoisted(() => vi.fn());
const openedPlan = vi.hoisted(() => ({ value: {} }));
const overlayInsets = vi.hoisted(() => ({ value: { top: 12, right: 13, bottom: 14, left: 15 } }));
const mapPoint = vi.hoisted(() => ({ value: { longitude: 8, latitude: 49 } }));

vi.mock("../../api/generated", async (importOriginal) => ({
  ...(await importOriginal<typeof import("../../api/generated")>()),
  getGetPlanQueryKey: (id: number) => ["plan", id],
  useCreatePlan: () => ({ isPending: false, mutateAsync: create }),
  useGetPlan: () => openedPlan.value,
  useListPlans: () => ({
    data: { data: { plans: [{ id: 4, name: "Draft loop", published: false }] } },
  }),
  usePreviewPlanRoute: () => ({ mutate: preview }),
  useReplacePlan: () => ({ isPending: false, mutateAsync: replace }),
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
  }: {
    map: React.ReactNode;
    children: React.ReactNode;
    dock: React.ReactNode;
  }) => (
    <main>
      {map}
      <div className="shell__overlay">
        {children}
        {dock}
      </div>
    </main>
  ),
}));
vi.mock("../../lib/overlayInsets", () => ({
  useOverlayInsets: () => overlayInsets.value,
}));
vi.mock("../../components/map/MapWidget", () => ({
  MapWidget: ({
    children,
    furniture,
    onClick,
  }: {
    children: React.ReactNode;
    furniture?: React.ReactNode;
    onClick?: (event: {
      lngLat: { lng: number; lat: number };
      originalEvent: { altKey: boolean };
    }) => void;
  }) => (
    <>
      <button
        type="button"
        aria-label="Plan route map"
        onClick={(event) =>
          onClick?.({
            lngLat: { lng: mapPoint.value.longitude, lat: mapPoint.value.latitude },
            originalEvent: { altKey: event.altKey },
          })
        }
      >
        {children}
      </button>
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
  MapViewport: ({ bounds, insets }: { bounds: unknown; insets: unknown }) => (
    <output data-testid="plan-viewport" data-insets={JSON.stringify(insets)}>
      {JSON.stringify(bounds)}
    </output>
  ),
}));
vi.mock("react-map-gl/maplibre", () => ({
  Marker: ({ children }: { children: React.ReactNode }) => children,
  ScaleControl: ({ position, unit }: { position: string; unit: string }) => (
    <output data-testid="plan-scale" data-position={position} data-unit={unit} />
  ),
}));
vi.mock("../routes/RouteOverlay", () => ({
  RouteOverlay: ({ coordinates }: { coordinates: unknown[] }) => (
    <output data-testid="route-line">{coordinates.length}</output>
  ),
}));
vi.mock("../routes/ElevationProfile", () => ({
  ElevationProfile: () => <div>elevation profile</div>,
}));

const { PlanPage } = await import("./PlanPage");

function renderPage(path = "/plan") {
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
  openedPlan.value = {};
  mapPoint.value = { longitude: 8, latitude: 49 };
});

afterEach(() => vi.useRealTimers());

describe("PlanPage", () => {
  it("renders the planner with a mocked map and labels drafts", () => {
    renderPage();

    expect(screen.getByRole("button", { name: "Plan route map" })).toBeInTheDocument();
    expect(screen.getByText("Draft loop")).toBeInTheDocument();
    expect(screen.getByText("Draft")).toBeInTheDocument();
    expect(screen.getByTestId("plan-map-controls")).toBeInTheDocument();
    expect(screen.getByTestId("plan-basemap-picker")).toBeInTheDocument();
    expect(screen.getByTestId("plan-scale")).toHaveAttribute("data-position", "bottom-left");
    expect(screen.getByTestId("plan-scale")).toHaveAttribute("data-unit", "metric");
    expect(screen.getByTestId("plan-viewport")).toHaveAttribute(
      "data-insets",
      JSON.stringify(overlayInsets.value),
    );
  });

  it("folds and reopens its independent planner and elevation overlays", () => {
    renderPage();

    fireEvent.click(screen.getByRole("button", { name: "Hide planner controls" }));
    expect(screen.getByRole("button", { name: "Show planner controls" })).toBeInTheDocument();
    expect(screen.queryByLabelText("Name")).toBeNull();
    fireEvent.click(screen.getByRole("button", { name: "Show planner controls" }));
    expect(screen.getByLabelText("Name")).toBeInTheDocument();

    fireEvent.click(screen.getByRole("button", { name: "Hide elevation" }));
    expect(screen.getByRole("button", { name: "Show elevation" })).toBeInTheDocument();
    fireEvent.click(screen.getByRole("button", { name: "Show elevation" }));
    expect(screen.getByRole("button", { name: "Hide elevation" })).toBeInTheDocument();
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

    expect(preview).toHaveBeenCalledOnce();
    expect(screen.getByTestId("route-line")).toHaveTextContent("2");
    expect(screen.getByLabelText("Planned route summary")).toHaveTextContent("10.0 km · 100 m");
    failed = true;
    fireEvent.click(screen.getByRole("button", { name: "Plan route map" }));
    act(() => vi.advanceTimersByTime(300));

    expect(screen.getByText("Routing unavailable")).toBeInTheDocument();
    expect(screen.getByTestId("route-line")).toHaveTextContent("2");
  });

  it("shows a save failure instead of leaving a rejected action behind", async () => {
    create.mockRejectedValue(new Error("Save unavailable"));
    renderPage();

    fireEvent.change(screen.getByLabelText("Name"), { target: { value: "Draft" } });
    fireEvent.click(screen.getByRole("button", { name: "Plan route map" }));
    fireEvent.click(screen.getByRole("button", { name: "Plan route map" }));
    fireEvent.click(screen.getByRole("button", { name: "Save draft" }));

    await act(async () => {});
    expect(screen.getByText("Save unavailable")).toBeInTheDocument();
  });

  it("starts a new draft after leaving an opened plan", async () => {
    openedPlan.value = {
      data: {
        data: {
          id: 4,
          name: "Stored loop",
          profile: "trekking",
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
    preview.mockImplementation(
      (_variables: unknown, callbacks: { onError: (error: Error) => void }) =>
        callbacks.onError(new Error("Stored route could not refresh")),
    );
    renderPage("/plan/4");
    await act(async () => {});
    act(() => vi.advanceTimersByTime(300));

    expect(screen.getByDisplayValue("Stored loop")).toBeInTheDocument();
    expect(screen.getByLabelText("Planned route summary")).toHaveTextContent("10.0 km · 100 m");
    expect(screen.getByText("Stored route could not refresh")).toBeInTheDocument();
    expect(screen.getByTestId("route-line")).toHaveTextContent("2");
    expect(screen.getByTestId("plan-viewport")).toHaveTextContent("[8,49,8.1,49.1]");
    fireEvent.click(screen.getByRole("link", { name: "New" }));
    await act(async () => {});

    expect(screen.getByDisplayValue("")).toBeInTheDocument();
    expect(screen.queryByLabelText("Planned route summary")).toBeNull();
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

    fireEvent.click(screen.getByRole("button", { name: "Save changes" }));
    await act(async () => {});
    fireEvent.click(screen.getByRole("button", { name: "Publish — syncs on next run" }));
    await act(async () => {});

    expect(replace.mock.calls[0]?.[0]).toMatchObject({ headers: { "If-Match": "2" } });
    expect(replace.mock.calls[1]?.[0]).toMatchObject({ headers: { "If-Match": "3" } });
  });

  it("commits a negative waypoint coordinate after its text entry is complete", () => {
    renderPage();
    fireEvent.click(screen.getByRole("button", { name: "Plan route map" }));
    fireEvent.click(screen.getByRole("button", { name: "Plan route map" }));
    const latitude = screen.getByRole("textbox", { name: "Waypoint 1 latitude" });
    fireEvent.change(latitude, { target: { value: "-" } });
    expect(latitude).toHaveValue("-");
    fireEvent.change(latitude, {
      target: { value: "-49.5" },
    });
    expect(latitude).toHaveValue("-49.5");
    fireEvent.blur(latitude);
    act(() => vi.advanceTimersByTime(300));

    expect(preview).toHaveBeenCalledWith(
      expect.objectContaining({
        data: expect.objectContaining({
          waypoints: [
            expect.objectContaining({ longitude: 8, latitude: -49.5 }),
            expect.objectContaining({ longitude: 8, latitude: 49 }),
          ],
        }),
      }),
      expect.anything(),
    );
  });

  it("does not reroute after changing only the plan name", () => {
    renderPage();
    fireEvent.click(screen.getByRole("button", { name: "Plan route map" }));
    fireEvent.click(screen.getByRole("button", { name: "Plan route map" }));
    act(() => vi.advanceTimersByTime(300));
    expect(preview).toHaveBeenCalledOnce();

    fireEvent.change(screen.getByLabelText("Name"), { target: { value: "No detour" } });
    act(() => vi.advanceTimersByTime(300));

    expect(preview).toHaveBeenCalledOnce();
  });

  it("inserts map clicks into the nearest leg and appends at the final endpoint, with Alt, or before two waypoints", () => {
    renderPage();
    const map = screen.getByRole("button", { name: "Plan route map" });

    mapPoint.value = { longitude: 8, latitude: 49 };
    fireEvent.click(map);
    mapPoint.value = { longitude: 8.2, latitude: 49 };
    fireEvent.click(map);
    expect(screen.getByLabelText("Waypoint 2 longitude")).toHaveValue("8.2");

    mapPoint.value = { longitude: 8.1, latitude: 49.01 };
    fireEvent.click(map);
    expect(screen.getByLabelText("Waypoint 2 longitude")).toHaveValue("8.1");
    expect(screen.getByLabelText("Waypoint 3 longitude")).toHaveValue("8.2");

    mapPoint.value = { longitude: 9, latitude: 50 };
    fireEvent.click(map);
    expect(screen.getByLabelText("Waypoint 4 longitude")).toHaveValue("9");

    mapPoint.value = { longitude: 10, latitude: 50 };
    fireEvent.click(map, { altKey: true });
    expect(screen.getByLabelText("Waypoint 5 longitude")).toHaveValue("10");
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

  it("numbers draggable waypoint handles and reroutes after reordering them", async () => {
    openedPlan.value = {
      data: {
        data: {
          id: 4,
          name: "Stored loop",
          profile: "trekking",
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
    preview.mockClear();

    const first = screen.getByRole("button", { name: "Drag waypoint 1 to reorder" });
    const third = screen.getByRole("button", { name: "Drag waypoint 3 to reorder" });
    expect(first).toHaveTextContent("1");
    expect(third).toHaveTextContent("3");
    expect(screen.getByRole("button", { name: "Move waypoint 2 up" })).toBeInTheDocument();

    fireEvent.dragStart(first);
    fireEvent.dragOver(third.closest("li") as HTMLElement);
    fireEvent.drop(third.closest("li") as HTMLElement);
    act(() => vi.advanceTimersByTime(300));

    expect(screen.getByLabelText("Waypoint 1 longitude")).toHaveValue("8.1");
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
});
