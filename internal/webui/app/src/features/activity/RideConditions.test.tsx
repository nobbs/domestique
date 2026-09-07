import { render, screen } from "@testing-library/react";
import { describe, expect, it } from "vitest";
import type { ActivitySplit, RideWeatherStep } from "../../api/types";
import { MIN_WIDTH, PADDING } from "../../lib/plotAxis";
import { RideConditions, stepStarts } from "./RideConditions";

function step(overrides: Partial<RideWeatherStep> = {}): RideWeatherStep {
  return {
    time: "2026-08-24T06:00:00Z",
    stepSeconds: 3600,
    temperatureCelsius: 12.4,
    apparentTemperatureCelsius: 10,
    precipitationMillimetres: 0,
    windSpeedKmh: 14,
    windDirectionDegrees: 240,
    weatherCode: 3,
    cloudCoverPercent: 55,
    ...overrides,
  };
}

const TWO_STEPS = [step(), step({ time: "2026-08-24T07:00:00Z", temperatureCelsius: 18.6 })];

/** The strip as the page draws it: two hours over twenty kilometres, the second from the tenth. */
function show(steps: RideWeatherStep[] | undefined = TWO_STEPS, starts = [0, 10_000]) {
  return render(<RideConditions steps={steps} starts={starts} totalMetres={20_000} />);
}

/** The plot's width in a DOM that measures nothing, which the strip draws at instead. */
const PLOT_WIDTH = MIN_WIDTH - PADDING.left - PADDING.right;

describe("RideConditions", () => {
  it("draws one tile per step of the ride", () => {
    show();

    expect(screen.getAllByRole("listitem")).toHaveLength(2);
    expect(screen.getByText("12°")).toBeInTheDocument();
    expect(screen.getByText("19°")).toBeInTheDocument();
  });

  // A tile is as wide as the ground the ride covered in its hour, laid on the
  // same axis the profile above it draws distance on.
  it("lands each tile under the kilometres it describes", () => {
    show(TWO_STEPS, [0, 15_000]);

    const [first, second] = screen.getAllByRole("listitem") as HTMLElement[];
    expect(first?.style.left).toBe("0px");
    expect(Number.parseFloat(first?.style.width ?? "")).toBeCloseTo(PLOT_WIDTH * 0.75, 5);
    expect(Number.parseFloat(second?.style.left ?? "")).toBeCloseTo(PLOT_WIDTH * 0.75, 5);
    expect(Number.parseFloat(second?.style.width ?? "")).toBeCloseTo(PLOT_WIDTH * 0.25, 5);
  });

  // The provider answers whole hours; an hour that began after the ride ended
  // has no ground to stand on.
  it("leaves out a step that began beyond the end of the ride", () => {
    show(TWO_STEPS, [0, 25_000]);

    expect(screen.getAllByRole("listitem")).toHaveLength(1);
  });

  // The provider says where the wind came from; every arrow in this application
  // points the way the air is going, and the spoken label says the same thing.
  // A wind from the south-west is a wind toward the north-east.
  it("says which way the wind was going, for a reader who cannot see the arrow", () => {
    show([step({ windDirectionDegrees: 240 })], [0]);

    expect(screen.getByText("Wind 14 km/h toward the north-east")).toBeInTheDocument();
  });

  it("names what fell on a wet step", () => {
    show([step({ precipitationMillimetres: 2.4 })], [0]);

    expect(screen.getByText("Wind 14 km/h toward the north-east, 2.4 mm")).toBeInTheDocument();
  });

  // The absence is the reading: a dry step says nothing about rain.
  it("says nothing about rain on a dry step", () => {
    show([step()], [0]);

    expect(screen.getByText("Wind 14 km/h toward the north-east")).toBeInTheDocument();
  });

  // A ride nobody has asked about carries no strip at all, rather than an empty
  // one claiming the weather is unknown.
  it("draws nothing for a ride with no recorded weather", () => {
    const { container, rerender } = render(
      <RideConditions steps={undefined} starts={[]} totalMetres={20_000} />,
    );
    expect(container).toBeEmptyDOMElement();

    rerender(<RideConditions steps={[]} starts={[]} totalMetres={20_000} />);
    expect(container).toBeEmptyDOMElement();
  });

  // A recent ride is answered by the quarter hour, and the clock already shows
  // minutes: nothing about the tile needs to change to draw one correctly.
  it("draws a quarter-hour step at its own minute, not rounded to the hour", () => {
    // Through the platform's own formatter, so the assertion carries no locale
    // or zone of its own and still fails if the minute is dropped.
    const clock = (iso: string) =>
      new Date(iso).toLocaleTimeString(undefined, { hour: "2-digit", minute: "2-digit" });
    show([step({ time: "2026-08-24T06:15:00Z", stepSeconds: 900 })], [0]);

    expect(screen.getAllByRole("listitem")).toHaveLength(1);
    expect(screen.getByText(new RegExp(clock("2026-08-24T06:15:00Z")))).toBeInTheDocument();
    expect(screen.queryByText(new RegExp(clock("2026-08-24T06:00:00Z")))).toBeNull();
  });
});

describe("stepStarts", () => {
  const split = (movingSeconds: number): ActivitySplit => ({
    distanceMetres: 1000,
    movingSeconds,
    ascentMetres: 0,
  });
  const started = "2026-08-24T06:00:00Z";

  // Walked along the splits' own clock: the second hour arrives during the
  // third kilometre of a ride that took half an hour a kilometre.
  it("places each step where the splits' clock had got to", () => {
    const starts = stepStarts(TWO_STEPS, started, 9_000, 4_000, [
      split(1800),
      split(1800),
      split(1800),
      split(1800),
    ]);

    expect(starts).toEqual([0, 2_000]);
  });

  it("places a step the ride outlasted at its end", () => {
    const late = step({ time: "2026-08-24T09:00:00Z" });

    expect(stepStarts([late], started, 3_600, 2_000, [split(1800), split(1800)])).toEqual([2_000]);
  });

  // The provider's hour can begin before the ride did.
  it("places a step that began before the ride at the start", () => {
    const early = step({ time: "2026-08-24T05:30:00Z" });

    expect(stepStarts([early], started, 3_600, 2_000, [split(1800), split(1800)])).toEqual([0]);
  });

  // Without splits there is only elapsed time to spread the steps by.
  it("spreads the steps by elapsed time when there are no splits", () => {
    expect(stepStarts(TWO_STEPS, started, 7_200, 20_000, [])).toEqual([0, 10_000]);
    expect(stepStarts(TWO_STEPS, started, 0, 20_000, [])).toEqual([0, 0]);
  });
});
