import { render, screen, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { describe, expect, it } from "vitest";
import type { ActivitySplit } from "../../api/types";
import { RideSplits } from "./RideSplits";

function split(overrides: Partial<ActivitySplit> = {}): ActivitySplit {
  return { distanceMetres: 1000, movingSeconds: 180, ascentMetres: 0, ...overrides };
}

/** The table, once asked for. */
async function table() {
  await userEvent.click(screen.getByRole("button", { name: "Show the table" }));

  return screen.getByRole("table");
}

/** The rows of the table, header aside, once it has been asked for. */
function rows() {
  return within(screen.getByRole("table")).getAllByRole("row").slice(1);
}

describe("RideSplits", () => {
  it("keeps the table behind a toggle", async () => {
    render(<RideSplits splits={[split()]} />);

    expect(screen.queryByRole("table")).not.toBeInTheDocument();
    await table();
    expect(screen.getByRole("button", { name: "Hide the table" })).toHaveAttribute(
      "aria-expanded",
      "true",
    );
  });

  it("counts the distance up across the ride rather than repeating a kilometre", async () => {
    render(
      <RideSplits splits={[split(), split(), split({ distanceMetres: 500, movingSeconds: 90 })]} />,
    );

    await table();
    const cells = rows().map((row) => within(row).getAllByRole("cell")[0]?.textContent);
    expect(cells).toEqual(["1.0 km", "2.0 km", "2.5 km"]);
  });

  it("works each stretch's speed out from its own distance and moving time", async () => {
    render(<RideSplits splits={[split({ distanceMetres: 1000, movingSeconds: 120 })]} />);
    await table();

    expect(screen.getByText("30.0 km/h")).toBeInTheDocument();
    expect(screen.getByText("2 min")).toBeInTheDocument();
  });

  // A gap in the recording is not a slow kilometre.
  it("gives no speed to a stretch the odometer never advanced over", async () => {
    render(<RideSplits splits={[split(), split({ movingSeconds: 0 })]} />);
    await table();

    const second = within(rows()[1] as HTMLElement).getAllByRole("cell");
    expect(second[1]).toHaveTextContent("0 s");
    expect(second[2]).toHaveTextContent("—");
  });

  // A ride with no strap should not be ruled a column of dashes.
  it("leaves out a column no stretch of the ride carried", async () => {
    render(<RideSplits splits={[split({ heartRateBpm: 148 })]} />);
    await table();

    expect(screen.getByRole("columnheader", { name: "Heart rate" })).toBeInTheDocument();
    expect(screen.getByText("148 bpm")).toBeInTheDocument();
    expect(screen.queryByRole("columnheader", { name: "Power" })).not.toBeInTheDocument();
    expect(screen.queryByRole("columnheader", { name: "Ascent" })).not.toBeInTheDocument();
  });

  it("shows measured power where the bicycle carried a meter", async () => {
    render(<RideSplits splits={[split({ powerWatts: 212.6 }), split()]} />);
    await table();

    expect(screen.getByRole("columnheader", { name: "Power" })).toBeInTheDocument();
    expect(screen.getByText("213 W")).toBeInTheDocument();
    const second = within(rows()[1] as HTMLElement).getAllByRole("cell");
    expect(second[second.length - 1]).toHaveTextContent("—");
  });

  it("shows ascent once any stretch of the ride climbed", async () => {
    render(<RideSplits splits={[split({ ascentMetres: 42 }), split()]} />);
    await table();

    expect(screen.getByRole("columnheader", { name: "Ascent" })).toBeInTheDocument();
    expect(screen.getByText("42 m")).toBeInTheDocument();
  });

  it("draws one bar per stretch, each as tall as its speed against the fastest", () => {
    const { container } = render(
      <RideSplits splits={[split({ movingSeconds: 120 }), split({ movingSeconds: 240 })]} />,
    );

    const heights = Array.from(container.querySelectorAll("rect")).map((bar) =>
      Number.parseFloat(bar.getAttribute("height") ?? ""),
    );
    expect(heights).toEqual([100, 50]);
  });

  // Every stretch standing still leaves no fastest one to scale against, and
  // nought over nought is not a height.
  it("draws empty bars for a ride no stretch of which was ridden", () => {
    const { container } = render(
      <RideSplits splits={[split({ movingSeconds: 0 }), split({ movingSeconds: 0 })]} />,
    );

    const bars = container.querySelectorAll("rect");
    expect(bars).toHaveLength(2);
    for (const bar of bars) {
      expect(bar.getAttribute("height")).toBe("0");
    }
  });

  // Nought over nought is not a position: bars of no length draw as none.
  it("draws stretches of no length without dividing by them", () => {
    const { container } = render(
      <RideSplits splits={[split({ distanceMetres: 0 }), split({ distanceMetres: 0 })]} />,
    );

    for (const bar of container.querySelectorAll("rect")) {
      expect(Number.isFinite(Number(bar.getAttribute("x")))).toBe(true);
      expect(bar.getAttribute("width")).toBe("0");
    }
  });

  it("reads out the stretch the shared position is on", () => {
    const { rerender } = render(
      <RideSplits
        splits={[split(), split({ ascentMetres: 42, heartRateBpm: 148.4 })]}
        activeMetres={1500}
      />,
    );
    expect(screen.getByText("2.0 km · 20.0 km/h · 42 m · 148 bpm")).toBeInTheDocument();

    // A ride no stretch of which climbed says nothing about ascent, as the table does.
    rerender(<RideSplits splits={[split(), split()]} activeMetres={1500} />);
    expect(screen.getByText("2.0 km · 20.0 km/h")).toBeInTheDocument();

    rerender(<RideSplits splits={[split(), split()]} activeMetres={null} />);
    expect(screen.getByText(/Speed by the kilometre/)).toBeInTheDocument();
  });

  // The shared position is on the profile's axis, which the odometer's
  // kilometres add up to a little short of: a position two thirds along the
  // axis is on the second of three stretches, not the third.
  it("reads the shared position on the axis it was given", () => {
    render(
      <RideSplits splits={[split(), split(), split()]} activeMetres={2_100} axisMetres={3_300} />,
    );

    expect(screen.getByText(/^2\.0 km/)).toBeInTheDocument();
  });

  it("shows nothing at all for a ride with no splits", () => {
    const { rerender } = render(<RideSplits splits={[]} />);
    expect(screen.queryByLabelText("Splits")).not.toBeInTheDocument();

    rerender(<RideSplits splits={undefined} />);
    expect(screen.queryByLabelText("Splits")).not.toBeInTheDocument();
  });
});
