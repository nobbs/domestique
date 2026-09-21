/**
 * The ⌘K jump, wherever it is offered.
 *
 * What is tested here is the agreement it exists to keep: it is a way to any
 * route from any page but the ones with a search of their own, and picking a
 * route from it is a normal navigation — it carries the address the reader was
 * on along, the same way a catalogue row does.
 */

import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { render, screen, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { MemoryRouter, Route, Routes, useLocation } from "react-router";
import { describe, expect, it } from "vitest";
import { getListPlansQueryKey } from "../api/generated";
import { routesQuery, webUIConfigQuery } from "../api/queries";
import type { Route as LibraryRoute, WebUIConfig } from "../api/types";
import { RouteJump } from "./RouteJump";

function route(sourceRouteId: number, title: string): LibraryRoute {
  return {
    provider: "veloplanner",
    sourceRouteId,
    stageOrder: 1,
    sourceRouteName: title,
    routeName: "",
    title,
    sourceRevision: "2026-08-17",
    contentHash: `hash-${sourceRouteId}`,
    distanceMetres: 10_000 * sourceRouteId,
    ascentMetres: 100,
    descentMetres: 80,
    maxGradientPercent: 6,
    pointCount: 10,
  };
}

const RHINE = route(1, "Rhine Traverse");
const KAISERSTUHL = route(2, "Kaiserstuhl Loop");
const LIBRARY = [RHINE, KAISERSTUHL];

/** Reports the address and router state the page landed on, for a route reached by jumping. */
function Landed() {
  const location = useLocation();

  return (
    <span data-testid="landed">
      {`${location.pathname}${location.search}`}
      {location.state ? ` state=${JSON.stringify(location.state)}` : ""}
    </span>
  );
}

const PLANS = [
  {
    id: 7,
    name: "Saturday gravel",
    profile: "gravel" as const,
    published: false,
    version: 2,
    distanceMetres: 42_000,
    ascentMetres: 610,
    waypointCount: 6,
    updatedAt: "2026-09-20T09:00:00Z",
  },
  {
    id: 8,
    name: "Weekday loop",
    profile: "fastbike" as const,
    published: true,
    version: 1,
    distanceMetres: 18_000,
    ascentMetres: 120,
    waypointCount: 3,
    updatedAt: "2026-09-15T09:00:00Z",
  },
];

function config(planner: boolean): WebUIConfig {
  return {
    basemaps: [],
    sourceBaseUrls: {},
    timezone: "Europe/Berlin",
    planning: planner,
    identity: { display: "someone@example.test", admin: planner },
  };
}

function show(at: string, state?: unknown, { planner = false }: { planner?: boolean } = {}) {
  const client = new QueryClient({
    defaultOptions: { queries: { retry: false, staleTime: Number.POSITIVE_INFINITY } },
  });
  client.setQueryData(routesQuery().queryKey, LIBRARY);
  client.setQueryData(webUIConfigQuery().queryKey, config(planner));
  // A rider's jump asks for no plans: unseeded, the listing would reach the refusing fetch.
  if (planner) {
    client.setQueryData(getListPlansQueryKey(), { data: { plans: PLANS } });
  }
  // No geometry is seeded: opening the panel must ask for none, and the suite's
  // refusing fetch fails any test that does.

  const entry = { pathname: at, ...(state !== undefined ? { state } : {}) };

  return render(
    <QueryClientProvider client={client}>
      <MemoryRouter initialEntries={[entry]}>
        <Landed />
        <Routes>
          <Route path="*" element={<RouteJump />} />
        </Routes>
      </MemoryRouter>
    </QueryClientProvider>,
  );
}

describe("RouteJump", () => {
  it("renders nothing on the catalogue, which has its own search", () => {
    show("/catalogue");

    expect(screen.queryByRole("button", { name: "Jump to a route" })).toBeNull();
  });

  it("opens on ⌘K from a page with no search of its own", async () => {
    show("/activities");

    await userEvent.keyboard("{Meta>}k{/Meta}");

    expect(screen.getByRole("searchbox", { name: "Search the route library" })).toBeVisible();
  });

  it("does not bind ⌘K on the planner, which has its own search", async () => {
    show("/plan");

    await userEvent.keyboard("{Meta>}k{/Meta}");

    expect(screen.queryByRole("searchbox")).toBeNull();
    // The button itself still stands, as the one way in from that page.
    expect(screen.getByRole("button", { name: "Jump to a route" })).toBeInTheDocument();
  });

  it("narrows the list as the query is typed", async () => {
    show("/activities");
    await userEvent.click(screen.getByRole("button", { name: "Jump to a route" }));

    await userEvent.type(screen.getByRole("searchbox"), "kaiserstuhl");

    expect(screen.getByRole("option", { name: /Kaiserstuhl Loop/ })).toBeInTheDocument();
    expect(screen.queryByRole("option", { name: /Rhine Traverse/ })).toBeNull();
  });

  it("opens the route named by Enter, forwarding the address it was reached from", async () => {
    show("/activities", { catalogue: "?sort=ascent&dir=asc" });
    await userEvent.click(screen.getByRole("button", { name: "Jump to a route" }));
    await userEvent.type(screen.getByRole("searchbox"), "kaiserstuhl");
    await userEvent.keyboard("{Enter}");

    expect(screen.getByTestId("landed")).toHaveTextContent("/routes/veloplanner/2/1");
    expect(screen.getByTestId("landed")).toHaveTextContent(
      'state={"catalogue":"?sort=ascent&dir=asc"}',
    );
  });

  it("lists an admin's drafts first, marked as drafts, and opens them in the planner", async () => {
    show("/activities", undefined, { planner: true });
    await userEvent.click(screen.getByRole("button", { name: "Jump to a route" }));

    const options = screen.getAllByRole("option");
    expect(options[0]).toHaveTextContent("Saturday gravel");
    expect(within(options[0] as HTMLElement).getByText("Draft")).toBeInTheDocument();
    // A published plan is in the library, not among the drafts.
    expect(screen.queryByRole("option", { name: /Weekday loop/ })).toBeNull();

    await userEvent.keyboard("{Enter}");
    expect(screen.getByTestId("landed")).toHaveTextContent("/plan/7");
  });

  it("shows a rider no drafts", async () => {
    show("/activities");
    await userEvent.click(screen.getByRole("button", { name: "Jump to a route" }));

    expect(screen.queryByText("Draft")).toBeNull();
    expect(screen.getAllByRole("option")).toHaveLength(LIBRARY.length);
  });
});
