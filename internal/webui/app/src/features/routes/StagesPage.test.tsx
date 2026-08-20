import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { render, screen, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { MemoryRouter } from "react-router";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import type { Stage } from "../../api/types";
import { StagesPage } from "./StagesPage";

function stage(routeId: number, stageOrder: number, routeName: string, stageName: string): Stage {
  return {
    routeId,
    stageOrder,
    routeName,
    stageName,
    title: stageName ? `${routeName} — ${stageName}` : routeName,
    sourceRevision: "2026-08-17",
    contentHash: `hash-${routeId}-${stageOrder}`,
    distanceMetres: 10_000 * stageOrder + routeId,
    ascentMetres: 100 * stageOrder,
    maxGradientPercent: 8,
    pointCount: 100,
  };
}

const LIBRARY = [
  stage(1, 1, "Rhine Traverse", "Valley floor"),
  stage(1, 2, "Rhine Traverse", "Forest ramps"),
  stage(2, 1, "Kaiserstuhl Loop", ""),
];

function renderPage(stages: Stage[] = LIBRARY) {
  // The seeded listing is the whole fixture. The cards' own geometry requests
  // are left to fail: a card without geometry still renders its facts, and this
  // page is judged on which cards it shows, not on what they drew.
  const client = new QueryClient({
    defaultOptions: { queries: { retry: false, staleTime: Number.POSITIVE_INFINITY } },
  });
  client.setQueryData(["stages"], stages);

  return render(
    <QueryClientProvider client={client}>
      <MemoryRouter>
        <StagesPage />
      </MemoryRouter>
    </QueryClientProvider>,
  );
}

function titles(): string[] {
  return screen
    .getAllByRole("link")
    .map((link) => link.querySelector(".route-card__title")?.textContent ?? "")
    .filter(Boolean);
}

/** The stage names the table view is showing, read down its rows. */
function rows(): string[] {
  return screen
    .getAllByRole("row")
    .slice(1)
    .map((row) => within(row).getByRole("link").textContent ?? "");
}

beforeEach(() => {
  vi.stubGlobal(
    "fetch",
    vi.fn(async () => new Response("{}", { status: 404 })),
  );
  // The cards read the colour scheme to pick a basemap, and jsdom implements no
  // `matchMedia` for them to read it from.
  vi.stubGlobal(
    "matchMedia",
    vi.fn(() => ({ matches: false, addEventListener: () => {}, removeEventListener: () => {} })),
  );
});

afterEach(() => {
  vi.unstubAllGlobals();
});

describe("StagesPage", () => {
  it("lists the library by name by default", () => {
    renderPage();

    expect(titles()).toEqual([
      "Kaiserstuhl Loop",
      "Rhine Traverse — Forest ramps",
      "Rhine Traverse — Valley floor",
    ]);
  });

  it("narrows the grid to what a search matches", async () => {
    const user = userEvent.setup();
    renderPage();

    await user.type(screen.getByRole("searchbox", { name: "Search" }), "forest");

    expect(titles()).toEqual(["Rhine Traverse — Forest ramps"]);
    expect(screen.getByText("Showing 1 of 3 stages")).toBeInTheDocument();
  });

  it("reorders the grid without refetching the library", async () => {
    const user = userEvent.setup();
    renderPage();

    await user.selectOptions(screen.getByRole("combobox", { name: "Sort by" }), "distance");

    expect(titles()).toEqual([
      "Rhine Traverse — Forest ramps",
      "Kaiserstuhl Loop",
      "Rhine Traverse — Valley floor",
    ]);
    // Narrowing and ordering are questions about the listing already held, so
    // neither may become a query the service has to answer: the cards fetch
    // their own geometry and the map configuration, and nothing asks for a
    // filtered or ordered listing. The run history is the one request on this
    // page that does carry a query, and it asks about runs rather than routes.
    const calls = vi
      .mocked(fetch)
      .mock.calls.map(([input]) => String(input))
      .filter((url) => !url.startsWith("/v1/sync/runs"));
    expect(calls.every((url) => !url.includes("?"))).toBe(true);
  });

  it("keeps a split route's stage context when a search leaves one stage of it", async () => {
    const user = userEvent.setup();
    renderPage();

    await user.type(screen.getByRole("searchbox", { name: "Search" }), "forest");

    expect(screen.getByText("Stage 2 of 2")).toBeInTheDocument();
  });

  it("explains an empty grid a search caused, and offers the way back", async () => {
    const user = userEvent.setup();
    renderPage();

    await user.type(screen.getByRole("searchbox", { name: "Search" }), "summit");

    expect(screen.getByRole("status")).toHaveTextContent("No stages match “summit”.");
    expect(titles()).toEqual([]);

    await user.click(screen.getByRole("button", { name: "Clear search" }));

    expect(titles()).toHaveLength(3);
  });

  it("reads the library as rows when the table view is chosen", async () => {
    const user = userEvent.setup();
    renderPage();

    await user.click(screen.getByRole("radio", { name: "Table" }));

    // The same stages in the same order as the grid, and no cards left behind.
    expect(rows()).toEqual([
      "Kaiserstuhl Loop",
      "Rhine Traverse — Forest ramps",
      "Rhine Traverse — Valley floor",
    ]);
    expect(titles()).toEqual([]);
  });

  it("carries the search and the order across a change of presentation", async () => {
    const user = userEvent.setup();
    renderPage();

    await user.selectOptions(screen.getByRole("combobox", { name: "Sort by" }), "distance");
    await user.type(screen.getByRole("searchbox", { name: "Search" }), "rhine");
    await user.click(screen.getByRole("radio", { name: "Table" }));

    // The table is another way of reading the arranged list, not another list:
    // what the search left is still what is shown, in the order it was left in.
    expect(rows()).toEqual(["Rhine Traverse — Forest ramps", "Rhine Traverse — Valley floor"]);
    expect(screen.getByText("Showing 2 of 3 stages")).toBeInTheDocument();
  });

  it("points each row at the stage it names", async () => {
    const user = userEvent.setup();
    renderPage();

    await user.click(screen.getByRole("radio", { name: "Table" }));

    expect(screen.getByRole("link", { name: "Kaiserstuhl Loop" })).toHaveAttribute(
      "href",
      "/routes/2/1",
    );
  });

  it("explains a search that matched nothing whichever presentation is on", async () => {
    const user = userEvent.setup();
    renderPage();

    await user.click(screen.getByRole("radio", { name: "Table" }));
    await user.type(screen.getByRole("searchbox", { name: "Search" }), "summit");

    // One explanation serves both views rather than each carrying its own.
    expect(screen.getByRole("status")).toHaveTextContent("No stages match “summit”.");
    expect(screen.queryByRole("table")).not.toBeInTheDocument();

    await user.click(screen.getByRole("button", { name: "Clear search" }));

    expect(rows()).toHaveLength(3);
  });

  it("explains an empty library differently from an empty search", () => {
    renderPage([]);

    expect(screen.getByRole("status")).toHaveTextContent("No routes yet");
    expect(screen.queryByRole("searchbox")).not.toBeInTheDocument();
  });
});
