import { render, screen } from "@testing-library/react";
import { describe, expect, it } from "vitest";
import type { RideWeatherStep } from "../../api/types";
import { RideConditions } from "./RideConditions";

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

describe("RideConditions", () => {
  it("draws one tile per step of the ride", () => {
    render(
      <RideConditions
        steps={[step(), step({ time: "2026-08-24T07:00:00Z", temperatureCelsius: 18.6 })]}
      />,
    );

    expect(screen.getAllByRole("listitem")).toHaveLength(2);
    expect(screen.getByText("12°")).toBeInTheDocument();
    expect(screen.getByText("19°")).toBeInTheDocument();
  });

  // The provider says where the wind came from; every arrow in this application
  // points the way the air is going, and the spoken label says the same thing.
  // A wind from the south-west is a wind toward the north-east.
  it("says which way the wind was going, for a reader who cannot see the arrow", () => {
    render(<RideConditions steps={[step({ windDirectionDegrees: 240 })]} />);

    expect(screen.getByText("Wind 14 km/h toward the north-east")).toBeInTheDocument();
  });

  it("names what fell on a wet step", () => {
    render(<RideConditions steps={[step({ precipitationMillimetres: 2.4 })]} />);

    expect(screen.getByText("Wind 14 km/h toward the north-east, 2.4 mm")).toBeInTheDocument();
  });

  // The absence is the reading: a dry step says nothing about rain.
  it("says nothing about rain on a dry step", () => {
    render(<RideConditions steps={[step()]} />);

    expect(screen.getByText("Wind 14 km/h toward the north-east")).toBeInTheDocument();
  });

  // A ride nobody has asked about carries no strip at all, rather than an empty
  // one claiming the weather is unknown.
  it("draws nothing for a ride with no recorded weather", () => {
    const { container } = render(<RideConditions steps={undefined} />);
    expect(container).toBeEmptyDOMElement();

    const empty = render(<RideConditions steps={[]} />);
    expect(empty.container).toBeEmptyDOMElement();
  });

  // A recent ride is answered by the quarter hour, and the clock already shows
  // minutes: nothing about the tile needs to change to draw one correctly.
  it("draws a quarter-hour step at its own minute, not rounded to the hour", () => {
    // Through the platform's own formatter, so the assertion carries no locale
    // or zone of its own and still fails if the minute is dropped.
    const clock = (iso: string) =>
      new Date(iso).toLocaleTimeString(undefined, { hour: "2-digit", minute: "2-digit" });
    render(<RideConditions steps={[step({ time: "2026-08-24T06:15:00Z", stepSeconds: 900 })]} />);

    expect(screen.getAllByRole("listitem")).toHaveLength(1);
    expect(screen.getByText(clock("2026-08-24T06:15:00Z"))).toBeInTheDocument();
    expect(screen.queryByText(clock("2026-08-24T06:00:00Z"))).toBeNull();
  });
});
