import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { BrowserRouter, MemoryRouter, useLocation } from "react-router";
import { afterEach, describe, expect, it, vi } from "vitest";
import { webUIConfigQuery } from "../../api/queries";
import type { Route, WebUIConfig } from "../../api/types";
import { RoutePanel, type RoutePanelProps } from "./RoutePanel";

function route(overrides: Partial<Route> = {}): Route {
  return {
    provider: "veloplanner",
    sourceRouteId: 12,
    stageOrder: 2,
    title: "Alpine loop — Descent",
    sourceRouteName: "Alpine loop",
    routeName: "Descent",
    sourceRevision: "2026-08-17",
    contentHash: "hash",
    distanceMetres: 42_500,
    ascentMetres: 620,
    descentMetres: 540,
    maxGradientPercent: 11.4,
    pointCount: 1200,
    ...overrides,
  };
}

function seededClient(admin: boolean, planning = false): QueryClient {
  const client = new QueryClient({
    defaultOptions: { queries: { retry: false, staleTime: Number.POSITIVE_INFINITY } },
  });
  const config: WebUIConfig = {
    basemaps: [],
    sourceBaseUrls: {},
    timezone: "Europe/Berlin",
    identity: { display: "rider@example.test", admin },
    planning,
  };
  client.setQueryData(webUIConfigQuery().queryKey, config);

  return client;
}

function LocationState() {
  const { state } = useLocation();

  return <output data-testid="route-panel-location-state">{JSON.stringify(state)}</output>;
}

function renderPanel(
  overrides: Partial<RoutePanelProps> = {},
  admin = false,
  planning = false,
  router: boolean | "browser" = false,
) {
  const client = seededClient(admin, planning);
  const props: RoutePanelProps = {
    route: route(),
    highestMetres: null,
    lowestMetres: null,
    gradients: { averageClimbing: 0, steepestClimbing: 0, steepestDescent: 0 },
    surface: null,
    surfaceAbsence: "Surface not classified yet.",
    bands: [],
    highlight: null,
    onHighlightChange: () => {},
    onHighlightClear: () => {},
    collapsed: false,
    onCollapsedChange: () => {},
    onClose: () => {},
    sourceBaseUrls: {},
    ...overrides,
  };

  return render(
    <QueryClientProvider client={client}>
      {router === "browser" ? (
        <BrowserRouter>
          <RoutePanel {...props} />
          <LocationState />
        </BrowserRouter>
      ) : router ? (
        <MemoryRouter>
          <RoutePanel {...props} />
          <LocationState />
        </MemoryRouter>
      ) : (
        <RoutePanel {...props} />
      )}
    </QueryClientProvider>,
  );
}

afterEach(() => window.history.replaceState(null, "", "/"));

describe("RoutePanel", () => {
  it("offers edit only for a local route when planning is available", async () => {
    renderPanel({ route: route({ provider: "local", sourceRouteId: 44 }) }, true, true, true);
    await userEvent.click(screen.getByRole("button", { name: "More about this route" }));

    expect(await screen.findByRole("menuitem", { name: "Edit" })).toHaveAttribute(
      "href",
      "/plan/44",
    );
    expect(screen.queryByRole("menuitem", { name: "Copy and edit" })).toBeNull();
  });

  it("offers copy and edit only to an admin with planning and usable geometry", async () => {
    renderPanel(
      {
        copySeed: {
          name: "Alpine loop — Descent",
          profile: "trekking",
          waypoints: [
            { longitude: 8, latitude: 49 },
            { longitude: 8.1, latitude: 49.1 },
          ],
        },
      },
      true,
      true,
      "browser",
    );
    await userEvent.click(screen.getByRole("button", { name: "More about this route" }));

    const action = await screen.findByRole("menuitem", { name: "Copy and edit" });
    await userEvent.click(action);
    expect(window.location.pathname).toBe("/plan");
    expect(window.history.state.usr).toEqual({
      name: "Alpine loop — Descent",
      profile: "trekking",
      waypoints: [
        { longitude: 8, latitude: 49 },
        { longitude: 8.1, latitude: 49.1 },
      ],
    });
    expect(screen.getByTestId("route-panel-location-state")).toHaveTextContent(
      JSON.stringify({
        name: "Alpine loop — Descent",
        profile: "trekking",
        waypoints: [
          { longitude: 8, latitude: 49 },
          { longitude: 8.1, latitude: 49.1 },
        ],
      }),
    );
  });

  it.each([
    ["a rider", false, true],
    ["an admin without planning", true, false],
  ])("withholds copy and edit from %s", async (_reader, admin, planning) => {
    renderPanel(
      {
        copySeed: {
          name: "Alpine loop — Descent",
          profile: "trekking",
          waypoints: [
            { longitude: 8, latitude: 49 },
            { longitude: 8.1, latitude: 49.1 },
          ],
        },
      },
      admin,
      planning,
      true,
    );
    await userEvent.click(screen.getByRole("button", { name: "More about this route" }));

    expect(screen.queryByRole("menuitem", { name: "Copy and edit" })).toBeNull();
  });

  it("withholds copy and edit until geometry supplies two valid positions", async () => {
    renderPanel({}, true, true, true);
    await userEvent.click(screen.getByRole("button", { name: "More about this route" }));

    expect(screen.queryByRole("menuitem", { name: "Copy and edit" })).toBeNull();
  });

  it("rests as a pill with the headline figures, not the full grid", () => {
    renderPanel({ collapsed: true, route: route({ movingSeconds: 6420 }) });

    expect(screen.getByText("42.5 km · 620 m")).toBeInTheDocument();
    expect(screen.getByText("Moving time").parentElement).toHaveTextContent("1 h 45 min");
    expect(screen.getByText("Rolling")).toBeInTheDocument();
    expect(screen.queryByText("620 m of climbing")).toBeNull();
    expect(screen.queryByText("Mixed surface")).toBeNull();
  });

  it("clears the highlight on collapse without touching the zoom", async () => {
    const onHighlightClear = vi.fn();
    const onHighlightChange = vi.fn();
    renderPanel({ onHighlightClear, onHighlightChange });

    await userEvent.click(screen.getByRole("button", { expanded: true }));

    expect(onHighlightClear).toHaveBeenCalledOnce();
    expect(onHighlightChange).not.toHaveBeenCalled();
  });

  it("shows nothing for a route nothing has predicted", () => {
    renderPanel({ route: route() });

    expect(screen.getByText("Moving time").parentElement).toHaveTextContent("Moving time —");
    expect(screen.getByText("no moving time predicted")).toBeInTheDocument();
  });

  it("reads the climbing as a verdict beside the ascent", () => {
    renderPanel({ route: route({ distanceMetres: 42_500, ascentMetres: 620 }) });

    expect(screen.getByText("620 m of climbing").nextElementSibling).toHaveTextContent("Rolling");
  });

  it("reads the surface as a verdict, naming what the route is mostly made of", () => {
    renderPanel({
      surface: {
        bands: [],
        totalMetres: 10_000,
        shares: [
          { kind: "asphalt", metres: 6_000, share: 0.6 },
          { kind: "gravel", metres: 4_000, share: 0.4 },
        ],
      },
    });

    expect(screen.getByText("Mixed surface").nextElementSibling).toHaveTextContent("40% unsealed");
    expect(screen.getByText("asphalt 6.0 km · gravel 4.0 km")).toBeInTheDocument();
    expect(screen.getByText("unsealed 4.0 km")).toBeInTheDocument();
  });

  it("says why there is no surface verdict", () => {
    renderPanel({ surface: null, surfaceAbsence: "Surface not classified yet." });

    expect(screen.getByText("Surface")).toBeInTheDocument();
    expect(screen.getByText("Surface not classified yet.")).toBeInTheDocument();
  });

  it("shows ascent and descent together in one climbing entry", () => {
    renderPanel({ route: route({ ascentMetres: 620, descentMetres: 540 }) });

    expect(screen.getByText("620 m of climbing")).toBeInTheDocument();
    expect(screen.getByText("540 m down", { exact: false })).toBeInTheDocument();
  });

  it("shows the steepest climb and descent together as one Max grade figure", () => {
    renderPanel({
      gradients: { averageClimbing: 4.8, steepestClimbing: 11, steepestDescent: 9.2 },
    });

    expect(screen.getByText("11%")).toBeInTheDocument();
    expect(screen.getByText("9.2%")).toBeInTheDocument();
  });

  it("shows the predicted moving time and its qualifier", () => {
    renderPanel({
      route: route({
        movingSeconds: 6420,
        validation: { biasPercent: -1.2, maePercent: 6.8, p90Percent: 14.1, evaluatedRides: 42 },
      }),
    });

    expect(screen.getByText("1 h 45 min")).toBeInTheDocument();
    expect(screen.getByText("moving time ±7% typical")).toBeInTheDocument();
  });

  it("omits the qualifier when the loaded profile carries no measured result", () => {
    renderPanel({ route: route({ movingSeconds: 6420 }) });

    expect(screen.getByText("1 h 45 min")).toBeInTheDocument();
    expect(screen.queryByText("±", { exact: false })).toBeNull();
  });

  /*
   * The acceptance criterion this exists to prove: selecting a stretch of the
   * profile swaps the whole-route figure for the stretch's own, and clearing
   * the selection — an undefined override — restores the whole-route figure.
   */
  it("shows the selection's moving time in place of the whole-route figure", () => {
    renderPanel({ route: route({ movingSeconds: 6420 }), movingSecondsOverride: 300 });

    expect(screen.getByText("5 min")).toBeInTheDocument();
    expect(screen.queryByText("1 h 45 min")).toBeNull();
  });

  it("restores the whole-route figure once the override is cleared", () => {
    const { rerender } = renderPanel({
      route: route({ movingSeconds: 6420 }),
      movingSecondsOverride: 300,
    });
    expect(screen.getByText("5 min")).toBeInTheDocument();

    rerender(
      <QueryClientProvider client={seededClient(false)}>
        <RoutePanel
          route={route({ movingSeconds: 6420 })}
          highestMetres={null}
          lowestMetres={null}
          gradients={{ averageClimbing: 0, steepestClimbing: 0, steepestDescent: 0 }}
          surface={null}
          surfaceAbsence="Surface not classified yet."
          bands={[]}
          highlight={null}
          onHighlightChange={() => {}}
          onHighlightClear={() => {}}
          collapsed={false}
          onCollapsedChange={() => {}}
          onClose={() => {}}
          sourceBaseUrls={{}}
        />
      </QueryClientProvider>,
    );

    expect(screen.getByText("1 h 45 min")).toBeInTheDocument();
  });

  // Reprocessing spends the shared upstream budget on a route every rider
  // sees, so it is offered only once the caller is known to be an admin.
  it("offers reprocess to an admin", async () => {
    renderPanel({}, true);

    await userEvent.click(screen.getByRole("button", { name: "More about this route" }));

    expect(await screen.findByText("Reprocess")).toBeInTheDocument();
  });

  it("hides reprocess from a non-admin", async () => {
    renderPanel({}, false);

    await userEvent.click(screen.getByRole("button", { name: "More about this route" }));

    expect(screen.queryByText("Reprocess")).not.toBeInTheDocument();
  });
});
