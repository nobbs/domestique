import { render, screen } from "@testing-library/react";
import { describe, expect, it } from "vitest";
import type { Activity, ActivityMetrics } from "../../api/types";
import { RideFigures } from "./RideFigures";

function ride(metrics?: ActivityMetrics, totals?: Partial<Activity>): Activity {
  return {
    id: "1",
    startedAt: "2026-09-01T06:00:00Z",
    distanceMetres: 36000,
    movingSeconds: 3600,
    elapsedSeconds: 4000,
    ascentMetres: 420,
    typeId: 0,
    locationId: 0,
    provider: "wahoo",
    ...(metrics ? { metrics } : {}),
    ...totals,
  };
}

describe("RideFigures", () => {
  it("sets the four figures that decide the ride", () => {
    render(<RideFigures ride={ride({ powerTss: 91.4, intensityFactor: 0.74 })} />);

    expect(screen.getByText("Distance")).toBeInTheDocument();
    expect(screen.getByText("36.0 km")).toBeInTheDocument();
    expect(screen.getByText("Moving")).toBeInTheDocument();
    expect(screen.getByText("1 h")).toBeInTheDocument();
    expect(screen.getByText("1 h 6 min elapsed")).toBeInTheDocument();
    expect(screen.getByText("Climbed")).toBeInTheDocument();
    expect(screen.getByText("420 m")).toBeInTheDocument();
    expect(screen.getByText("Training stress")).toBeInTheDocument();
    expect(screen.getByText("91")).toBeInTheDocument();
    expect(screen.getByText("TSS")).toBeInTheDocument();
    expect(screen.getByText("0.74 of threshold")).toBeInTheDocument();
  });

  // Two near-identical figures say less than one.
  it("leaves out elapsed time for a ride that barely stopped", () => {
    render(<RideFigures ride={ride(undefined, { elapsedSeconds: 3630 })} />);

    expect(screen.queryByText(/elapsed/)).not.toBeInTheDocument();
  });

  // Most specific scale first: a meter's TSS, else the strap's, else Banister's.
  it("names the ride's load on the scale its sensors allowed", () => {
    const { rerender } = render(<RideFigures ride={ride({ heartRateTss: 73.2, trimp: 130 })} />);
    expect(screen.getByText("73")).toBeInTheDocument();
    expect(screen.getByText("hrTSS")).toBeInTheDocument();
    expect(screen.queryByText("TRIMP")).not.toBeInTheDocument();

    rerender(<RideFigures ride={ride({ trimp: 130.4 })} />);
    expect(screen.getByText("Training impulse")).toBeInTheDocument();
    expect(screen.getByText("130")).toBeInTheDocument();
    expect(screen.getByText("TRIMP")).toBeInTheDocument();
  });

  // The headline figure is the most prominent number on the page, and the one
  // most likely to be read on its own — it is owed the coverage share too.
  it("appends the coverage share to the headline load figure's note", () => {
    const { rerender } = render(
      <RideFigures ride={ride({ powerTss: 91.4, intensityFactor: 0.74, powerCoverage: 0.85 })} />,
    );
    expect(screen.getByText("0.74 of threshold · 85% sensor coverage")).toBeInTheDocument();

    rerender(<RideFigures ride={ride({ heartRateTss: 73.2, heartRateCoverage: 0.92 })} />);
    expect(screen.getByText("92% sensor coverage")).toBeInTheDocument();

    rerender(<RideFigures ride={ride({ trimp: 130.4, heartRateCoverage: 0.6 })} />);
    expect(screen.getByText("60% sensor coverage")).toBeInTheDocument();
  });

  it("leaves the headline figure's note unmarked at full coverage", () => {
    render(
      <RideFigures ride={ride({ powerTss: 91.4, intensityFactor: 0.74, powerCoverage: 1 })} />,
    );

    expect(screen.getByText("0.74 of threshold")).toBeInTheDocument();
    expect(screen.queryByText(/sensor coverage/)).not.toBeInTheDocument();
  });

  // A ride whose recorded file was never readable still has its totals.
  it("shows three figures for a ride with no derived metrics", () => {
    render(<RideFigures ride={ride(undefined)} />);

    expect(screen.getByText("36.0 km")).toBeInTheDocument();
    expect(screen.queryByText(/Training/)).not.toBeInTheDocument();
  });

  it("shows nothing at all for a ride the page does not hold", () => {
    render(<RideFigures ride={undefined} />);

    expect(screen.queryByLabelText("Ride figures")).not.toBeInTheDocument();
  });

  it("shows the ride's descent and calories when the file gave them", () => {
    render(<RideFigures ride={ride(undefined, { descentMetres: 380, caloriesKcal: 1420 })} />);

    expect(screen.getByText("Descended")).toBeInTheDocument();
    expect(screen.getByText("380 m")).toBeInTheDocument();
    expect(screen.getByText("Calories")).toBeInTheDocument();
    expect(screen.getByText("1420")).toBeInTheDocument();
    expect(screen.getByText("kcal")).toBeInTheDocument();
  });

  it("leaves out descent and calories for a ride whose file did not give them", () => {
    render(<RideFigures ride={ride(undefined)} />);

    expect(screen.queryByText("Descended")).not.toBeInTheDocument();
    expect(screen.queryByText("Calories")).not.toBeInTheDocument();
  });

  it("notes the power-based estimate beside the file's own calories", () => {
    render(<RideFigures ride={ride({ estimatedCaloriesKcal: 1381.2 }, { caloriesKcal: 1420 })} />);

    expect(screen.getByText("Calories")).toBeInTheDocument();
    expect(screen.getByText("1420")).toBeInTheDocument();
    expect(screen.getByText("~1381 kcal estimated")).toBeInTheDocument();
  });

  it("shows the estimate alone for a ride whose file gave no calories", () => {
    render(<RideFigures ride={ride({ estimatedCaloriesKcal: 1381.2 })} />);

    expect(screen.getByText("Calories (est.)")).toBeInTheDocument();
    expect(screen.getByText("1381")).toBeInTheDocument();
  });

  it("badges a Zwift ride and no other", () => {
    render(<RideFigures ride={ride(undefined, { provider: "zwift" })} />);
    expect(screen.getByText("Zwift")).toBeInTheDocument();
  });

  it("shows no provider badge for a Wahoo ride", () => {
    render(<RideFigures ride={ride(undefined)} />);
    expect(screen.queryByText("Zwift")).not.toBeInTheDocument();
  });

  it("names the ride's structured workout and its completion", () => {
    render(
      <RideFigures
        ride={ride(undefined, {
          provider: "zwift",
          workoutName: "Sweet Spot Progression",
          workoutHash: 998877,
          workoutCompletion: 0.8,
        })}
      />,
    );

    expect(screen.getByText("Sweet Spot Progression, 80 % completed")).toBeInTheDocument();
  });

  it("names a completed ride without a completion figure", () => {
    render(
      <RideFigures
        ride={ride(undefined, {
          provider: "zwift",
          workoutName: "Watopia Waistband",
          workoutHash: 1,
          workoutCompletion: 1,
        })}
      />,
    );

    expect(screen.getByText("Watopia Waistband")).toBeInTheDocument();
    expect(screen.queryByText(/completed/)).not.toBeInTheDocument();
  });

  it("shows no name line for a Zwift ride without one", () => {
    render(<RideFigures ride={ride(undefined, { provider: "zwift" })} />);
    expect(screen.queryByText(/completed/)).not.toBeInTheDocument();
  });
});
