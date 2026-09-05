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
