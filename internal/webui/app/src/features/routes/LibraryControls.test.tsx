import { render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { describe, expect, it, vi } from "vitest";
import { LibraryControls, type LibraryControlsProps } from "./LibraryControls";

function renderControls(overrides: Partial<LibraryControlsProps> = {}) {
  const props: LibraryControlsProps = {
    query: "",
    onQueryChange: vi.fn(),
    sort: "name",
    onSortChange: vi.fn(),
    shown: 6,
    total: 6,
    ...overrides,
  };
  render(<LibraryControls {...props} />);

  return props;
}

describe("LibraryControls", () => {
  it("offers a named search box and a named order", () => {
    renderControls();

    expect(screen.getByRole("searchbox", { name: "Search" })).toBeInTheDocument();
    expect(screen.getByRole("combobox", { name: "Sort by" })).toBeInTheDocument();
  });

  it("groups both controls in one named search landmark", () => {
    renderControls();

    // Asserted on the element rather than through its role: `search` is a
    // landmark in a browser, but jsdom's accessibility mapping does not yet know
    // the element and reports no role for it at all.
    const landmark = document.querySelector("search");
    expect(landmark).toHaveAttribute("aria-label", "Route library");
    expect(landmark).toContainElement(screen.getByRole("searchbox", { name: "Search" }));
    expect(landmark).toContainElement(screen.getByRole("combobox", { name: "Sort by" }));
  });

  it("reports every typed character to the page", async () => {
    const user = userEvent.setup();
    const props = renderControls();

    await user.type(screen.getByRole("searchbox", { name: "Search" }), "rh");

    expect(props.onQueryChange).toHaveBeenNthCalledWith(1, "r");
    expect(props.onQueryChange).toHaveBeenNthCalledWith(2, "h");
  });

  it("is reachable and usable by keyboard alone", async () => {
    const user = userEvent.setup();
    const props = renderControls();

    await user.tab();
    expect(screen.getByRole("searchbox", { name: "Search" })).toHaveFocus();
    await user.tab();
    const sort = screen.getByRole("combobox", { name: "Sort by" });
    expect(sort).toHaveFocus();

    await user.selectOptions(sort, "ascent");
    expect(props.onSortChange).toHaveBeenCalledWith("ascent");
  });

  it("names each order with the direction it runs in", () => {
    renderControls();

    expect(screen.getByRole("option", { name: "Name (A–Z)" })).toBeInTheDocument();
    expect(screen.getByRole("option", { name: "Distance (longest first)" })).toBeInTheDocument();
    expect(
      screen.getByRole("option", { name: "Ascent (most climbing first)" }),
    ).toBeInTheDocument();
  });

  it("says politely how much of the library is left once a search narrows it", () => {
    renderControls({ query: "rhine", shown: 3, total: 6 });

    expect(screen.getByText("Showing 3 of 6 stages")).toBeInTheDocument();
  });

  it("says nothing while the whole library is on show", () => {
    renderControls();

    expect(screen.queryByText(/Showing/)).not.toBeInTheDocument();
  });
});
