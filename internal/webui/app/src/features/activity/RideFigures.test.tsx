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
});
