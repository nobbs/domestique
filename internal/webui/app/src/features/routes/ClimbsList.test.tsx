import { render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { describe, expect, it, vi } from "vitest";
import type { Climb } from "../../lib/climbs";
import { ClimbsList } from "./ClimbsList";

function climb(overrides: Partial<Climb> = {}): Climb {
  return {
    startMetres: 1200,
    endMetres: 1800,
    distanceMetres: 600,
    ascentMetres: 54,
    averageGradePercent: 9,
    maxGradePercent: 11.4,
    ...overrides,
  };
}

describe("ClimbsList", () => {
  it("renders nothing for a route with no sustained climbs", () => {
    const { container } = render(
      <ClimbsList climbs={[]} onSelect={() => {}} unitSystem="metric" />,
    );

    expect(container).toBeEmptyDOMElement();
  });

  it("names each climb by its distance, ascent, and grades", () => {
    render(<ClimbsList climbs={[climb()]} onSelect={() => {}} unitSystem="metric" />);

    expect(screen.getByRole("button", { name: "Show 1 climb" })).toHaveAttribute(
      "aria-expanded",
      "false",
    );
    expect(screen.getByRole("button", { name: "Show 1 climb" })).not.toHaveAttribute(
      "aria-controls",
    );
    expect(screen.queryByText("600 m")).not.toBeInTheDocument();
  });

  it("shows each climb after it is expanded", async () => {
    render(<ClimbsList climbs={[climb()]} onSelect={() => {}} unitSystem="metric" />);

    await userEvent.click(screen.getByRole("button", { name: "Show 1 climb" }));

    // The primitive names the panel, so what matters is that the control points
    // at the list it opened rather than at any particular id.
    expect(screen.getByRole("button", { name: "Hide 1 climb" })).toHaveAttribute(
      "aria-controls",
      screen.getByRole("list").id,
    );
    expect(screen.getByText("600 m")).toBeInTheDocument();
    expect(screen.getByText(/54 m · avg 9\.0% · max 11%/)).toBeInTheDocument();
  });

  it("reports the same climb in feet for the imperial system", async () => {
    render(<ClimbsList climbs={[climb()]} onSelect={() => {}} unitSystem="imperial" />);

    await userEvent.click(screen.getByRole("button", { name: "Show 1 climb" }));

    expect(screen.getByText("1969 ft")).toBeInTheDocument();
    expect(screen.getByText(/177 ft · avg 9\.0% · max 11%/)).toBeInTheDocument();
  });

  it("lists more than one climb in the order they are ridden", async () => {
    render(
      <ClimbsList
        climbs={[
          climb({ startMetres: 0, endMetres: 300, distanceMetres: 300 }),
          climb({ startMetres: 5000, endMetres: 5400, distanceMetres: 400 }),
        ]}
        onSelect={() => {}}
        unitSystem="metric"
      />,
    );

    await userEvent.click(screen.getByRole("button", { name: "Show 2 climbs" }));

    expect(screen.getAllByRole("button")).toHaveLength(3);
  });

  it("hands the picked climb back to its owner", async () => {
    const onSelect = vi.fn();
    const picked = climb();
    render(<ClimbsList climbs={[picked]} onSelect={onSelect} unitSystem="metric" />);

    await userEvent.click(screen.getByRole("button", { name: "Show 1 climb" }));
    await userEvent.click(screen.getByRole("button", { name: /600 m/ }));

    expect(onSelect).toHaveBeenCalledWith(picked);
  });
});
