import { render, screen } from "@testing-library/react";
import { describe, expect, it } from "vitest";
import type { ActivityMetrics } from "../../api/types";
import { TrainingLoad } from "./TrainingLoad";

function show(metrics: ActivityMetrics | undefined) {
  render(<TrainingLoad metrics={metrics} />);
}

describe("TrainingLoad", () => {
  it("shows both load scales side by side, each named", () => {
    show({ trimp: 42.4, heartRateTss: 88.6, powerTss: 73.2, normalizedPowerWatts: 214 });

    expect(screen.getByText("TRIMP")).toBeInTheDocument();
    expect(screen.getByText("42")).toBeInTheDocument();
    expect(screen.getByText("Banister")).toBeInTheDocument();
    expect(screen.getByText("hrTSS")).toBeInTheDocument();
    expect(screen.getByText("89")).toBeInTheDocument();
    expect(screen.getByText("heart rate")).toBeInTheDocument();
  });

  it("names each zone and how long the ride held it", () => {
    show({ zoneSeconds: [60, 120, 180, 240, 300] });

    expect(screen.getByText("Recovery")).toBeInTheDocument();
    expect(screen.getByText("VO₂ max")).toBeInTheDocument();
    expect(screen.getByText("1 min")).toBeInTheDocument();
    expect(screen.getByText("5 min")).toBeInTheDocument();
  });

  // A ride carries the sensors it carries: a figure the profile or the ride did
  // not allow is left out rather than shown as a zero the reader would believe.
  it("leaves out a figure that was not worked out", () => {
    show({ trimp: 30 });

    expect(screen.getByText("TRIMP")).toBeInTheDocument();
    expect(screen.queryByText("TSS")).not.toBeInTheDocument();
    expect(screen.queryByText("Normalized power")).not.toBeInTheDocument();
    expect(screen.queryByText("Recovery")).not.toBeInTheDocument();
  });

  // Measured, not predicted: a zone held for forty seconds says forty seconds
  // rather than being rounded up to the nearest five minutes.
  it("says a zone's time exactly as long as it was", () => {
    show({ zoneSeconds: [40, 0, 0, 5400, 3600] });

    expect(screen.getByText("40 s")).toBeInTheDocument();
    expect(screen.getByText("1 h 30 min")).toBeInTheDocument();
    expect(screen.getByText("1 h")).toBeInTheDocument();
  });

  it("shows nothing at all for a ride with no derived metrics", () => {
    show(undefined);

    expect(screen.queryByLabelText("Training load")).not.toBeInTheDocument();
  });

  it("shows nothing for a row whose zones are all empty", () => {
    show({ zoneSeconds: [0, 0, 0, 0, 0] });

    expect(screen.queryByText("Recovery")).not.toBeInTheDocument();
  });
});
