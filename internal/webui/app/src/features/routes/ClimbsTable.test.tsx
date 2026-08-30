/**
 * The climbs, as a table of one subject.
 *
 * Folded by default, because the count and the worst of them is the deciding
 * fact and the rest is detail — and the summary that says so is the control
 * that opens it, rather than a caption with a separate word to press.
 *
 * Both gradients are carried: the average says what a climb asks of you over
 * its length, the steepest hundred metres says whether you get up it at all.
 * Nothing about the weather is here, however tempting: everything the forecast
 * knows is drawn along the dock's band at every reading rather than only at the
 * few a climb happens to fall on.
 */

import { render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { describe, expect, it, vi } from "vitest";
import type { Climb } from "../../lib/climbs";
import { ClimbsTable } from "./ClimbsTable";

const CLIMBS: Climb[] = [
  {
    startMetres: 8_100,
    endMetres: 20_000,
    distanceMetres: 11_900,
    ascentMetres: 699,
    averageGradePercent: 5.9,
    maxGradePercent: 8.1,
  },
  {
    startMetres: 32_000,
    endMetres: 35_000,
    distanceMetres: 3_000,
    ascentMetres: 350,
    averageGradePercent: 11.6,
    maxGradePercent: 13.4,
  },
];

function renderTable(climbs = CLIMBS, onSelect = vi.fn()) {
  render(<ClimbsTable climbs={climbs} unitSystem="metric" onSelect={onSelect} />);

  return onSelect;
}

describe("ClimbsTable", () => {
  it("renders nothing for a route with no sustained climb", () => {
    const { container } = render(
      <ClimbsTable climbs={[]} unitSystem="metric" onSelect={() => {}} />,
    );

    expect(container).toBeEmptyDOMElement();
  });

  it("says how many and which is worst before it is opened", () => {
    renderTable();

    // The line says what the route has; the control's name says what pressing
    // it does, which is the half a summary cannot carry.
    expect(screen.getByText(/2 climbs · biggest 11\.9 km at 5\.9%/)).toBeInTheDocument();
    expect(screen.getByRole("button", { name: "Show 2 climbs" })).toBeInTheDocument();
    expect(screen.queryByText("Ascent")).toBeNull();
  });

  it("carries both the average and the steepest gradient once opened", async () => {
    renderTable();

    await userEvent.click(screen.getByRole("button", { name: "Show 2 climbs" }));

    expect(screen.getByText("5.9%")).toBeInTheDocument();
    expect(screen.getByText("8.1%")).toBeInTheDocument();
    expect(screen.getByText("Avg")).toBeInTheDocument();
    expect(screen.getByText("Max")).toBeInTheDocument();
  });

  it("opens the shared window on the climb that was pressed", async () => {
    const onSelect = renderTable();

    await userEvent.click(screen.getByRole("button", { name: "Show 2 climbs" }));
    // The row rather than the summary above it, which also names the biggest.
    // Its ascent appears nowhere else, so it is the unambiguous handle.
    await userEvent.click(screen.getByRole("button", { name: /699 m/ }));

    expect(onSelect).toHaveBeenCalledWith(CLIMBS[0]);
  });
});
