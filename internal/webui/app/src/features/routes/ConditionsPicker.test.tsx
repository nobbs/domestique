/**
 * The one choice the wash is made by, and the key that says what it means.
 *
 * Nothing here touches a map: the picker reports a measure and the legend
 * reads the registry, so both can be asked about as ordinary DOM.
 */

import { render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { describe, expect, it, vi } from "vitest";
import type { ForecastSample } from "../../lib/forecastSamples";
import { MEASURES } from "../../lib/measures";
import { ConditionsPicker } from "./ConditionsPicker";

const SAMPLES: ForecastSample[] = [0, 1_500, 3_000].map((distanceMetres, index) => ({
  position: [8 + index * 0.01, 49],
  distanceMetres,
  arrivalAt: new Date(Date.now() + (1 + index) * 3_600_000),
}));

function show(
  options: {
    measure?: Parameters<typeof ConditionsPicker>[0]["measure"];
    samples?: ForecastSample[];
    movingSeconds?: number | undefined;
  } = {},
) {
  const onMeasureChange = vi.fn();
  render(
    <ConditionsPicker
      measure={options.measure ?? null}
      onMeasureChange={onMeasureChange}
      samples={options.samples ?? SAMPLES}
      movingSeconds={"movingSeconds" in options ? options.movingSeconds : 6_000}
    />,
  );

  return { onMeasureChange };
}

describe("choosing what the map is washed in", () => {
  it("offers every measure the registry holds, and off", () => {
    show();

    for (const measure of MEASURES) {
      expect(screen.getByRole("button", { name: measure.label })).toBeInTheDocument();
    }
    expect(screen.getByRole("button", { name: "Off" })).toBeInTheDocument();
  });

  it("starts off, because an overlay nobody asked for is an overlay in the way", () => {
    show();

    expect(screen.getByRole("button", { name: "Off" })).toHaveAttribute("aria-pressed", "true");
  });

  it("marks exactly one choice as pressed", () => {
    show({ measure: "rain" });

    expect(screen.getByRole("button", { name: "Rain" })).toHaveAttribute("aria-pressed", "true");
    for (const name of ["Off", "Wind", "Temperature", "Cloud"]) {
      expect(screen.getByRole("button", { name })).toHaveAttribute("aria-pressed", "false");
    }
  });

  it("swaps one measure for another rather than adding to it", async () => {
    const view = show({ measure: "rain" });
    await userEvent.click(screen.getByRole("button", { name: "Wind" }));

    expect(view.onMeasureChange).toHaveBeenCalledWith("wind");
  });

  it("turns off through the off choice", async () => {
    const view = show({ measure: "rain" });
    await userEvent.click(screen.getByRole("button", { name: "Off" }));

    expect(view.onMeasureChange).toHaveBeenCalledWith(null);
  });

  it("turns off by pressing the measure already showing", async () => {
    const view = show({ measure: "rain" });
    await userEvent.click(screen.getByRole("button", { name: "Rain" }));

    expect(view.onMeasureChange).toHaveBeenCalledWith(null);
  });
});

describe("the key beside the choice", () => {
  it("names every band of the measure on show, not only its colour", () => {
    show({ measure: "cloud" });
    const cloud = MEASURES.find((measure) => measure.key === "cloud");

    for (const band of cloud?.bands ?? []) {
      expect(screen.getByText(new RegExp(band.description))).toBeInTheDocument();
    }
  });

  it("swatches from the page's own theme, which is where a panel's colours come from", () => {
    show({ measure: "rain" });
    const swatch = document.querySelector<HTMLElement>("li span[aria-hidden]");

    expect(swatch?.style.backgroundColor).toBe("var(--rain-0)");
  });

  it("says plainly which band the map paints nothing for", () => {
    show({ measure: "rain" });

    expect(screen.getByText(/essentially dry \(not washed\)/)).toBeInTheDocument();
  });

  it("has no key at all while the wash is off", () => {
    show();

    expect(screen.queryByRole("list")).not.toBeInTheDocument();
  });
});

describe("a route with no forecast to wash it in", () => {
  it("leaves every choice inert rather than offering one that draws nothing", () => {
    show({ samples: [] });

    for (const measure of MEASURES) {
      expect(screen.getByRole("button", { name: measure.label })).toBeDisabled();
    }
  });

  it("asks for a start time when that is the only thing missing", () => {
    show({ samples: [] });

    expect(screen.getByText(/Pick a ride start/)).toBeInTheDocument();
  });

  it("says so instead when nothing has predicted a moving time", () => {
    show({ samples: [], movingSeconds: undefined });

    expect(screen.getByText(/no forecast to wash it in/)).toBeInTheDocument();
  });
});
