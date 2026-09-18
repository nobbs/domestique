import { render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { useMemo, useState } from "react";
import { describe, expect, it, vi } from "vitest";
import type { Route } from "../../api/types";
import { routeKey } from "../../api/types";
import { EMPTY_FILTERS, type LibraryFilters, matchesFilters } from "../../lib/filters";
import { matchingRoutes } from "../../lib/library";
import type { RouteChange } from "../../lib/seenRoutes";
import { CommandSearch, type RouteShape } from "./CommandSearch";

function route(overrides: Partial<Route> = {}): Route {
  return {
    provider: "veloplanner",
    sourceRouteId: 1,
    stageOrder: 1,
    title: "Rhine Traverse — Valley floor",
    sourceRouteName: "Rhine Traverse",
    routeName: "Valley floor",
    sourceRevision: "2026-08-17",
    contentHash: "hash",
    distanceMetres: 42_500,
    ascentMetres: 100,
    descentMetres: 540,
    maxGradientPercent: 11.4,
    pointCount: 1200,
    ...overrides,
  };
}

const VALLEY = route();
const FOREST = route({
  sourceRouteId: 2,
  title: "Rhine Traverse — Forest ramps",
  routeName: "Forest ramps",
  ascentMetres: 200,
});
const KAISERSTUHL = route({
  sourceRouteId: 3,
  title: "Kaiserstuhl Loop",
  sourceRouteName: "Kaiserstuhl Loop",
  routeName: "",
  ascentMetres: 100,
});
const LIBRARY = [VALLEY, FOREST, KAISERSTUHL];

/**
 * Wires `CommandSearch` the way `AtlasPage` does — the search and filters
 * narrow a real library through the same library functions the page uses.
 */
function Harness({
  library = LIBRARY,
  routeOpen = false,
  initialActiveKey = null,
  onOpen = () => {},
  shapes = new Map<string, RouteShape>(),
  changeOf = () => null,
}: {
  library?: Route[];
  routeOpen?: boolean;
  initialActiveKey?: string | null;
  onOpen?: (key: string) => void;
  shapes?: Map<string, RouteShape>;
  changeOf?: (route: Route) => RouteChange;
}) {
  const [query, setQuery] = useState("");
  const [filters, setFilters] = useState<LibraryFilters>(EMPTY_FILTERS);
  const [open, setOpen] = useState(false);
  const shown = useMemo(
    () => matchingRoutes(library, query).filter((entry) => matchesFilters(entry, filters)),
    [library, query, filters],
  );

  return (
    <CommandSearch
      open={open}
      onOpenChange={setOpen}
      routeOpen={routeOpen}
      library={library}
      shown={shown}
      query={query}
      onQueryChange={setQuery}
      filters={filters}
      onFiltersChange={setFilters}
      activeKey={initialActiveKey}
      onOpen={onOpen}
      shapes={shapes}
      changeOf={changeOf}
    />
  );
}

describe("CommandSearch", () => {
  it("shows the count on the trigger, and the query once typed", async () => {
    render(<Harness />);

    const trigger = screen.getByRole("button", { name: "Search the route library" });
    expect(trigger).toHaveTextContent("Search 3 routes");

    await userEvent.click(trigger);
    await userEvent.type(screen.getByRole("searchbox"), "rhine");
    await userEvent.keyboard("{Escape}");

    expect(screen.getByRole("button", { name: "Search the route library" })).toHaveTextContent(
      "rhine",
    );
  });

  it("marks the trigger with a dot once a filter is active", async () => {
    render(<Harness />);
    await userEvent.click(screen.getByRole("button", { name: "Search the route library" }));
    await userEvent.click(screen.getByRole("button", { name: "Filters" }));
    screen.getByRole("slider", { name: "Ascent min" }).focus();
    await userEvent.keyboard("{ArrowRight}");
    await userEvent.keyboard("{Escape}");

    expect(screen.getByTestId("filters-active-dot")).toBeInTheDocument();
  });

  it("hides the trigger while a route is open", () => {
    render(<Harness routeOpen />);

    expect(screen.queryByRole("button", { name: "Search the route library" })).toBeNull();
  });

  it("opens on ⌘K and closes on a second press", async () => {
    render(<Harness />);

    await userEvent.keyboard("{Meta>}k{/Meta}");
    expect(screen.getByRole("searchbox", { name: "Search the route library" })).toBeVisible();

    await userEvent.keyboard("{Meta>}k{/Meta}");
    expect(screen.queryByRole("searchbox")).toBeNull();
  });

  it("narrows the list as the query is typed", async () => {
    render(<Harness />);
    await userEvent.click(screen.getByRole("button", { name: "Search the route library" }));

    await userEvent.type(screen.getByRole("searchbox"), "kaiserstuhl");

    expect(screen.getByRole("option", { name: /Kaiserstuhl Loop/ })).toBeInTheDocument();
    expect(screen.queryByRole("option", { name: /Rhine Traverse/ })).toBeNull();
  });

  it("moves the active row with the arrow keys and opens it with Enter", async () => {
    const onOpen = vi.fn();
    render(<Harness onOpen={onOpen} />);
    await userEvent.click(screen.getByRole("button", { name: "Search the route library" }));

    // Sorted by title: Kaiserstuhl Loop, then the two Rhine Traverse stages.
    const field = screen.getByRole("searchbox");
    expect(screen.getByRole("option", { name: /Kaiserstuhl Loop/ })).toHaveAttribute(
      "aria-selected",
      "true",
    );

    await userEvent.type(field, "{ArrowDown}");
    expect(screen.getByRole("option", { name: /Forest ramps/ })).toHaveAttribute(
      "aria-selected",
      "true",
    );

    await userEvent.type(field, "{Enter}");

    expect(onOpen).toHaveBeenCalledWith(routeKey(FOREST));
    expect(screen.queryByRole("searchbox")).toBeNull();
  });

  it("leaves Enter on the panel's own buttons to them", async () => {
    const onOpen = vi.fn();
    render(<Harness onOpen={onOpen} />);
    await userEvent.click(screen.getByRole("button", { name: "Search the route library" }));

    const filters = screen.getByRole("button", { name: "Filters" });
    filters.focus();
    await userEvent.keyboard("{Enter}");

    expect(onOpen).not.toHaveBeenCalled();
    expect(filters).toHaveAttribute("aria-expanded", "true");
  });

  it("opens the route a row is clicked on", async () => {
    const onOpen = vi.fn();
    render(<Harness onOpen={onOpen} />);
    await userEvent.click(screen.getByRole("button", { name: "Search the route library" }));

    await userEvent.click(screen.getByRole("option", { name: /Kaiserstuhl Loop/ }));

    expect(onOpen).toHaveBeenCalledWith(routeKey(KAISERSTUHL));
    expect(screen.queryByRole("searchbox")).toBeNull();
  });

  it("closes on Escape without opening a route", async () => {
    const onOpen = vi.fn();
    render(<Harness onOpen={onOpen} />);
    await userEvent.click(screen.getByRole("button", { name: "Search the route library" }));

    await userEvent.keyboard("{Escape}");

    expect(screen.queryByRole("searchbox")).toBeNull();
    expect(onOpen).not.toHaveBeenCalled();
  });

  it("narrows the list with a slider once the filters are open", async () => {
    render(<Harness />);
    await userEvent.click(screen.getByRole("button", { name: "Search the route library" }));

    await userEvent.click(screen.getByRole("button", { name: "Filters" }));
    // The library ascends 100, 200 and 100 m, so the track runs to 200 m by 10 m.
    screen.getByRole("slider", { name: "Ascent min" }).focus();
    await userEvent.keyboard("{ArrowRight}".repeat(11));

    expect(screen.getByRole("option", { name: /Forest ramps/ })).toBeInTheDocument();
    expect(screen.queryByRole("option", { name: /Valley floor/ })).toBeNull();
    expect(screen.queryByRole("option", { name: /Kaiserstuhl Loop/ })).toBeNull();
  });

  it("clears the filters from the panel's own control", async () => {
    render(<Harness />);
    await userEvent.click(screen.getByRole("button", { name: "Search the route library" }));
    await userEvent.click(screen.getByRole("button", { name: "Filters" }));
    screen.getByRole("slider", { name: "Ascent min" }).focus();
    await userEvent.keyboard("{ArrowRight}".repeat(11));
    expect(screen.queryByRole("option", { name: /Valley floor/ })).toBeNull();

    await userEvent.click(screen.getByRole("button", { name: "Clear filters" }));

    expect(screen.getByRole("option", { name: /Valley floor/ })).toBeInTheDocument();
  });

  it("says a search matched nothing rather than showing an empty list", async () => {
    render(<Harness />);
    await userEvent.click(screen.getByRole("button", { name: "Search the route library" }));

    await userEvent.type(screen.getByRole("searchbox"), "nothing is called this");

    expect(screen.getByText("Nothing here is called that.")).toBeInTheDocument();
  });

  it("blames the filters for an empty result when a query and a filter both narrowed to nothing", async () => {
    render(<Harness />);
    await userEvent.click(screen.getByRole("button", { name: "Search the route library" }));
    await userEvent.type(screen.getByRole("searchbox"), "kaiserstuhl");
    await userEvent.click(screen.getByRole("button", { name: "Filters" }));
    screen.getByRole("slider", { name: "Ascent min" }).focus();
    await userEvent.keyboard("{ArrowRight}".repeat(11));

    expect(
      screen.getByText("Nothing here matches this search and these filters."),
    ).toBeInTheDocument();
  });

  it("lands the active row on the route already open when reopened", async () => {
    render(<Harness initialActiveKey={routeKey(FOREST)} />);

    await userEvent.click(screen.getByRole("button", { name: "Search the route library" }));

    expect(screen.getByRole("option", { name: /Forest ramps/ })).toHaveAttribute(
      "aria-selected",
      "true",
    );
  });
});
