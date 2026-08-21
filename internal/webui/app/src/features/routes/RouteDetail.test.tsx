/**
 * The route page before it has a route to draw.
 *
 * What is tested here is everything the page says instead of a map: a route
 * address that is not one, geometry the library does not hold, and a service
 * that could not answer. Each of them is a page an operator can arrive at from
 * a link, and each has to say which of the three it is — "something went wrong"
 * repeated four times would tell them nothing.
 *
 * The drawn page itself is exercised in the browser suite, where there is a
 * canvas to draw on.
 */

import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { render, screen } from "@testing-library/react";
import { MemoryRouter, Route, Routes } from "react-router";
import { afterEach, describe, expect, it, vi } from "vitest";
import { RouteDetail } from "./RouteDetail";

const GEOMETRY = "/v1/routes/4102/stages/1/geometry";
const CONFIG = "/v1/webui/config";

/** Enough of a route for the page to get past the geometry it asks for first. */
const GEOMETRY_BODY = {
  bbox: [8, 49, 8.1, 49.1],
  geometry: {
    coordinates: [
      [8, 49, 100],
      [8.1, 49.1, 180],
    ],
  },
  properties: {
    route_id: 4102,
    stage: 1,
    title: "Kaiserstuhl Loop",
    route_name: "Kaiserstuhl Loop",
    stage_name: "",
    distance_metres: 42_000,
    point_count: 2,
  },
};

/** One answer per endpoint, by status; anything unnamed is answered normally. */
function renderRoute(path: string, answers: Record<string, number> = {}) {
  vi.stubGlobal(
    "fetch",
    vi.fn(async (input: RequestInfo | URL) => {
      const url = String(input);
      const status = Object.entries(answers).find(([prefix]) => url.startsWith(prefix))?.[1];
      if (status !== undefined && status !== 200) {
        return new Response(JSON.stringify({ error: { code: "no", message: "no" } }), { status });
      }
      if (url.startsWith(CONFIG)) {
        return new Response(
          JSON.stringify({ tile_style_url: "https://tiles.example/style.json" }),
          {
            status: 200,
          },
        );
      }
      if (url.startsWith(GEOMETRY)) {
        return new Response(JSON.stringify(GEOMETRY_BODY), { status: 200 });
      }

      return new Response("{}", { status: 500 });
    }),
  );
  const client = new QueryClient({ defaultOptions: { queries: { retry: false } } });

  return render(
    <QueryClientProvider client={client}>
      <MemoryRouter initialEntries={[path]}>
        <Routes>
          <Route path="/routes/:routeId/:stage" element={<RouteDetail />} />
        </Routes>
      </MemoryRouter>
    </QueryClientProvider>,
  );
}

afterEach(() => {
  vi.unstubAllGlobals();
});

describe("RouteDetail", () => {
  /*
   * A route's address is two positive integers. Anything else is a link that was
   * mistyped or truncated, and the page says so rather than asking the service
   * about a route that cannot exist.
   */
  it("refuses an address that is not a route's", () => {
    renderRoute("/routes/none/1");

    expect(screen.getByText("That is not a valid route address.")).toBeInTheDocument();
    expect(fetch).not.toHaveBeenCalledWith(expect.stringContaining("/geometry"), expect.anything());
  });

  it("refuses a stage order of zero, which no route has", () => {
    renderRoute("/routes/4102/0");

    expect(screen.getByText("That is not a valid route address.")).toBeInTheDocument();
  });

  // A route in the listing whose geometry has not been stored yet is an ordinary
  // state of a library mid-read, not a fault: the page says what is missing and
  // what will fill it.
  it("says geometry is missing rather than failing when the library has none", async () => {
    renderRoute("/routes/4102/1", { [GEOMETRY]: 404 });

    expect(await screen.findByText("No geometry for this route yet.")).toBeInTheDocument();
  });

  it("reports a service that could not answer for the geometry", async () => {
    renderRoute("/routes/4102/1", { [GEOMETRY]: 503 });

    expect(await screen.findByText(/the route geometry/)).toBeInTheDocument();
  });

  // The map configuration carries the basemap, so a page that cannot read it has
  // no ground to draw the route on. That is its own failure and says so.
  it("reports a map configuration that could not be read", async () => {
    renderRoute("/routes/4102/1", { [CONFIG]: 503 });

    expect(await screen.findByText(/the map configuration/)).toBeInTheDocument();
  });

  it("says it is loading while the two requests are in flight", () => {
    renderRoute("/routes/4102/1");

    expect(screen.getByText(/the route/)).toBeInTheDocument();
  });
});
