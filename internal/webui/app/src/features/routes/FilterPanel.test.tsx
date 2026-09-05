import { render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { describe, expect, it, vi } from "vitest";
import type { Route } from "../../api/types";
import { EMPTY_FILTERS, type LibraryFilters } from "../../lib/filters";
import { focusThumb } from "../../test/filterPanel";
import { FilterPanel } from "./FilterPanel";

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

// Sets the domains: distance 0–48 km by 1 km, ascent 0–1200 m by 20 m,
// duration 0–4 h by 5 min.
const LIBRARY = [
  route({ sourceRouteId: 1 }),
  route({ sourceRouteId: 2, distanceMetres: 47_500, ascentMetres: 1_190, movingSeconds: 4 * 3600 }),
];

function renderPanel(overrides: Partial<React.ComponentProps<typeof FilterPanel>> = {}) {
  const props: React.ComponentProps<typeof FilterPanel> = {
    library: LIBRARY,
    filters: EMPTY_FILTERS,
    onFiltersChange: () => {},
    expanded: false,
    onExpandedChange: () => {},
    ...overrides,
  };

  return render(<FilterPanel {...props} />);
}

describe("FilterPanel", () => {
  it("folds the controls away until asked for", () => {
    renderPanel();

    expect(screen.getByRole("button", { name: "Show the library filters" })).toBeInTheDocument();
    expect(screen.queryByRole("slider", { name: "Distance min" })).not.toBeInTheDocument();
  });

  it("says filters are active without a query having to be typed", () => {
    const filters: LibraryFilters = { ...EMPTY_FILTERS, ascentMetres: { min: 500, max: null } };
    renderPanel({ filters });

    expect(
      screen.getByRole("button", { name: "Show the library filters — filters are active" }),
    ).toBeInTheDocument();
  });

  it("opens on request", async () => {
    const onExpandedChange = vi.fn();
    renderPanel({ onExpandedChange });

    await userEvent.click(screen.getByRole("button", { name: "Show the library filters" }));

    expect(onExpandedChange.mock.calls[0]?.[0]).toBe(true);
  });

  it("offers two thumbs each for distance, ascent and duration", () => {
    renderPanel({ expanded: true });

    for (const name of ["Distance", "Ascent", "Duration"]) {
      expect(screen.getByRole("slider", { name: `${name} min` })).toBeInTheDocument();
      expect(screen.getByRole("slider", { name: `${name} max` })).toBeInTheDocument();
    }
  });

  it("stores a distance minimum in metres, one library-derived step in", async () => {
    const onFiltersChange = vi.fn();
    renderPanel({ expanded: true, onFiltersChange });

    await focusThumb("Distance min");
    await userEvent.keyboard("{ArrowRight}");

    expect(onFiltersChange).toHaveBeenLastCalledWith({
      ...EMPTY_FILTERS,
      distanceMetres: { min: 1000, max: null },
    });
  });

  it("stores a duration maximum in seconds", async () => {
    const onFiltersChange = vi.fn();
    renderPanel({ expanded: true, onFiltersChange });

    await focusThumb("Duration max");
    await userEvent.keyboard("{ArrowLeft}");

    expect(onFiltersChange).toHaveBeenLastCalledWith({
      ...EMPTY_FILTERS,
      movingSeconds: { min: null, max: 4 * 3600 - 300 },
    });
  });

  // The ends of the track mean "no bound", so a thumb pushed back to the edge
  // must unset the side rather than filter at the domain's own limit.
  it("leaves a side unbounded again when its thumb returns to the edge", async () => {
    const onFiltersChange = vi.fn();
    renderPanel({
      expanded: true,
      filters: { ...EMPTY_FILTERS, ascentMetres: { min: 20, max: null } },
      onFiltersChange,
    });

    await focusThumb("Ascent min");
    await userEvent.keyboard("{ArrowLeft}");

    expect(onFiltersChange).toHaveBeenLastCalledWith(EMPTY_FILTERS);
  });

  it("reads the set sides out beside the legend, and 'any' when nothing is set", () => {
    renderPanel({
      expanded: true,
      filters: {
        ...EMPTY_FILTERS,
        distanceMetres: { min: 10_000, max: 40_000 },
        movingSeconds: { min: 3600, max: null },
      },
    });

    expect(screen.getByText("10 km – 40 km")).toBeInTheDocument();
    expect(screen.getByText("from 1 h")).toBeInTheDocument();
    expect(screen.getByText("any")).toBeInTheDocument();
  });

  it("ends each track where the library does", () => {
    renderPanel({ expanded: true });

    expect(screen.getByRole("slider", { name: "Distance max" })).toHaveAttribute("max", "48000");
    expect(screen.getByRole("slider", { name: "Ascent max" })).toHaveAttribute("max", "1200");
    expect(screen.getByRole("slider", { name: "Duration max" })).toHaveAttribute("max", "14400");
    expect(screen.getByText("48 km")).toBeInTheDocument();
  });

  it("disables clearing when nothing is set", () => {
    renderPanel({ expanded: true });

    expect(screen.getByRole("button", { name: "Clear filters" })).toBeDisabled();
  });

  it("clears every filter in one action", async () => {
    const onFiltersChange = vi.fn();
    renderPanel({
      expanded: true,
      filters: {
        distanceMetres: { min: 1000, max: null },
        ascentMetres: { min: null, max: 2000 },
        movingSeconds: { min: 3600, max: null },
      },
      onFiltersChange,
    });

    const clear = screen.getByRole("button", { name: "Clear filters" });
    expect(clear).not.toBeDisabled();
    await userEvent.click(clear);

    expect(onFiltersChange).toHaveBeenCalledWith(EMPTY_FILTERS);
  });
});
