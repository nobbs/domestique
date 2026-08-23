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
    const { container } = render(<ClimbsList climbs={[]} onSelect={() => {}} />);

    expect(container).toBeEmptyDOMElement();
  });

  it("names each climb by its distance, ascent, and grades", () => {
    render(<ClimbsList climbs={[climb()]} onSelect={() => {}} />);

    expect(screen.getByText("600 m")).toBeInTheDocument();
    expect(screen.getByText(/54 m · avg 9\.0% · max 11%/)).toBeInTheDocument();
  });

  it("lists more than one climb in the order they are ridden", () => {
    render(
      <ClimbsList
        climbs={[
          climb({ startMetres: 0, endMetres: 300, distanceMetres: 300 }),
          climb({ startMetres: 5000, endMetres: 5400, distanceMetres: 400 }),
        ]}
        onSelect={() => {}}
      />,
    );

    expect(screen.getAllByRole("button")).toHaveLength(2);
  });

  it("hands the picked climb back to its owner", async () => {
    const onSelect = vi.fn();
    const picked = climb();
    render(<ClimbsList climbs={[picked]} onSelect={onSelect} />);

    await userEvent.click(screen.getByRole("button"));

    expect(onSelect).toHaveBeenCalledWith(picked);
  });
});
