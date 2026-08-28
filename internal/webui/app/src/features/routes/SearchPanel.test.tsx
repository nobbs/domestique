import { render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { MemoryRouter } from "react-router";
import { describe, expect, it, vi } from "vitest";
import type { Position, Route } from "../../api/types";
import { routeKey } from "../../api/types";
import { EMPTY_FILTERS } from "../../lib/filters";
import { SearchPanel, type SearchPanelProps } from "./SearchPanel";

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

/** A climb, so the mix bar has more than one band to divide. */
const CLIMB: Position[] = Array.from(
  { length: 60 },
  (_, index): Position => [8, 49 + index * 0.0004, index < 30 ? 100 : 100 + (index - 30) * 12],
);

function renderPanel(overrides: Partial<SearchPanelProps> = {}) {
  const props: SearchPanelProps = {
    shown: [route()],
    total: 47,
    query: "",
    onQueryChange: () => {},
    filters: EMPTY_FILTERS,
    onFiltersChange: () => {},
    filtersExpanded: false,
    onFiltersExpandedChange: () => {},
    pickedKey: null,
    onSelect: () => {},
    onOpen: () => {},
    shapes: new Map([[routeKey(route()), { coordinates: CLIMB }]]),
    readAt: "19:38",
    changeOf: () => null,
    unitSystem: "metric",
    ...overrides,
  };

  return render(
    <MemoryRouter>
      <SearchPanel {...props} />
    </MemoryRouter>,
  );
}

describe("SearchPanel", () => {
  it("is two compact controls until search is opened", async () => {
    renderPanel();

    const search = screen.getByRole("button", { name: "Search the route library" });

    expect(search.closest("[data-compact-workspace]")).toBeInTheDocument();
    expect(screen.queryByRole("searchbox", { name: "Search the route library" })).toBeNull();
    expect(screen.queryByText("Search")).toBeNull();
    expect(screen.queryByText("Filters")).toBeNull();
    await userEvent.click(search);

    expect(screen.getByRole("searchbox", { name: "Search the route library" })).toHaveAttribute(
      "placeholder",
      "Search 47 routes",
    );
    expect(screen.getByRole("searchbox", { name: "Search the route library" })).toHaveFocus();
    expect(screen.getByRole("searchbox").closest("[data-compact-workspace]")).toBeNull();
    expect(screen.queryByRole("list")).toBeNull();
    expect(screen.queryByRole("button", { name: "Search the route library" })).toBeNull();
  });

  it("leaves the result count out of a narrowed library", () => {
    renderPanel({ query: "alpine", shown: [route()], total: 47 });

    expect(screen.queryByText("1 of 47")).toBeNull();
  });

  it("counts the library in its own words for a library of one", async () => {
    renderPanel({ total: 1 });

    await userEvent.click(screen.getByRole("button", { name: "Search the route library" }));
    expect(screen.getByRole("searchbox")).toHaveAttribute("placeholder", "Search 1 route");
  });

  it("hands every keystroke to its owner rather than keeping one of its own", async () => {
    const onQueryChange = vi.fn();
    renderPanel({ onQueryChange });

    await userEvent.click(screen.getByRole("button", { name: "Search the route library" }));
    await userEvent.type(screen.getByRole("searchbox"), "a");

    expect(onQueryChange).toHaveBeenCalledWith("a");
  });

  it("says a search matched nothing rather than showing an empty column", () => {
    renderPanel({ query: "kaiserstuhl", shown: [] });

    expect(screen.getByText("Nothing here is called that.")).toBeInTheDocument();
  });

  it("blames a filter alone for an empty result when no name was typed", () => {
    renderPanel({ query: "", filters: { ...EMPTY_FILTERS, surfaces: ["gravel"] }, shown: [] });

    expect(screen.getByText("Nothing here matches these filters.")).toBeInTheDocument();
  });

  // Neither a name nor a filter alone is the honest cause when both narrowed
  // the library to nothing: blaming one over the other points the reader at
  // the wrong control to relax.
  it("names both when a search and a filter are narrowing at once", () => {
    renderPanel({
      query: "kaiserstuhl",
      filters: { ...EMPTY_FILTERS, surfaces: ["gravel"] },
      shown: [],
    });

    expect(
      screen.getByText("Nothing here matches this search and these filters."),
    ).toBeInTheDocument();
  });

  it("offers each result as the thing that opens it", async () => {
    const onSelect = vi.fn();
    renderPanel({ query: "alpine", onSelect });

    await userEvent.click(screen.getByRole("button", { name: /Alpine loop — Descent/ }));

    expect(onSelect).toHaveBeenCalledWith(routeKey(route()));
  });

  // The word is what a reader without colour actually reads; the badge is
  // never rendered on a route changeOf calls unchanged.
  it("marks a new or changed row with a text badge", () => {
    renderPanel({ query: "alpine", changeOf: () => "new" });

    expect(screen.getByText("New")).toBeInTheDocument();
  });

  it("marks a row whose revision moved on as updated rather than new", () => {
    renderPanel({ query: "alpine", changeOf: () => "updated" });

    expect(screen.getByText("Updated")).toBeInTheDocument();
  });

  it("shows no badge on a row changeOf calls unchanged", () => {
    renderPanel({ query: "alpine", changeOf: () => null });

    expect(screen.queryByText("New")).toBeNull();
    expect(screen.queryByText("Updated")).toBeNull();
  });

  it("carries the badge onto the route's own card once it is opened", () => {
    renderPanel({ query: "alpine", pickedKey: routeKey(route()), changeOf: () => "new" });

    expect(screen.getByRole("heading", { name: "Alpine loop — Descent" })).toBeInTheDocument();
    expect(screen.getByText("New")).toBeInTheDocument();
  });

  /*
   * The selected row becomes the card in place. Two rows for one route — the
   * closed one and the opened one — would be the same route said twice.
   */
  it("replaces the picked row with the route's card", () => {
    renderPanel({ query: "alpine", pickedKey: routeKey(route()) });

    expect(screen.getByRole("heading", { name: "Alpine loop — Descent" })).toBeInTheDocument();
    expect(screen.getByText("42.5 km")).toBeInTheDocument();
    expect(screen.queryByRole("button", { name: /Alpine loop — Descent/ })).toBeNull();
  });

  it("reports the card's figures in miles and feet for the imperial system", () => {
    renderPanel({
      query: "alpine",
      pickedKey: routeKey(route()),
      unitSystem: "imperial",
    });

    expect(screen.getByText("26.4 mi")).toBeInTheDocument();
    expect(screen.getByText("2,034 ft")).toBeInTheDocument();
  });

  it("shows the predicted moving time and its qualifier in a library result", () => {
    const predicted = route({
      movingSeconds: 6420,
      validation: { biasPercent: -1.2, maePercent: 6.8, p90Percent: 14.1, evaluatedRides: 42 },
    });
    renderPanel({ query: "alpine", shown: [predicted] });

    expect(screen.getByText("1 h 45 min")).toBeInTheDocument();
    expect(screen.getByText("±7% typical")).toBeInTheDocument();
  });

  it("shows nothing for a route nothing has predicted", () => {
    renderPanel({ query: "alpine", shown: [route()], pickedKey: routeKey(route()) });

    expect(screen.getByText("—")).toBeInTheDocument();
    expect(screen.queryByText("±", { exact: false })).toBeNull();
  });

  /*
   * The card's one button opens the route in place. There is no route page to
   * leave for: it swaps this panel for the route's own, which is a thing the
   * page does rather than a thing the address does on its way somewhere else.
   */
  it("opens the route in place rather than linking away to a page", async () => {
    const onOpen = vi.fn();
    renderPanel({ query: "alpine", pickedKey: routeKey(route()), onOpen });

    expect(screen.queryByRole("link", { name: "Open route" })).toBeNull();
    await userEvent.click(screen.getByRole("button", { name: "Open route" }));

    expect(onOpen).toHaveBeenCalledWith(routeKey(route()));
  });

  // Picking a route opens its card without turning the whole library into a
  // result list: selection names one answer, while typing asks for many.
  it("shows only the picked route without a search", () => {
    const other = route({ sourceRouteId: 13, title: "Valley loop" });
    renderPanel({ pickedKey: routeKey(route()), shown: [route(), other] });

    expect(screen.getByRole("heading", { name: "Alpine loop — Descent" })).toBeInTheDocument();
    expect(screen.queryByRole("button", { name: /Valley loop/ })).toBeNull();
  });

  it("says when the library was read, beside where the route is", () => {
    renderPanel({ pickedKey: routeKey(route()) });

    expect(screen.getByText("Alpine loop · read 19:38")).toBeInTheDocument();
  });

  // The card's second line is what there is to say: a route whose title is
  // already its name has nothing to add, and a library that has never been read
  // has no time to give.
  it("leaves out a second line it has nothing to put on", () => {
    renderPanel({
      shown: [route({ title: "Alpine loop", sourceRouteName: "Alpine loop" })],
      pickedKey: routeKey(route({ title: "Alpine loop", sourceRouteName: "Alpine loop" })),
      readAt: null,
      shapes: new Map(),
    });

    expect(screen.queryByText(/Alpine loop · read/)).toBeNull();
  });

  /*
   * The mix bar is the only colour on the card, and every band in it is named on
   * the route's own page. It is hidden from assistive technology because the
   * three figures above it already say what it says.
   */
  it("divides the mix bar by the share each band covers", () => {
    renderPanel({ pickedKey: routeKey(route()) });

    const bar = screen.getByTestId("gradient-mix");
    expect(bar).toHaveAttribute("aria-hidden", "true");
    const runs = Array.from(bar?.children ?? []);
    expect(runs.length).toBeGreaterThan(1);
    const widths = runs.map((run) => Number.parseFloat((run as HTMLElement).style.width));
    expect(widths.reduce((total, width) => total + width, 0)).toBeCloseTo(100, 1);
  });

  // Geometry arrives per route and after the listing. A row is drawn either way,
  // with the glyph and the bar filling in when their shape does.
  it("draws a route whose shape has not arrived", () => {
    renderPanel({ query: "alpine", shapes: new Map() });

    expect(screen.getByRole("button", { name: /Alpine loop — Descent/ })).toBeInTheDocument();
    expect(screen.queryByTestId("gradient-mix")).toBeNull();
  });
});
