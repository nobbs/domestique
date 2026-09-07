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

  // The provider says where the wind came from; every arrow in this application
  // points the way the air is going, and the spoken label says the same thing.
  // A wind from the south-west is a wind toward the north-east.
  it("says which way the wind was going, for a reader who cannot see the arrow", () => {
    render(<RideConditions hours={[hour({ windDirectionDegrees: 240 })]} />);

    expect(screen.getByText("Wind 14 km/h toward the north-east")).toBeInTheDocument();
  });

  it("names what fell on a wet hour", () => {
    render(<RideConditions hours={[hour({ precipitationMillimetres: 2.4 })]} />);

    expect(screen.getByText("Wind 14 km/h toward the north-east, 2.4 mm")).toBeInTheDocument();
  });

  // The absence is the reading: a dry hour says nothing about rain.
  it("says nothing about rain on a dry hour", () => {
    render(<RideConditions hours={[hour()]} />);

    expect(screen.getByText("Wind 14 km/h toward the north-east")).toBeInTheDocument();
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
