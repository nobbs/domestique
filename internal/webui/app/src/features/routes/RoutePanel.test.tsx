import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { describe, expect, it, vi } from "vitest";
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

function renderPanel(overrides: Partial<RoutePanelProps> = {}, admin?: boolean) {
  const client = new QueryClient({ defaultOptions: { queries: { retry: false } } });
  if (admin !== undefined) {
    const config: WebUIConfig = {
      basemaps: [],
      sourceBaseUrls: {},
      timezone: "Europe/Berlin",
      identity: { display: "rider@example.test", admin },
    };
    client.setQueryData(webUIConfigQuery().queryKey, config);
  }
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
    libraryCount: 0,
    onClose: () => {},
    sourceBaseUrls: {},
    ...overrides,
  };

  return render(
    <QueryClientProvider client={client}>
      <RoutePanel {...props} />
    </QueryClientProvider>,
  );
}

describe("RoutePanel", () => {
  it("rests as a pill with the headline figures, not the full grid", () => {
    renderPanel({ collapsed: true });

    expect(screen.getByText("42.5 km · 620 m")).toBeInTheDocument();
    expect(screen.queryByText("Elevation")).toBeNull();
    expect(screen.queryByText("Moving time")).toBeNull();
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

    expect(screen.getByText("Moving time").nextElementSibling).toHaveTextContent("—");
  });

  it("shows ascent and descent together as one Ascent figure", () => {
    renderPanel({ route: route({ ascentMetres: 620, descentMetres: 540 }) });

    const value = screen.getByText("Ascent").nextElementSibling;
    expect(value).toHaveTextContent("620 m");
    expect(value).toHaveTextContent("540 m");
  });

  it("shows the steepest climb and descent together as one Max grade figure", () => {
    renderPanel({
      gradients: { averageClimbing: 4.8, steepestClimbing: 11, steepestDescent: 9.2 },
    });

    const value = screen.getByText("Max grade").nextElementSibling;
    expect(value).toHaveTextContent("11%");
    expect(value).toHaveTextContent("9.2%");
  });

  it("shows the predicted moving time and its qualifier", () => {
    renderPanel({
      route: route({
        movingSeconds: 6420,
        validation: { biasPercent: -1.2, maePercent: 6.8, p90Percent: 14.1, evaluatedRides: 42 },
      }),
    });

    expect(screen.getByText("1 h 45 min")).toBeInTheDocument();
    expect(screen.getByText("±7% typical")).toBeInTheDocument();
  });

  it("omits the qualifier when the loaded profile carries no measured result", () => {
    renderPanel({ route: route({ movingSeconds: 6420 }) });

    expect(screen.getByText("1 h 45 min")).toBeInTheDocument();
    expect(screen.queryByText("±", { exact: false })).toBeNull();
  });

  it("shows the door-to-door window and the allowance behind it", () => {
    renderPanel({ route: route({ movingSeconds: 6420 }) });

    expect(screen.getByText("1 h 50 min to 2 h")).toBeInTheDocument();
    expect(
      screen.getByText("4.4 min stopped per moving hour · spread from", {
        exact: false,
      }),
    ).toBeInTheDocument();
    expect(screen.getByText("242 current-bike rides", { exact: false })).toBeInTheDocument();
    expect(screen.getByRole("slider", { name: /^Stopping allowance, / })).toBeInTheDocument();
  });

  /*
   * The acceptance criterion for the measured habit: a rider whose own rides
   * carry one is offered it, and accepting moves the window onto their median.
   */
  it("offers the rider's own stopping habit and moves the window onto it", async () => {
    // The allowance is remembered in storage, so a choice made here would be
    // the next test's starting point.
    localStorage.clear();
    const user = userEvent.setup();
    renderPanel({
      route: route({ movingSeconds: 3600 }),
      stopping: {
        medianSecondsPerHour: 600,
        lowerQuartileSecondsPerHour: 300,
        upperQuartileSecondsPerHour: 1200,
        rides: 37,
      },
    });

    expect(screen.getByText("your 37 rides", { exact: false })).toBeInTheDocument();
    const offer = screen.getByRole("button", { name: /Your rides stop 10.0 min per moving hour/ });

    await user.click(offer);

    expect(screen.getByText("1 h 5 min to 1 h 20 min")).toBeInTheDocument();
    expect(
      screen.getByText("10.0 min stopped per moving hour", { exact: false }),
    ).toBeInTheDocument();
  });

  // A habit past the slider's end lands on the end, so the offer is withdrawn
  // there rather than staying up for a figure the slider cannot reach.
  it("withdraws the offer at the slider's end for a habit that runs past it", async () => {
    localStorage.clear();
    const user = userEvent.setup();
    renderPanel({
      route: route({ movingSeconds: 3600 }),
      stopping: {
        medianSecondsPerHour: 1800,
        lowerQuartileSecondsPerHour: 900,
        upperQuartileSecondsPerHour: 2700,
        rides: 8,
      },
    });

    await user.click(screen.getByRole("button", { name: /Your rides stop 30.0 min/ }));

    expect(screen.queryByRole("button", { name: /use that/ })).toBeNull();
    expect(
      screen.getByText("15.0 min stopped per moving hour", { exact: false }),
    ).toBeInTheDocument();
  });

  // Nothing to accept once the allowance already sits on the measured median.
  it("withdraws the offer once the rider is on their own median", () => {
    localStorage.clear();
    renderPanel({
      route: route({ movingSeconds: 3600 }),
      stopping: {
        medianSecondsPerHour: 266,
        lowerQuartileSecondsPerHour: 100,
        upperQuartileSecondsPerHour: 500,
        rides: 12,
      },
    });

    expect(screen.queryByRole("button", { name: /use that/ })).toBeNull();
    expect(screen.getByText("your 12 rides", { exact: false })).toBeInTheDocument();
  });

  it("shows no arrival at all for a route nothing has predicted", () => {
    renderPanel({ route: route() });

    expect(screen.queryByText("Door to door")).toBeNull();
    expect(screen.queryByRole("slider")).toBeNull();
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

    const client = new QueryClient({ defaultOptions: { queries: { retry: false } } });
    rerender(
      <QueryClientProvider client={client}>
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
          libraryCount={0}
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
