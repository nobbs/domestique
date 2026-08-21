import { render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { MemoryRouter } from "react-router";
import { describe, expect, it, vi } from "vitest";
import type { Position, Route } from "../../api/types";
import { routeKey } from "../../api/types";
import { SearchPanel, type SearchPanelProps } from "./SearchPanel";

function route(overrides: Partial<Route> = {}): Route {
  return {
    routeId: 12,
    stageOrder: 2,
    title: "Alpine loop — Descent",
    routeName: "Alpine loop",
    stageName: "Descent",
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
    selectedKey: null,
    onSelect: () => {},
    onOpen: () => {},
    shapes: new Map([[routeKey(route()), { coordinates: CLIMB }]]),
    readAt: "19:38",
    ...overrides,
  };

  return render(
    <MemoryRouter>
      <SearchPanel {...props} />
    </MemoryRouter>,
  );
}

describe("SearchPanel", () => {
  /*
   * At rest the panel is one pill over the map. A results column standing open
   * on arrival would cover the cartography the page exists to show.
   */
  it("is only a pill until it is used", () => {
    renderPanel();

    expect(screen.getByRole("searchbox", { name: "Search the route library" })).toHaveAttribute(
      "placeholder",
      "Search 47 routes",
    );
    expect(screen.queryByRole("list")).toBeNull();
  });

  it("counts the library in its own words for a library of one", () => {
    renderPanel({ total: 1 });

    expect(screen.getByRole("searchbox")).toHaveAttribute("placeholder", "Search 1 route");
  });

  it("hands every keystroke to its owner rather than keeping one of its own", async () => {
    const onQueryChange = vi.fn();
    renderPanel({ onQueryChange });

    await userEvent.type(screen.getByRole("searchbox"), "a");

    expect(onQueryChange).toHaveBeenCalledWith("a");
  });

  // The count and the column are the same fact, derived from the same filter,
  // so they cannot disagree — and "47 of 47" is a sum with no question behind it.
  it("says how much was left only once the library has been narrowed", () => {
    const { unmount } = renderPanel({ query: "alpine", shown: [route()], total: 47 });
    expect(screen.getByText("1 of 47")).toBeInTheDocument();
    unmount();

    renderPanel({ query: "", total: 47 });
    expect(screen.queryByText(/of 47/)).toBeNull();
  });

  it("says a search matched nothing rather than showing an empty column", () => {
    renderPanel({ query: "kaiserstuhl", shown: [] });

    expect(screen.getByText("Nothing here is called that.")).toBeInTheDocument();
  });

  it("offers each result as the thing that opens it", async () => {
    const onSelect = vi.fn();
    renderPanel({ query: "alpine", onSelect });

    await userEvent.click(screen.getByRole("button", { name: /Alpine loop — Descent/ }));

    expect(onSelect).toHaveBeenCalledWith(routeKey(route()));
  });

  /*
   * The selected row becomes the card in place. Two rows for one route — the
   * closed one and the opened one — would be the same route said twice.
   */
  it("replaces the picked row with the route's card", () => {
    renderPanel({ query: "alpine", selectedKey: routeKey(route()) });

    expect(screen.getByRole("heading", { name: "Alpine loop — Descent" })).toBeInTheDocument();
    expect(screen.getByText("42.5 km")).toBeInTheDocument();
    expect(screen.queryByRole("button", { name: /Alpine loop — Descent/ })).toBeNull();
  });

  /*
   * The card's one button opens the route in place. There is no route page to
   * leave for: it swaps this panel for the route's own, which is a thing the
   * page does rather than a thing the address does on its way somewhere else.
   */
  it("opens the route in place rather than linking away to a page", async () => {
    const onOpen = vi.fn();
    renderPanel({ query: "alpine", selectedKey: routeKey(route()), onOpen });

    expect(screen.queryByRole("link", { name: "Open route" })).toBeNull();
    await userEvent.click(screen.getByRole("button", { name: "Open route" }));

    expect(onOpen).toHaveBeenCalledWith(routeKey(route()));
  });

  // Picking a route opens the card whether or not anything was typed, so the
  // column has to grow for a selection made on the map as well.
  it("grows for a route picked without a search", () => {
    renderPanel({ selectedKey: routeKey(route()) });

    expect(screen.getByRole("heading", { name: "Alpine loop — Descent" })).toBeInTheDocument();
  });

  it("says when the library was read, beside where the route is", () => {
    renderPanel({ selectedKey: routeKey(route()) });

    expect(screen.getByText("Alpine loop · read 19:38")).toBeInTheDocument();
  });

  // The card's second line is what there is to say: a route whose title is
  // already its name has nothing to add, and a library that has never been read
  // has no time to give.
  it("leaves out a second line it has nothing to put on", () => {
    renderPanel({
      shown: [route({ title: "Alpine loop", routeName: "Alpine loop" })],
      selectedKey: routeKey(route({ title: "Alpine loop", routeName: "Alpine loop" })),
      readAt: null,
      shapes: new Map(),
    });

    expect(document.querySelector(".route-card__where")).toBeNull();
  });

  /*
   * The mix bar is the only colour on the card, and every band in it is named on
   * the route's own page. It is hidden from assistive technology because the
   * three figures above it already say what it says.
   */
  it("divides the mix bar by the share each band covers", () => {
    renderPanel({ selectedKey: routeKey(route()) });

    const bar = document.querySelector(".route-card__mix");
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
    expect(document.querySelector(".route-card__mix")).toBeNull();
  });
});
