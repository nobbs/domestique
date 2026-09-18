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
    ...overrides,
  };

  return render(<CatalogueFilters {...props} />);
}

describe("CatalogueFilters", () => {
  it("offers no source choice for a library with one source", () => {
    renderFilters();

    expect(screen.queryByRole("group", { name: "Source" })).toBeNull();
  });

  it("offers each source with its count, and adds a chosen one to the filters", async () => {
    const onFiltersChange = vi.fn();
    renderFilters({
      library: [...LIBRARY, route({ sourceRouteId: 3, provider: "local" })],
      onFiltersChange,
    });

    expect(screen.getByRole("button", { name: "VeloPlanner, 2 routes" })).toBeInTheDocument();
    await userEvent.click(screen.getByRole("button", { name: "Planner, 1 route" }));

    expect(onFiltersChange).toHaveBeenCalledWith({ ...EMPTY_FILTERS, providers: ["local"] });
  });

  it("shows one slider per measure, in a card of its own", () => {
    renderFilters();

    expect(screen.getByRole("heading", { name: "Filters" })).toBeInTheDocument();
    expect(screen.getByRole("slider", { name: "Distance min" })).toBeInTheDocument();
    expect(screen.getByRole("slider", { name: "Ascent min" })).toBeInTheDocument();
    expect(screen.getByRole("slider", { name: "Duration min" })).toBeInTheDocument();
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

  it("clears every filter in one action, disabled until one is set", async () => {
    const user = userEvent.setup();
    const onFiltersChange = vi.fn();

    renderFilters();
    expect(screen.getByRole("button", { name: "Clear" })).toBeDisabled();

    const filters: LibraryFilters = { ...EMPTY_FILTERS, ascentMetres: { min: 20, max: null } };
    renderFilters({ filters, onFiltersChange });
    const clear = screen.getAllByRole("button", { name: "Clear" })[1] as HTMLElement;
    expect(clear).toBeEnabled();

    await user.click(clear);

    expect(onFiltersChange).toHaveBeenCalledWith(EMPTY_FILTERS);
  });
});
