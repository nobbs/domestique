import { render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { describe, expect, it, vi } from "vitest";
import { EMPTY_FILTERS, type LibraryFilters } from "../../lib/filters";
import { FilterPanel } from "./FilterPanel";

function renderPanel(overrides: Partial<React.ComponentProps<typeof FilterPanel>> = {}) {
  const props: React.ComponentProps<typeof FilterPanel> = {
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
    expect(screen.queryByText("Distance (km)")).not.toBeInTheDocument();
  });

  it("says filters are active without a query having to be typed", () => {
    const filters: LibraryFilters = { ...EMPTY_FILTERS, surfaces: ["gravel"] };
    renderPanel({ filters });

    expect(
      screen.getByRole("button", { name: "Show the library filters — filters are active" }),
    ).toBeInTheDocument();
  });

  it("opens on request and shows every control", async () => {
    const onExpandedChange = vi.fn();
    renderPanel({ onExpandedChange });

    await userEvent.click(screen.getByRole("button", { name: "Show the library filters" }));

    expect(onExpandedChange).toHaveBeenCalledWith(true);
  });

  it("stores a distance minimum in metres from a kilometre input", async () => {
    const onFiltersChange = vi.fn();
    renderPanel({ expanded: true, onFiltersChange });

    const [distanceMin] = screen.getAllByLabelText("Min");
    await userEvent.type(distanceMin as HTMLElement, "1");

    expect(onFiltersChange).toHaveBeenLastCalledWith({
      ...EMPTY_FILTERS,
      distanceMetres: { min: 1000, max: null },
    });
  });

  it("leaves a bound unbounded again when the field is cleared", async () => {
    const onFiltersChange = vi.fn();
    renderPanel({
      expanded: true,
      filters: { ...EMPTY_FILTERS, ascentMetres: { min: 500, max: null } },
      onFiltersChange,
    });

    const ascentMin = screen.getAllByLabelText("Min")[1] as HTMLElement;
    await userEvent.clear(ascentMin);

    expect(onFiltersChange).toHaveBeenLastCalledWith({
      ...EMPTY_FILTERS,
      ascentMetres: { min: null, max: null },
    });
  });

  it("checks a surface by its display name", async () => {
    const onFiltersChange = vi.fn();
    renderPanel({ expanded: true, onFiltersChange });

    await userEvent.click(screen.getByRole("checkbox", { name: "Gravel" }));

    expect(onFiltersChange).toHaveBeenLastCalledWith({ ...EMPTY_FILTERS, surfaces: ["gravel"] });
  });

  it("unchecks a surface without disturbing the others already checked", async () => {
    const onFiltersChange = vi.fn();
    renderPanel({
      expanded: true,
      filters: { ...EMPTY_FILTERS, surfaces: ["asphalt", "gravel"] },
      onFiltersChange,
    });

    await userEvent.click(screen.getByRole("checkbox", { name: "Gravel" }));

    expect(onFiltersChange).toHaveBeenLastCalledWith({ ...EMPTY_FILTERS, surfaces: ["asphalt"] });
  });

  it("stores a max gradient bound in the percent it is shown in", async () => {
    const onFiltersChange = vi.fn();
    renderPanel({ expanded: true, onFiltersChange });

    const gradientMax = screen.getAllByLabelText("Max")[2] as HTMLElement;
    await userEvent.type(gradientMax, "8");

    expect(onFiltersChange).toHaveBeenLastCalledWith({
      ...EMPTY_FILTERS,
      maxGradientPercent: { min: null, max: 8 },
    });
  });

  it("offers every surface class, including the unsurveyed one", () => {
    renderPanel({ expanded: true });

    for (const label of ["Asphalt", "Paving", "Compacted", "Gravel", "Ground", "Unsurveyed"]) {
      expect(screen.getByRole("checkbox", { name: label })).toBeInTheDocument();
    }
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
        maxGradientPercent: { min: null, max: null },
        surfaces: ["asphalt", "gravel"],
      },
      onFiltersChange,
    });

    const clear = screen.getByRole("button", { name: "Clear filters" });
    expect(clear).not.toBeDisabled();
    await userEvent.click(clear);

    expect(onFiltersChange).toHaveBeenCalledWith(EMPTY_FILTERS);
  });
});
