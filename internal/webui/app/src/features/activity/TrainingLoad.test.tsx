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
    expect(screen.getAllByRole("listitem")[0]).toHaveTextContent("Recovery1 min");
  });

  it("draws one bar per zone, all on the scale the longest zone sets", () => {
    show({ zoneSeconds: [60, 120, 0, 240, 120] });

    const bars = screen
      .getAllByRole("listitem")
      .map((row) => row.querySelector<HTMLElement>("span[style]"));

    expect(bars).toHaveLength(5);
    expect(bars.map((bar) => bar?.style.width)).toEqual(["25%", "50%", "0%", "100%", "50%"]);
  });

  // Open at both ends: neither the easiest nor the hardest zone is given a
  // limit the profile never said.
  it("says the heart rates each zone covered", () => {
    show({ zoneSeconds: [60, 120, 180, 240, 300], zoneBoundsBpm: [144.5, 153, 161.5, 170] });

    expect(screen.getByText("below 145 bpm")).toBeInTheDocument();
    expect(screen.getByText("145–152 bpm")).toBeInTheDocument();
    expect(screen.getByText("170 bpm and up")).toBeInTheDocument();
  });

  // A bound of 144.2 puts 144 bpm in the easiest zone and 145 in the next, so
  // the edge is the beat above it rather than the nearer one.
  it("keeps a bound between two beats on the side the zones were cut", () => {
    show({ zoneSeconds: [60, 120, 180, 240, 300], zoneBoundsBpm: [144.2, 153.6, 161.5, 170.9] });

    expect(screen.getByText("below 145 bpm")).toBeInTheDocument();
    expect(screen.getByText("145–153 bpm")).toBeInTheDocument();
    expect(screen.getByText("171 bpm and up")).toBeInTheDocument();
  });

  it("leaves the rates out for a row that was derived without them", () => {
    show({ zoneSeconds: [60, 120, 180, 240, 300] });

    expect(screen.getByText("Recovery")).toBeInTheDocument();
    expect(screen.queryByText(/bpm/)).not.toBeInTheDocument();
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

  // Measured, not predicted, and floored at every step: a zone held for a
  // minute and a half says so rather than being rounded up to two minutes.
  it("says a zone's time exactly as long as it was", () => {
    show({ zoneSeconds: [40, 90, 0, 5400, 3600] });

    expect(screen.getByText("40 s")).toBeInTheDocument();
    expect(screen.getByText("1 min 30 s")).toBeInTheDocument();
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
