import { fireEvent, render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { describe, expect, it, vi } from "vitest";
import type { Route } from "../../api/types";
import { EMPTY_FILTERS, type LibraryFilters } from "../../lib/filters";
import { CatalogueFilters } from "./CatalogueFilters";

function route(overrides: Partial<Route>): Route {
  return {
    provider: "veloplanner",
    sourceRouteId: 1,
    stageOrder: 1,
    title: "Loop",
    sourceRouteName: "Loop",
    routeName: "",
    sourceRevision: "1",
    contentHash: "hash",
    distanceMetres: 10_000,
    ascentMetres: 100,
    descentMetres: 100,
    maxGradientPercent: 5,
    movingSeconds: 3600,
    pointCount: 10,
    ...overrides,
  };
}

const LIBRARY = [
  route({ sourceRouteId: 1 }),
  route({ sourceRouteId: 2, distanceMetres: 47_500, movingSeconds: 4 * 3600 }),
];

function renderFilters(overrides: Partial<React.ComponentProps<typeof CatalogueFilters>> = {}) {
  const props: React.ComponentProps<typeof CatalogueFilters> = {
    library: LIBRARY,
    filters: EMPTY_FILTERS,
    onFiltersChange: () => {},
    narrow: false,
    expanded: false,
    onExpandedChange: () => {},
    ...overrides,
  };

  return render(<CatalogueFilters {...props} />);
}

describe("CatalogueFilters", () => {
  it("stands in view rather than behind a toggle above the breakpoint", () => {
    renderFilters();

    expect(screen.getByRole("slider", { name: "Distance min" })).toBeInTheDocument();
    expect(
      screen.queryByRole("button", { name: /Show the library filters/ }),
    ).not.toBeInTheDocument();
  });

  it("stores a distance bound in metres", () => {
    const onFiltersChange = vi.fn();
    renderFilters({ onFiltersChange });

    screen.getByRole("slider", { name: "Distance min" }).focus();
    fireEvent.change(document.activeElement as HTMLInputElement, { target: { value: "1000" } });

    expect(onFiltersChange).toHaveBeenCalledWith({
      ...EMPTY_FILTERS,
      distanceMetres: { min: 1000, max: null },
    });
  });

  it("stores a duration bound in seconds", () => {
    const onFiltersChange = vi.fn();
    renderFilters({ onFiltersChange });

    screen.getByRole("slider", { name: "Duration max" }).focus();
    fireEvent.change(document.activeElement as HTMLInputElement, { target: { value: "3600" } });

    expect(onFiltersChange).toHaveBeenCalledWith({
      ...EMPTY_FILTERS,
      movingSeconds: { min: null, max: 3600 },
    });
  });

  it("clears every filter in one action", async () => {
    const user = userEvent.setup();
    const onFiltersChange = vi.fn();
    const filters: LibraryFilters = { ...EMPTY_FILTERS, ascentMetres: { min: 20, max: null } };
    renderFilters({ filters, onFiltersChange });

    await user.click(screen.getByRole("button", { name: "Clear filters" }));

    expect(onFiltersChange).toHaveBeenCalledWith(EMPTY_FILTERS);
  });

  it("folds behind a toggle below the breakpoint, open on request", async () => {
    const user = userEvent.setup();
    const onExpandedChange = vi.fn();
    renderFilters({ narrow: true, onExpandedChange });

    expect(screen.getByRole("button", { name: "Show the library filters" })).toBeInTheDocument();
    expect(screen.queryByRole("slider", { name: "Distance min" })).not.toBeInTheDocument();

    await user.click(screen.getByRole("button", { name: "Show the library filters" }));

    expect(onExpandedChange.mock.calls[0]?.[0]).toBe(true);
  });

  it("says filters are active on the toggle without opening it", () => {
    renderFilters({
      narrow: true,
      filters: { ...EMPTY_FILTERS, ascentMetres: { min: 20, max: null } },
    });

    expect(
      screen.getByRole("button", { name: "Show the library filters — filters are active" }),
    ).toBeInTheDocument();
  });
});
