import { render, screen } from "@testing-library/react";
import { describe, expect, it } from "vitest";
import type { RouteClimb, RouteClimbAttempt } from "../../api/types";
import { RideClimbs } from "./RideClimbs";

const RIDE_ID = "42";

function attempt(
  overrides: Partial<RouteClimbAttempt> & { activityId: string },
): RouteClimbAttempt {
  return {
    riddenAt: "2026-09-01T06:00:00Z",
    seconds: 420,
    vamMetresPerHour: 900,
    ...overrides,
  };
}

function climb(overrides: Partial<RouteClimb> = {}): RouteClimb {
  return {
    startMetres: 0,
    endMetres: 2000,
    distanceMetres: 2000,
    ascentMetres: 180,
    averageGradePercent: 6.2,
    maxGradePercent: 11.4,
    attempts: [attempt({ activityId: RIDE_ID })],
    ...overrides,
  };
}

describe("RideClimbs", () => {
  it("names a climb's distance, gradients, time, heart rate and measured power", () => {
    render(
      <RideClimbs
        climbs={[
          climb({
            attempts: [
              attempt({
                activityId: RIDE_ID,
                seconds: 365,
                heartRateBpm: 158.4,
                powerWatts: 268.2,
              }),
            ],
          }),
        ]}
        activityId={RIDE_ID}
      />,
    );

    expect(screen.getByText("Climb 1")).toBeInTheDocument();
    // formatGradient drops to no decimal at 10%, so the 11.4% max reads "11%".
    expect(screen.getByText("2.0 km · 6.2% avg · 11% max")).toBeInTheDocument();
    expect(screen.getByText("6:05")).toBeInTheDocument();
    expect(screen.getByText("158 bpm · 268 W")).toBeInTheDocument();
  });

  it("marks an estimate rather than a measured reading, and never shows both", () => {
    render(
      <RideClimbs
        climbs={[
          climb({ attempts: [attempt({ activityId: RIDE_ID, estimatedPowerWatts: 241.6 })] }),
        ]}
        activityId={RIDE_ID}
      />,
    );

    expect(screen.getByText(/~242 W/)).toBeInTheDocument();
  });

  it("shows the rider's rank and the best time here against an earlier attempt", () => {
    render(
      <RideClimbs
        climbs={[
          climb({
            attempts: [
              attempt({ activityId: "9", seconds: 340 }),
              attempt({ activityId: RIDE_ID, seconds: 365 }),
            ],
          }),
        ]}
        activityId={RIDE_ID}
      />,
    );

    expect(screen.getByText(/6:05 · #2 of 2/)).toBeInTheDocument();
    expect(screen.getByText("best here 5:40")).toBeInTheDocument();
  });

  it("leaves out the rank and best-here caption for a first-ever attempt", () => {
    render(
      <RideClimbs
        climbs={[climb({ attempts: [attempt({ activityId: RIDE_ID })] })]}
        activityId={RIDE_ID}
      />,
    );

    expect(screen.queryByText(/#1 of 1/)).not.toBeInTheDocument();
    expect(screen.queryByText(/best here/)).not.toBeInTheDocument();
  });

  it("leaves out the best-here caption when this attempt is already the best", () => {
    render(
      <RideClimbs
        climbs={[
          climb({
            attempts: [
              attempt({ activityId: RIDE_ID, seconds: 300 }),
              attempt({ activityId: "9", seconds: 340 }),
            ],
          }),
        ]}
        activityId={RIDE_ID}
      />,
    );

    expect(screen.getByText(/5:00 · #1 of 2/)).toBeInTheDocument();
    expect(screen.queryByText(/best here/)).not.toBeInTheDocument();
  });

  it("shows one row per climb the ride rode, skipping a climb it never attempted", () => {
    render(
      <RideClimbs
        climbs={[
          climb({ startMetres: 0, attempts: [attempt({ activityId: RIDE_ID })] }),
          climb({ startMetres: 5000, attempts: [attempt({ activityId: "9" })] }),
        ]}
        activityId={RIDE_ID}
      />,
    );

    expect(screen.getByText("Climb 1")).toBeInTheDocument();
    expect(screen.queryByText("Climb 2")).not.toBeInTheDocument();
  });

  // The ordinal is the climb's position in the route's own order — the same
  // one the route page's climb list carries — not its position after
  // filtering, which would relabel a later climb as "Climb 1".
  it("keeps a climb's route-order ordinal when an earlier climb was skipped", () => {
    render(
      <RideClimbs
        climbs={[
          climb({ startMetres: 0, attempts: [attempt({ activityId: "9" })] }),
          climb({ startMetres: 5000, attempts: [attempt({ activityId: RIDE_ID })] }),
        ]}
        activityId={RIDE_ID}
      />,
    );

    expect(screen.queryByText("Climb 1")).not.toBeInTheDocument();
    expect(screen.getByText("Climb 2")).toBeInTheDocument();
  });

  it("renders nothing for a matched route with no sustained climb", () => {
    const { container } = render(<RideClimbs climbs={[]} activityId={RIDE_ID} />);

    expect(container).toBeEmptyDOMElement();
  });

  it("renders nothing for an unmatched ride", () => {
    const { container } = render(<RideClimbs climbs={undefined} activityId={RIDE_ID} />);

    expect(container).toBeEmptyDOMElement();
  });

  it("renders nothing when the ride matched a route but rode none of its climbs", () => {
    const { container } = render(
      <RideClimbs
        climbs={[climb({ attempts: [attempt({ activityId: "9" })] })]}
        activityId={RIDE_ID}
      />,
    );

    expect(container).toBeEmptyDOMElement();
  });
});
