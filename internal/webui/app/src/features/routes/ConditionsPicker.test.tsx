/**
 * The one choice the wash is made by, and the key that says what it means.
 *
 * Nothing here touches a map: the choices report a measure and the key reads
 * the registry, so both can be asked about as ordinary DOM.
 */

import { render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { describe, expect, it, vi } from "vitest";
import type { ForecastSample } from "../../lib/forecastSamples";
import { bandRange, MEASURES, WIND_RELATION_KEY } from "../../lib/measures";
import { ConditionsChoices, ConditionsKey } from "./ConditionsPicker";

const SAMPLES: ForecastSample[] = [0, 1_500, 3_000].map((distanceMetres, index) => ({
  position: [8 + index * 0.01, 49],
  distanceMetres,
  arrivalAt: new Date(Date.now() + (1 + index) * 3_600_000),
}));

function showChoices(
  options: {
    measure?: Parameters<typeof ConditionsChoices>[0]["measure"];
    samples?: ForecastSample[];
    movingSeconds?: number | undefined;
  } = {},
) {
  const onMeasureChange = vi.fn();
  render(
    <ConditionsChoices
      measure={options.measure ?? null}
      onMeasureChange={onMeasureChange}
      samples={options.samples ?? SAMPLES}
      movingSeconds={"movingSeconds" in options ? options.movingSeconds : 6_000}
    />,
  );

  return { onMeasureChange };
}

function showKey(
  options: {
    measure?: Parameters<typeof ConditionsKey>[0]["measure"];
    samples?: ForecastSample[];
  } = {},
) {
  render(<ConditionsKey measure={options.measure ?? null} samples={options.samples ?? SAMPLES} />);
}

describe("choosing what the map is washed in", () => {
  it("offers every measure the registry holds, and off", () => {
    showChoices();

    for (const measure of MEASURES) {
      expect(screen.getByRole("button", { name: measure.label })).toBeInTheDocument();
    }
    expect(screen.getByRole("button", { name: "Off" })).toBeInTheDocument();
  });

  it("carries an icon on every choice", () => {
    showChoices();

    for (const name of ["Off", ...MEASURES.map((measure) => measure.label)]) {
      const button = screen.getByRole("button", { name });
      expect(button.querySelector("svg")).toBeInTheDocument();
    }
  });

  it("starts off, because an overlay nobody asked for is an overlay in the way", () => {
    showChoices();

    expect(screen.getByRole("button", { name: "Off" })).toHaveAttribute("aria-pressed", "true");
  });

  it("marks exactly one choice as pressed", () => {
    showChoices({ measure: "rain" });

    expect(screen.getByRole("button", { name: "Rain" })).toHaveAttribute("aria-pressed", "true");
    for (const name of ["Off", "Wind", "Temperature", "Cloud"]) {
      expect(screen.getByRole("button", { name })).toHaveAttribute("aria-pressed", "false");
    }
  });

  it("swaps one measure for another rather than adding to it", async () => {
    const view = showChoices({ measure: "rain" });
    await userEvent.click(screen.getByRole("button", { name: "Wind" }));

    expect(view.onMeasureChange).toHaveBeenCalledWith("wind");
  });

  it("turns off through the off choice", async () => {
    const view = showChoices({ measure: "rain" });
    await userEvent.click(screen.getByRole("button", { name: "Off" }));

    expect(view.onMeasureChange).toHaveBeenCalledWith(null);
  });

  it("turns off by pressing the measure already showing", async () => {
    const view = showChoices({ measure: "rain" });
    await userEvent.click(screen.getByRole("button", { name: "Rain" }));

    expect(view.onMeasureChange).toHaveBeenCalledWith(null);
  });
});

describe("a route with no forecast to wash it in", () => {
  it("leaves every choice inert rather than offering one that draws nothing", () => {
    showChoices({ samples: [] });

    for (const measure of MEASURES) {
      expect(screen.getByRole("button", { name: measure.label })).toBeDisabled();
    }
  });

  it("asks for a start time when that is the only thing missing", () => {
    showChoices({ samples: [] });

    expect(screen.getByText(/Pick a ride start/)).toBeInTheDocument();
  });

  it("says so instead when nothing has predicted a moving time", () => {
    showChoices({ samples: [], movingSeconds: undefined });

    expect(screen.getByText(/no forecast to wash it in/)).toBeInTheDocument();
  });
});

describe("the key beside the choice", () => {
  it("renders nothing while the wash is off", () => {
    const { container } = render(<ConditionsKey measure={null} samples={SAMPLES} />);

    expect(container).toBeEmptyDOMElement();
  });

  it("renders nothing without samples, even with a measure chosen", () => {
    const { container } = render(<ConditionsKey measure="rain" samples={[]} />);

    expect(container).toBeEmptyDOMElement();
  });

  it("sits on one line", () => {
    const { container } = render(<ConditionsKey measure="cloud" samples={SAMPLES} />);

    expect(container.firstElementChild?.className).toContain("flex-wrap");
  });

  it("names every band of the measure on show, not only its colour", () => {
    showKey({ measure: "cloud" });
    const cloud = MEASURES.find((measure) => measure.key === "cloud");

    for (const band of cloud?.bands ?? []) {
      expect(screen.getByText(band.label)).toBeInTheDocument();
    }
  });

  it("puts the description and the range under the pointer", async () => {
    const user = userEvent.setup();
    showKey({ measure: "cloud" });
    const cloud = MEASURES.find((measure) => measure.key === "cloud");
    const firstBand = cloud?.bands[0];
    if (!firstBand) {
      throw new Error("cloud has no bands");
    }

    await user.hover(screen.getByText(firstBand.label));
    const tooltip = await screen.findByRole("tooltip");

    expect(tooltip).toHaveTextContent(firstBand.description);
    expect(tooltip).toHaveTextContent(bandRange(cloud, 0));
  });

  it("says plainly which band the map paints nothing for", async () => {
    const user = userEvent.setup();
    showKey({ measure: "rain" });

    await user.hover(screen.getByText("dry"));
    const tooltip = await screen.findByRole("tooltip");

    expect(tooltip).toHaveTextContent("not washed");
  });

  it("carries both groups for wind: the corridor and the route line", () => {
    showKey({ measure: "wind" });

    expect(screen.getByText("Corridor")).toBeInTheDocument();
    expect(screen.getByText("Route line")).toBeInTheDocument();
    for (const label of new Set(WIND_RELATION_KEY.map((band) => band.label))) {
      expect(screen.getAllByText(label).length).toBeGreaterThan(0);
    }
  });

  it("says a route-line entry replaces the steepness edging, under the pointer", async () => {
    const user = userEvent.setup();
    showKey({ measure: "wind" });
    const headwind = WIND_RELATION_KEY[0];
    if (!headwind) {
      throw new Error("no headwind entry");
    }

    await user.hover(screen.getByText(headwind.label));
    const tooltip = await screen.findByRole("tooltip");

    expect(tooltip).toHaveTextContent("replaces the steepness edging");
  });

  it("carries no route-line group for a measure that is not wind", () => {
    showKey({ measure: "rain" });

    expect(screen.queryByText("Route line")).not.toBeInTheDocument();
  });
});
