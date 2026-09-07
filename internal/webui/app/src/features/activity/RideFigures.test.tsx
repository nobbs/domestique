import { render, screen } from "@testing-library/react";
import { describe, expect, it } from "vitest";
import type { Activity, ActivityMetrics } from "../../api/types";
import { RideFigures } from "./RideFigures";

function ride(metrics?: ActivityMetrics, totals?: Partial<Activity>): Activity {
  return {
    id: 1,
    startedAt: "2026-09-01T06:00:00Z",
    distanceMetres: 36000,
    movingSeconds: 3600,
    elapsedSeconds: 4000,
    ascentMetres: 420,
    typeId: 0,
    locationId: 0,
    ...(metrics ? { metrics } : {}),
    ...totals,
  };
}

describe("RideFigures", () => {
  it("shows what each sensor averaged, and the ride's peak heart rate", () => {
    render(
      <RideFigures
        ride={ride({
          averageHeartRateBpm: 142.4,
          maxHeartRateBpm: 178,
          averageCadenceRpm: 81.6,
          averagePowerWatts: 196.2,
        })}
      />,
    );

    expect(screen.getByText("Heart rate")).toBeInTheDocument();
    expect(screen.getByText("142")).toBeInTheDocument();
    expect(screen.getByText("Max heart rate")).toBeInTheDocument();
    expect(screen.getByText("178")).toBeInTheDocument();
    expect(screen.getByText("Cadence")).toBeInTheDocument();
    expect(screen.getByText("82")).toBeInTheDocument();
    expect(screen.getByText("Power")).toBeInTheDocument();
    expect(screen.getByText("196")).toBeInTheDocument();
  });

  // Distance over moving time, so it is there for a ride whose recorded file
  // was never readable and which therefore has no derived metrics at all.
  it("works the average speed out from the ride's own totals", () => {
    render(<RideFigures ride={ride(undefined)} />);

    expect(screen.getByText("Speed")).toBeInTheDocument();
    expect(screen.getByText("36.0")).toBeInTheDocument();
    expect(screen.queryByText("Heart rate")).not.toBeInTheDocument();
  });

  // A ride with no strap carries no heart rate rather than a heart rate of nought.
  it("leaves out a figure the ride carried no sensor for", () => {
    render(<RideFigures ride={ride({ averageCadenceRpm: 81.6 })} />);

    expect(screen.getByText("Cadence")).toBeInTheDocument();
    expect(screen.queryByText("Heart rate")).not.toBeInTheDocument();
    expect(screen.queryByText("Power")).not.toBeInTheDocument();
  });

  // The service serves an estimate only where it served no measurement, so the
  // two never sit side by side — but the estimate must say what it is either way.
  it("names estimated power as an estimate rather than a reading", () => {
    render(<RideFigures ride={ride({ estimatedPowerWatts: 187.4 })} />);

    expect(screen.getByText("Estimated power")).toBeInTheDocument();
    expect(screen.getByText("187")).toBeInTheDocument();
    expect(screen.getByText("watts, from the track")).toBeInTheDocument();
    expect(screen.queryByText("Power")).not.toBeInTheDocument();
  });

  it("shows no speed for a ride that has not moved", () => {
    render(<RideFigures ride={ride(undefined, { movingSeconds: 0 })} />);

    expect(screen.queryByLabelText("Ride averages")).not.toBeInTheDocument();
  });

  it("shows nothing at all for a ride the page does not hold", () => {
    render(<RideFigures ride={undefined} />);

    expect(screen.queryByLabelText("Ride averages")).not.toBeInTheDocument();
  });
});
