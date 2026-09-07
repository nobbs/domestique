import { render, screen } from "@testing-library/react";
import { describe, expect, it } from "vitest";
import type { RideWeatherHour } from "../../api/types";
import { RideConditions } from "./RideConditions";

function hour(overrides: Partial<RideWeatherHour> = {}): RideWeatherHour {
  return {
    time: "2026-08-24T06:00:00Z",
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
  it("draws one tile per hour of the ride", () => {
    render(
      <RideConditions
        hours={[hour(), hour({ time: "2026-08-24T07:00:00Z", temperatureCelsius: 18.6 })]}
      />,
    );

    expect(screen.getAllByRole("listitem")).toHaveLength(2);
    expect(screen.getByText("12°")).toBeInTheDocument();
    expect(screen.getByText("19°")).toBeInTheDocument();
  });

  it("says which way the wind was going, for a reader who cannot see the arrow", () => {
    render(<RideConditions hours={[hour({ windDirectionDegrees: 240 })]} />);

    // Words rather than an abbreviation: this is read aloud, not navigated by.
    expect(screen.getByText("south-west wind")).toBeInTheDocument();
  });

  it("names what fell, and says nothing about rain on a dry hour", () => {
    render(<RideConditions hours={[hour({ precipitationMillimetres: 2.4 })]} />);
    expect(screen.getByText("south-west wind, 2.4 mm")).toBeInTheDocument();

    render(<RideConditions hours={[hour()]} />);
    expect(screen.getByText("south-west wind")).toBeInTheDocument();
  });

  // A ride nobody has asked about carries no strip at all, rather than an empty
  // one claiming the weather is unknown.
  it("draws nothing for a ride with no recorded weather", () => {
    const { container } = render(<RideConditions hours={undefined} />);
    expect(container).toBeEmptyDOMElement();

    const empty = render(<RideConditions hours={[]} />);
    expect(empty.container).toBeEmptyDOMElement();
  });
});
