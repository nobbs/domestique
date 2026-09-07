import { render, screen } from "@testing-library/react";
import { describe, expect, it } from "vitest";
import type { Activity, ActivityMetrics } from "../../api/types";
import { TrainingLoad } from "./TrainingLoad";

function ride(metrics: ActivityMetrics | undefined, totals?: Partial<Activity>): Activity {
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

function show(metrics: ActivityMetrics | undefined, totals?: Partial<Activity>) {
  return render(<TrainingLoad ride={ride(metrics, totals)} />);
}

/** The zone bar's five segments, in zone order. */
function segments(container: HTMLElement): HTMLElement[] {
  const bar = container.querySelector<HTMLElement>('div[aria-hidden="true"]');

  return bar ? Array.from(bar.children as HTMLCollectionOf<HTMLElement>) : [];
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

  it("shows what each sensor averaged, and the ride's peak heart rate", () => {
    show({
      averageHeartRateBpm: 142.4,
      maxHeartRateBpm: 178,
      averageCadenceRpm: 81.6,
      averagePowerWatts: 196.2,
    });

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
    show(undefined);

    expect(screen.getByText("Speed")).toBeInTheDocument();
    expect(screen.getByText("36.0")).toBeInTheDocument();
    expect(screen.queryByText("Heart rate")).not.toBeInTheDocument();
  });

  // The service serves an estimate only where it served no measurement, so the
  // two never sit side by side — but the estimate must say what it is either way.
  it("names estimated power as an estimate rather than a reading", () => {
    show({ estimatedPowerWatts: 187.4 });

    expect(screen.getByText("Estimated power")).toBeInTheDocument();
    expect(screen.getByText("187")).toBeInTheDocument();
    expect(screen.getByText("watts, from the track")).toBeInTheDocument();
    expect(screen.queryByText("Power")).not.toBeInTheDocument();
  });

  it("shows the estimate's quality diagnostics beside it", () => {
    show({
      estimatedPowerWatts: 187.4,
      estimateQuality: {
        autocorrelation: 0.923,
        meanAbsDeltaWattsPerSecond: 12.34,
        clipBiasWatts: 3.456,
      },
    });

    expect(screen.getByText("Estimate steadiness")).toBeInTheDocument();
    expect(screen.getByText("0.92")).toBeInTheDocument();
    expect(screen.getByText("lag-1 correlation")).toBeInTheDocument();
    expect(screen.getByText("Estimate jitter")).toBeInTheDocument();
    expect(screen.getByText("12.3")).toBeInTheDocument();
    expect(screen.getByText("watts change per second")).toBeInTheDocument();
    expect(screen.getByText("Clamp bias")).toBeInTheDocument();
    expect(screen.getByText("3.5")).toBeInTheDocument();
    expect(screen.getByText("watts the zero clamp added")).toBeInTheDocument();
  });

  it("leaves out the quality diagnostics when the ride has no estimate", () => {
    show({ averagePowerWatts: 196.2 });

    expect(screen.queryByText("Estimate steadiness")).not.toBeInTheDocument();
    expect(screen.queryByText("Estimate jitter")).not.toBeInTheDocument();
    expect(screen.queryByText("Clamp bias")).not.toBeInTheDocument();
  });

  it("names each zone and how long the ride held it", () => {
    show({ zoneSeconds: [60, 120, 180, 240, 300] });

    expect(screen.getByText("Recovery")).toBeInTheDocument();
    expect(screen.getByText("VO₂ max")).toBeInTheDocument();
    expect(screen.getByText("1 min")).toBeInTheDocument();
    expect(screen.getByText("5 min")).toBeInTheDocument();
    expect(screen.getAllByRole("listitem")[0]).toHaveTextContent("Recovery1 min");
  });

  it("draws one bar whose segments are each zone's share of the ride", () => {
    const { container } = show({ zoneSeconds: [60, 120, 0, 240, 120] });

    const widths = segments(container).map((segment) => Number.parseFloat(segment.style.width));
    expect(widths).toHaveLength(5);
    expect(widths[0]).toBeCloseTo(11.11, 1);
    expect(widths[1]).toBeCloseTo(22.22, 1);
    expect(widths[2]).toBe(0);
    expect(widths[3]).toBeCloseTo(44.44, 1);
    expect(widths.reduce((sum, width) => sum + width, 0)).toBeCloseTo(100, 5);
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

  it("shows nothing at all for a ride with nothing to say about effort", () => {
    const { rerender } = show(undefined, { movingSeconds: 0 });
    expect(screen.queryByLabelText("Effort")).not.toBeInTheDocument();

    rerender(<TrainingLoad ride={undefined} />);
    expect(screen.queryByLabelText("Effort")).not.toBeInTheDocument();
  });

  it("shows no zones for a row whose zones are all empty", () => {
    show({ zoneSeconds: [0, 0, 0, 0, 0] });

    expect(screen.queryByText("Recovery")).not.toBeInTheDocument();
    expect(screen.getByText("Speed")).toBeInTheDocument();
  });
});
