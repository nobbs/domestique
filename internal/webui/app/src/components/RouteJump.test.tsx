/**
 * The ⌘K jump, wherever it is offered.
 *
 * What is tested here is the agreement it exists to keep: it is a way to any
 * route from any page but the ones with a search of their own, and picking a
 * route from it is a normal navigation — it carries the address the reader was
 * on along, the same way a catalogue row does.
 */

import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { MemoryRouter, Route, Routes, useLocation } from "react-router";
import { describe, expect, it } from "vitest";
import { routesQuery } from "../api/queries";
import type { Route as LibraryRoute } from "../api/types";
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

function show(at: string, state?: unknown) {
  const client = new QueryClient({
    defaultOptions: { queries: { retry: false, staleTime: Number.POSITIVE_INFINITY } },
  });
  client.setQueryData(routesQuery().queryKey, LIBRARY);
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
});
