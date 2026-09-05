import { render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { describe, expect, it, vi } from "vitest";
import { EMPTY_FILTERS, type LibraryFilters } from "../../lib/filters";
import { focusThumb } from "../../test/filterPanel";
import { FilterPanel } from "./FilterPanel";

function renderPanel(overrides: Partial<React.ComponentProps<typeof FilterPanel>> = {}) {
  const props: React.ComponentProps<typeof FilterPanel> = {
    library: [],
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

  it("stores a distance minimum in metres from a kilometre step", async () => {
    const onFiltersChange = vi.fn();
    renderPanel({ expanded: true, onFiltersChange });

    await focusThumb("Distance min");
    await userEvent.keyboard("{ArrowRight}");

    expect(onFiltersChange).toHaveBeenLastCalledWith({
      ...EMPTY_FILTERS,
      distanceMetres: { min: 5000, max: null },
    });
  });

  it("stores a duration maximum in seconds", async () => {
    const onFiltersChange = vi.fn();
    renderPanel({ expanded: true, onFiltersChange });

    await focusThumb("Duration max");
    await userEvent.keyboard("{ArrowLeft}");

    expect(onFiltersChange).toHaveBeenLastCalledWith({
      ...EMPTY_FILTERS,
      movingSeconds: { min: null, max: 11.75 * 3600 },
    });
  });

  // The ends of the track mean "no bound", so a thumb pushed back to the edge
  // must unset the side rather than filter at the domain's own limit.
  it("leaves a side unbounded again when its thumb returns to the edge", async () => {
    const onFiltersChange = vi.fn();
    renderPanel({
      expanded: true,
      filters: { ...EMPTY_FILTERS, ascentMetres: { min: 100, max: null } },
      onFiltersChange,
    });

    await focusThumb("Ascent min");
    await userEvent.keyboard("{ArrowLeft}");

    expect(onFiltersChange).toHaveBeenLastCalledWith(EMPTY_FILTERS);
  });

  it("reads the set range out beside the legend, and 'any' when nothing is set", () => {
    renderPanel({
      expanded: true,
      filters: {
        ...EMPTY_FILTERS,
        distanceMetres: { min: 20_000, max: 80_000 },
        movingSeconds: { min: 3600, max: null },
      },
    });

    expect(screen.getByText("20 km – 80 km")).toBeInTheDocument();
    expect(screen.getByText("1 h – 12 h")).toBeInTheDocument();
    expect(screen.getByText("any")).toBeInTheDocument();
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
