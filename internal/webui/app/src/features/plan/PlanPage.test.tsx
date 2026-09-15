import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { act, fireEvent, render, screen } from "@testing-library/react";
import { MemoryRouter, Route, Routes } from "react-router";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

const preview = vi.hoisted(() => vi.fn());
const create = vi.hoisted(() => vi.fn());
const openedPlan = vi.hoisted(() => ({ value: {} }));

vi.mock("../../api/generated", async (importOriginal) => ({
  ...(await importOriginal<typeof import("../../api/generated")>()),
  getGetPlanQueryKey: (id: number) => ["plan", id],
  useCreatePlan: () => ({ isPending: false, mutateAsync: create }),
  useGetPlan: () => openedPlan.value,
  useListPlans: () => ({
    data: { data: { plans: [{ id: 4, name: "Draft loop", published: false }] } },
  }),
  usePreviewPlanRoute: () => ({ mutate: preview }),
  useReplacePlan: () => ({ isPending: false, mutateAsync: vi.fn() }),
}));
vi.mock("../../api/queries", () => ({
  webUIConfigQuery: () => ({ queryKey: ["config"], queryFn: vi.fn() }),
}));
vi.mock("../../components/Layout", () => ({
  PageShell: ({ children }: { children: React.ReactNode }) => <main>{children}</main>,
}));
vi.mock("../../components/map/MapWidget", () => ({
  MapWidget: ({
    children,
    onClick,
  }: {
    children: React.ReactNode;
    onClick?: (event: { lngLat: { lng: number; lat: number } }) => void;
  }) => (
    <button
      type="button"
      aria-label="Plan route map"
      onClick={() => onClick?.({ lngLat: { lng: 8, lat: 49 } })}
    >
      {children}
    </button>
  ),
}));
vi.mock("react-map-gl/maplibre", () => ({
  Marker: ({ children }: { children: React.ReactNode }) => children,
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
  openedPlan.value = {};
});

afterEach(() => vi.useRealTimers());

describe("PlanPage", () => {
  it("renders the planner with a mocked map and labels drafts", () => {
    renderPage();

    expect(screen.getByRole("button", { name: "Plan route map" })).toBeInTheDocument();
    expect(screen.getByText("Draft loop")).toBeInTheDocument();
    expect(screen.getByText("Draft")).toBeInTheDocument();
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
    renderPage("/plan/4");
    await act(async () => {});

    expect(screen.getByDisplayValue("Stored loop")).toBeInTheDocument();
    fireEvent.click(screen.getByRole("link", { name: "New" }));
    await act(async () => {});

    expect(screen.getByDisplayValue("")).toBeInTheDocument();
    expect(screen.queryByLabelText("Planned route summary")).toBeNull();
  });
});
