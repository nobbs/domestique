/**
 * The basemap chooser, folded and unfolded.
 *
 * What these assert is that the choice is a real choice to something other than
 * a pointer: one named affordance whose state is reported rather than drawn, a
 * group of radios behind it, and exactly one of them checked — the one the map
 * says is on screen, which is not always the one the reader last named.
 */

import { render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { useState } from "react";
import { describe, expect, it, vi } from "vitest";
import type { Basemap } from "../api/types";
import { BasemapPicker } from "./BasemapPicker";

const STREETS: Basemap = {
  name: "Streets",
  styleUrl: "https://tiles.example.test/styles/liberty",
  darkCartography: false,
};
const SATELLITE: Basemap = {
  name: "Satellite",
  styleUrl: "https://imagery.example.test/styles/hybrid",
  darkCartography: true,
};

/**
 * Holds the fold, as the map does.
 *
 * The component reports a press rather than remembering it, because the portal
 * that moves it into MapLibre's cluster remounts it — so a test that presses
 * the button needs somebody to report it to.
 */
function Held({
  basemaps = [STREETS, SATELLITE],
  selectedName = "Streets",
  onSelect = () => {},
}: {
  basemaps?: Basemap[];
  selectedName?: string;
  onSelect?: (name: string) => void;
}) {
  const [expanded, setExpanded] = useState(false);

  return (
    <BasemapPicker
      basemaps={basemaps}
      selectedName={selectedName}
      onSelect={onSelect}
      expanded={expanded}
      onExpandedChange={setExpanded}
    />
  );
}

/** Unfolds the list, which is where everything below is. */
async function open() {
  await userEvent.click(screen.getByRole("button", { name: "Choose the basemap" }));
}

describe("BasemapPicker", () => {
  it("says nothing at all where the operator offers one basemap", () => {
    const { container } = render(<Held basemaps={[STREETS]} />);

    expect(container).toBeEmptyDOMElement();
  });

  it("keeps the names folded away until they are asked for", () => {
    render(<Held />);

    expect(screen.queryByRole("radiogroup")).not.toBeInTheDocument();
    expect(screen.getByRole("button", { name: "Choose the basemap" })).toHaveAttribute(
      "aria-expanded",
      "false",
    );
  });

  it("offers every configured basemap once unfolded", async () => {
    render(<Held />);
    await open();

    const group = screen.getByRole("radiogroup", { name: "Basemap" });
    expect(group).toBeInTheDocument();
    expect(screen.getByRole("radio", { name: "Streets" })).toBeInTheDocument();
    expect(screen.getByRole("radio", { name: "Satellite" })).toBeInTheDocument();
    expect(screen.getAllByRole("radio")).toHaveLength(2);
  });

  /*
   * The name is the whole of what the button says, so it has to change with the
   * fold: the mark inside it does not, and `aria-expanded` alone would leave a
   * reader who cannot see the list guessing what pressing it does next.
   */
  it("names what pressing the mark would do, either way", async () => {
    render(<Held />);
    await open();

    const toggle = screen.getByRole("button", { name: "Hide the basemap choices" });
    expect(toggle).toHaveAttribute("aria-expanded", "true");
    expect(toggle).toHaveAttribute("aria-controls", screen.getByRole("radiogroup").id);
  });

  it("marks the basemap on screen, and only that one", async () => {
    render(<Held selectedName="Satellite" />);
    await open();

    expect(screen.getByRole("radio", { name: "Satellite" })).toBeChecked();
    expect(screen.getByRole("radio", { name: "Streets" })).not.toBeChecked();
  });

  it("reports a pick by name", async () => {
    const onSelect = vi.fn();
    render(<Held onSelect={onSelect} />);
    await open();

    await userEvent.click(screen.getByRole("radio", { name: "Satellite" }));

    expect(onSelect).toHaveBeenCalledWith("Satellite");
  });

  /*
   * The mark follows the ground actually loaded rather than the press: a name
   * the operator has since dropped falls back to the first entry, and the
   * checked radio has to say so rather than claiming a basemap that is not on
   * screen.
   */
  it("marks nothing of its own accord when the caller names no entry", async () => {
    render(<Held selectedName="Ordnance Survey" />);
    await open();

    expect(screen.queryAllByRole("radio", { checked: true })).toHaveLength(0);
  });
});
