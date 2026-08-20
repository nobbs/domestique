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
    view: "grid",
    onViewChange: vi.fn(),
    shown: 6,
    total: 6,
    ...overrides,
  };
  render(<LibraryControls {...props} />);

  return props;
}

describe("LibraryControls", () => {
  it("offers a named search box, a named order and a named pair of views", () => {
    renderControls();

    expect(screen.getByRole("searchbox", { name: "Search" })).toBeInTheDocument();
    expect(screen.getByRole("combobox", { name: "Sort by" })).toBeInTheDocument();
    expect(screen.getByRole("radiogroup", { name: "View" })).toBeInTheDocument();
    expect(screen.getByRole("radio", { name: "Grid" })).toBeInTheDocument();
    expect(screen.getByRole("radio", { name: "Table" })).toBeInTheDocument();
  });

  it("shows which presentation the library is in", () => {
    renderControls({ view: "table" });

    expect(screen.getByRole("radio", { name: "Table" })).toBeChecked();
    expect(screen.getByRole("radio", { name: "Grid" })).not.toBeChecked();
  });

  it("reports a change of presentation back to the page", async () => {
    const user = userEvent.setup();
    const props = renderControls();

    await user.click(screen.getByRole("radio", { name: "Table" }));

    expect(props.onViewChange).toHaveBeenCalledExactlyOnceWith("table");
  });

  it("leaves the presentation alone when the one already chosen is pressed again", async () => {
    const user = userEvent.setup();
    // Radix reports deselection as an empty value, and an empty value is not one
    // of the two presentations: pressing "Grid" while in the grid must not leave
    // the page with no view to render.
    const props = renderControls();

    await user.click(screen.getByRole("radio", { name: "Grid" }));

    expect(props.onViewChange).not.toHaveBeenCalled();
  });

  it("groups every control in one named search landmark", () => {
    renderControls();

    // Asserted on the element rather than through its role: `search` is a
    // landmark in a browser, but jsdom's accessibility mapping does not yet know
    // the element and reports no role for it at all.
    const landmark = document.querySelector("search");
    expect(landmark).toHaveAttribute("aria-label", "Route library");
    expect(landmark).toContainElement(screen.getByRole("searchbox", { name: "Search" }));
    expect(landmark).toContainElement(screen.getByRole("combobox", { name: "Sort by" }));
    expect(landmark).toContainElement(screen.getByRole("radiogroup", { name: "View" }));
  });

  it("reports what the box holds back to the page", async () => {
    const user = userEvent.setup();
    // The box is controlled by the page. This harness holds the query at "" and
    // never feeds a keystroke back, so what arrives here is one press against an
    // empty box rather than a growing word; `StagesPage` is where a whole word is
    // typed against the state that actually holds it.
    const props = renderControls();

    await user.type(screen.getByRole("searchbox", { name: "Search" }), "r");

    expect(props.onQueryChange).toHaveBeenCalledTimes(1);
    expect(props.onQueryChange).toHaveBeenCalledWith("r");
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

    // The pair of views is one tab stop rather than two: the arrows walk between
    // them and the press is what chooses one.
    await user.tab();
    expect(screen.getByRole("radio", { name: "Grid" })).toHaveFocus();
    await user.keyboard("{ArrowRight}");
    expect(screen.getByRole("radio", { name: "Table" })).toHaveFocus();
    await user.keyboard("{Enter}");
    expect(props.onViewChange).toHaveBeenCalledWith("table");
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
