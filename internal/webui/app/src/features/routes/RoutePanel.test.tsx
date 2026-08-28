import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { render, screen } from "@testing-library/react";
import { describe, expect, it } from "vitest";
import type { Route } from "../../api/types";
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
    maxGradientPercent: 11.4,
    pointCount: 1200,
    ...overrides,
  };
}

function renderPanel(overrides: Partial<RoutePanelProps> = {}) {
  const client = new QueryClient({ defaultOptions: { queries: { retry: false } } });
  const props: RoutePanelProps = {
    route: route(),
    profile: null,
    highestMetres: null,
    subtitle: "",
    surface: null,
    surfaceAbsence: "Surface not classified yet.",
    bands: [],
    highlight: null,
    onHighlightChange: () => {},
    climbs: [],
    onSelectClimb: () => {},
    libraryCount: 0,
    onClose: () => {},
    sourceBaseUrls: {},
    unitSystem: "metric",
    ...overrides,
  };

  return render(
    <QueryClientProvider client={client}>
      <RoutePanel {...props} />
    </QueryClientProvider>,
  );
}

describe("RoutePanel", () => {
  it("shows nothing for a route nothing has predicted", () => {
    renderPanel({ route: route() });

    expect(screen.getByText("Moving time").nextElementSibling).toHaveTextContent("—");
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
          profile={null}
          highestMetres={null}
          subtitle=""
          surface={null}
          surfaceAbsence="Surface not classified yet."
          bands={[]}
          highlight={null}
          onHighlightChange={() => {}}
          climbs={[]}
          onSelectClimb={() => {}}
          libraryCount={0}
          onClose={() => {}}
          sourceBaseUrls={{}}
          unitSystem="metric"
        />
      </QueryClientProvider>,
    );

    expect(screen.getByText("1 h 45 min")).toBeInTheDocument();
  });
});
