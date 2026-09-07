import { render, screen, within } from "@testing-library/react";
import { describe, expect, it } from "vitest";
import type { ActivitySplit } from "../../api/types";
import { RideSplits } from "./RideSplits";

function split(overrides: Partial<ActivitySplit> = {}): ActivitySplit {
  return { distanceMetres: 1000, movingSeconds: 180, ascentMetres: 0, ...overrides };
}

/** The rows of the table, header aside. */
function rows() {
  return within(screen.getByRole("table")).getAllByRole("row").slice(1);
}

describe("RideSplits", () => {
  it("counts the distance up across the ride rather than repeating a kilometre", () => {
    render(
      <RideSplits splits={[split(), split(), split({ distanceMetres: 500, movingSeconds: 90 })]} />,
    );

    const cells = rows().map((row) => within(row).getAllByRole("cell")[0]?.textContent);
    expect(cells).toEqual(["1.0 km", "2.0 km", "2.5 km"]);
  });

  it("works each stretch's speed out from its own distance and moving time", () => {
    render(<RideSplits splits={[split({ distanceMetres: 1000, movingSeconds: 120 })]} />);

    expect(screen.getByText("30.0 km/h")).toBeInTheDocument();
    expect(screen.getByText("2 min")).toBeInTheDocument();
  });

  // A gap in the recording is not a slow kilometre.
  it("gives no speed to a stretch the odometer never advanced over", () => {
    render(<RideSplits splits={[split(), split({ movingSeconds: 0 })]} />);

    const second = within(rows()[1] as HTMLElement).getAllByRole("cell");
    expect(second[1]).toHaveTextContent("0 s");
    expect(second[2]).toHaveTextContent("—");
  });

  // A ride with no strap should not be ruled a column of dashes.
  it("leaves out a column no stretch of the ride carried", () => {
    render(<RideSplits splits={[split({ heartRateBpm: 148 })]} />);

    expect(screen.getByRole("columnheader", { name: "Heart rate" })).toBeInTheDocument();
    expect(screen.getByText("148 bpm")).toBeInTheDocument();
    expect(screen.queryByRole("columnheader", { name: "Power" })).not.toBeInTheDocument();
    expect(screen.queryByRole("columnheader", { name: "Ascent" })).not.toBeInTheDocument();
  });

  it("shows measured power where the bicycle carried a meter", () => {
    render(<RideSplits splits={[split({ powerWatts: 212.6 }), split()]} />);

    expect(screen.getByRole("columnheader", { name: "Power" })).toBeInTheDocument();
    expect(screen.getByText("213 W")).toBeInTheDocument();
    const second = within(rows()[1] as HTMLElement).getAllByRole("cell");
    expect(second[second.length - 1]).toHaveTextContent("—");
  });

  it("shows ascent once any stretch of the ride climbed", () => {
    render(<RideSplits splits={[split({ ascentMetres: 42 }), split()]} />);

    expect(screen.getByRole("columnheader", { name: "Ascent" })).toBeInTheDocument();
    expect(screen.getByText("42 m")).toBeInTheDocument();
  });

  // Every stretch standing still leaves no fastest one to scale against, and
  // nought over nought is not a width. React discards the NaN, so what pins the
  // guard is the bar asking for a width at all.
  it("draws an empty bar for a ride no stretch of which was ridden", () => {
    const { container } = render(
      <RideSplits splits={[split({ movingSeconds: 0 }), split({ movingSeconds: 0 })]} />,
    );

    const bars = container.querySelectorAll<HTMLElement>('[aria-hidden="true"] > span');
    expect(bars).toHaveLength(2);
    for (const bar of bars) {
      expect(bar.style.width).toBe("0%");
    }
  });

  it("shows nothing at all for a ride with no splits", () => {
    const { rerender } = render(<RideSplits splits={[]} />);
    expect(screen.queryByLabelText("Splits")).not.toBeInTheDocument();

    rerender(<RideSplits splits={undefined} />);
    expect(screen.queryByLabelText("Splits")).not.toBeInTheDocument();
  });
});
