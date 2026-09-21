/**
 * One route's own page, without a canvas.
 *
 * The map needs WebGL, so it is stood in for by a fake that only renders
 * whatever overlay it was handed. What is tested here is the page's own
 * contract: the address names the route, the panel and dock agree with it, and
 * closing it returns to wherever the reader opened it from.
 */

import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { render, screen, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { MemoryRouter, Route, Routes, useLocation } from "react-router";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import {
  activitiesQuery,
  riderProfileQuery,
  routeClimbsQuery,
  routeGeometryQuery,
  routesQuery,
  statusQuery,
  webUIConfigQuery,
} from "../../api/queries";
import type { Activity, Route as LibraryRoute, Position, RouteGeometry } from "../../api/types";
import { IDLE_STATUS } from "../../test/status";

vi.mock("./LibraryMap", () => ({
  // The overlay needs a real map's cartography context, which this stand-in
  // has none of — its own content is not this file's concern.
  LibraryMap: () => <div data-testid="library-map" />,
}));

const { AtlasPage } = await import("./AtlasPage");

function route(overrides: Partial<LibraryRoute> = {}): LibraryRoute {
  return {
    provider: "veloplanner",
    sourceRouteId: 2,
    stageOrder: 1,
    sourceRouteName: "Kaiserstuhl Loop",
    routeName: "",
    title: "Kaiserstuhl Loop",
    sourceRevision: "2026-08-17",
    contentHash: "hash-2-1",
    distanceMetres: 20_000,
    ascentMetres: 200,
    descentMetres: 180,
    maxGradientPercent: 8,
    pointCount: 100,
    ...overrides,
  };
}

const KAISERSTUHL = route();
const LIBRARY = [KAISERSTUHL];

const GEOMETRY: RouteGeometry = {
  bbox: [8, 49, 8.1, 49.1],
  coordinates: [
    [8, 49, 100],
    [8.05, 49.05, 140],
    [8.1, 49.1, 180],
  ] as Position[],
};

/** Reports the address and router state the page landed on. */
function Landed() {
  const location = useLocation();

  return (
    <span data-testid="landed">
      {`${location.pathname}${location.search}`}
      {location.state ? ` state=${JSON.stringify(location.state)}` : ""}
    </span>
  );
}

function renderPage(
  options: {
    library?: LibraryRoute[];
    at?: string;
    /** Entries before the route itself, so closing has in-app history to go back to. */
    entries?: string[];
    /** "failed" leaves geometry unseeded, so the stubbed 404 fetch answers it. */
    geometry?: RouteGeometry | "failed";
    activities?: Activity[];
  } = {},
) {
  const library = options.library ?? LIBRARY;
  const client = new QueryClient({
    defaultOptions: { queries: { retry: false, staleTime: Number.POSITIVE_INFINITY } },
  });
  client.setQueryData(routesQuery().queryKey, library);
  client.setQueryData(activitiesQuery().queryKey, options.activities ?? []);
  client.setQueryData(riderProfileQuery().queryKey, {
    profile: {},
    suggestions: {},
    zwift: { emailSet: false, passwordSet: false },
  });
  client.setQueryData(webUIConfigQuery().queryKey, {
    basemaps: [
      { name: "Streets", styleUrl: "https://tiles.example/style.json", darkCartography: false },
    ],
    sourceBaseUrls: {},
    timezone: "Europe/Berlin",
    identity: { display: "rider@example.test", admin: false },
  });
  client.setQueryData(statusQuery().queryKey, IDLE_STATUS);
  for (const entry of library) {
    client.setQueryData(
      routeClimbsQuery(entry.provider, entry.sourceRouteId, entry.stageOrder).queryKey,
      {
        climbs: [],
      },
    );
    if (options.geometry !== "failed") {
      client.setQueryData(
        routeGeometryQuery(entry.provider, entry.sourceRouteId, entry.stageOrder).queryKey,
        options.geometry ?? GEOMETRY,
      );
    }
  }

  const entries = [...(options.entries ?? []), options.at ?? "/routes/veloplanner/2/1"];

  return render(
    <QueryClientProvider client={client}>
      <MemoryRouter initialEntries={entries} initialIndex={entries.length - 1}>
        <Landed />
        <Routes>
          <Route
            path="/routes/:provider/:sourceRouteId/:stageOrder"
            element={<AtlasPage themeChoice="system" />}
          />
          <Route path="/activities" element={<span>the activities page</span>} />
        </Routes>
      </MemoryRouter>
    </QueryClientProvider>,
  );
}

beforeEach(() => {
  vi.stubGlobal(
    "fetch",
    vi.fn(async () => new Response("{}", { status: 404 })),
  );
});

afterEach(() => {
  vi.unstubAllGlobals();
});

describe("AtlasPage", () => {
  it("names the page after the open route", () => {
    renderPage();

    const heading = screen.getByRole("heading", { level: 1, name: "Kaiserstuhl Loop" });
    expect(heading).toHaveClass("visually-hidden");
    expect(screen.getByRole("region", { name: "Kaiserstuhl Loop" })).toBeInTheDocument();
  });

  it("gives the open route's rides a stop on the dock", async () => {
    renderPage({
      activities: [
        {
          id: "8",
          startedAt: "2026-08-26T08:00:00Z",
          distanceMetres: 42_000,
          movingSeconds: 5_400,
          elapsedSeconds: 6_000,
          ascentMetres: 600,
          typeId: 40,
          locationId: 0,
          indoor: false,
          provider: "wahoo",
          routeMatch: {
            provider: "veloplanner",
            sourceRouteId: 2,
            stageOrder: 1,
            routeCoverage: 1,
            rideCoverage: 0.98,
            direction: "forward",
          },
        },
      ],
    });

    await userEvent.click(await screen.findByRole("tab", { name: /Rides/ }));
    const history = screen.getByRole("list", { name: "Ride history" });
    expect(screen.getByText("Ridden 1 time")).toBeInTheDocument();
    expect(within(history).getByRole("link")).toHaveAttribute("href", "/activities/8");
  });

  it("gives a route nobody has ridden no rides stop", () => {
    renderPage({ activities: [] });

    expect(screen.queryByRole("tab", { name: /Rides/ })).toBeNull();
    expect(screen.queryByRole("list", { name: "Ride history" })).toBeNull();
  });

  it("puts the detail dock away and leaves the route open", async () => {
    renderPage();
    await userEvent.click(await screen.findByRole("button", { name: "Hide the route detail" }));

    expect(screen.getByRole("button", { name: "Show the profile" })).toBeInTheDocument();
    expect(screen.getByRole("region", { name: "Kaiserstuhl Loop" })).toBeInTheDocument();
  });

  it("says so when the address names a route the library does not have", () => {
    renderPage({ at: "/routes/veloplanner/99/1" });

    expect(screen.getByText("No route at that address.")).toBeInTheDocument();
  });

  it("says so when the open route's geometry never arrives", async () => {
    renderPage({ geometry: "failed" });

    expect(await screen.findByText("Could not load that route's geometry.")).toBeInTheDocument();
    expect(screen.queryByRole("region", { name: "Kaiserstuhl Loop" })).toBeNull();
  });

  it("closes back to wherever it was opened from, in-app", async () => {
    renderPage({ entries: ["/activities"] });

    await userEvent.click(await screen.findByRole("button", { name: "Close the route" }));

    expect(screen.getByText("the activities page")).toBeInTheDocument();
    expect(screen.getByTestId("landed")).toHaveTextContent("/activities");
  });

  it("closes to the activities page when it was opened directly, with no history", async () => {
    renderPage();

    await userEvent.click(await screen.findByRole("button", { name: "Close the route" }));

    expect(screen.getByText("the activities page")).toBeInTheDocument();
    expect(screen.getByTestId("landed")).toHaveTextContent("/activities");
  });

  it("closes on Escape the same way the close button does", async () => {
    renderPage();
    await screen.findByRole("region", { name: "Kaiserstuhl Loop" });

    await userEvent.keyboard("{Escape}");

    expect(screen.getByText("the activities page")).toBeInTheDocument();
  });
});
