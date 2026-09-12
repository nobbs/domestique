import { render, screen } from "@testing-library/react";
import { describe, expect, it } from "vitest";
import type { Activity, ActivityMetrics } from "../../api/types";
import { TrainingLoad } from "./TrainingLoad";

function ride(metrics: ActivityMetrics | undefined, totals?: Partial<Activity>): Activity {
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
    // "Power" also names the group heading, so the figure is found by its tag.
    expect(screen.getByText("Power", { selector: "span" })).toBeInTheDocument();
    expect(screen.getByText("196")).toBeInTheDocument();
  });

  it("groups a power-meter ride's figures under Sensors, Power, Load and Physiology", () => {
    show({
      averageHeartRateBpm: 142.4,
      averageCadenceRpm: 81.6,
      averagePowerWatts: 196.2,
      normalizedPowerWatts: 214,
      trimp: 42.4,
      decouplingPercent: 4.2,
    });

    expect(screen.getByRole("heading", { name: "Sensors" })).toBeInTheDocument();
    expect(screen.getByRole("heading", { name: "Power" })).toBeInTheDocument();
    expect(screen.getByRole("heading", { name: "Load" })).toBeInTheDocument();
    expect(screen.getByRole("heading", { name: "Physiology" })).toBeInTheDocument();
    expect(screen.getByText("Power", { selector: "span" })).toBeInTheDocument();
    expect(screen.getByText("Normalized power")).toBeInTheDocument();
  });

  it("renders only the Sensors heading for a bare ride with speed alone", () => {
    show(undefined);

    expect(screen.getByRole("heading", { name: "Sensors" })).toBeInTheDocument();
    expect(screen.queryByRole("heading", { name: "Power" })).not.toBeInTheDocument();
    expect(screen.queryByRole("heading", { name: "Load" })).not.toBeInTheDocument();
    expect(screen.queryByRole("heading", { name: "Physiology" })).not.toBeInTheDocument();
  });

  it("shows the ride's maximum speed beside its average", () => {
    show({ maxSpeedKmh: 54.2 });

    expect(screen.getByText("Max speed")).toBeInTheDocument();
    expect(screen.getByText("54.2")).toBeInTheDocument();
  });

  it("shows no maximum speed for a ride with no speed series", () => {
    show({ averageHeartRateBpm: 142.4 });

    expect(screen.queryByText("Max speed")).not.toBeInTheDocument();
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
    expect(screen.queryByText("Power", { selector: "span" })).not.toBeInTheDocument();
  });

  it("shows the estimate's pedalling share as its scale", () => {
    show({ estimatedPowerWatts: 187.4, estimatedPedallingShare: 0.87 });

    expect(
      screen.getByText("watts while pedalling, 87% of its estimated samples"),
    ).toBeInTheDocument();
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

  it("shows the ride's decoupling and the heat drift beside it", () => {
    show({
      decouplingPercent: 4.2,
      heatDrift: { heartRateBpm: 141.6, temperatureCelsius: 29.4, samples: 1800 },
    });

    expect(screen.getByText("Decoupling")).toBeInTheDocument();
    expect(screen.getByText("4.2")).toBeInTheDocument();
    expect(screen.getByText("% of ratio lost over the second half")).toBeInTheDocument();
    expect(screen.getByText("Heat drift")).toBeInTheDocument();
    expect(screen.getByText("142")).toBeInTheDocument();
    // The pair, not the beats alone: a heart rate without its temperature is
    // not a reading of riding warm.
    expect(screen.getByText("bpm in the endurance band at 29 °C")).toBeInTheDocument();
  });

  it("shows neither where the ride carries neither", () => {
    show({ averageHeartRateBpm: 142.4 });

    expect(screen.queryByText("Decoupling")).toBeNull();
    expect(screen.queryByText("Heat drift")).toBeNull();
  });

  it("uses the server's average speed over the ride's own totals when it is given", () => {
    show({ averageSpeedKmh: 28.6 });

    expect(screen.getByText("28.6")).toBeInTheDocument();
  });

  it("shows the ride's maximum cadence and its threshold and maximum power", () => {
    show({ maxCadenceRpm: 108, maxPowerWatts: 612, thresholdPowerWatts: 260 });

    expect(screen.getByText("Max cadence")).toBeInTheDocument();
    expect(screen.getByText("108")).toBeInTheDocument();
    expect(screen.getByText("Max power")).toBeInTheDocument();
    expect(screen.getByText("612")).toBeInTheDocument();
    expect(screen.getByText("Threshold power")).toBeInTheDocument();
    expect(screen.getByText("260")).toBeInTheDocument();
    expect(screen.getByText("watts set on the device")).toBeInTheDocument();
  });

  it("leaves out the device figures a ride did not carry", () => {
    show({ averageHeartRateBpm: 142.4 });

    expect(screen.queryByText("Max cadence")).not.toBeInTheDocument();
    expect(screen.queryByText("Max power")).not.toBeInTheDocument();
    expect(screen.queryByText("Threshold power")).not.toBeInTheDocument();
  });

  it("still renders the profile's zones when the device also cut its own", () => {
    show({ zoneSeconds: [60, 120, 180, 240, 300], deviceZoneSeconds: [70, 110, 190, 230, 300] });

    expect(screen.getByText("Recovery")).toBeInTheDocument();
    expect(screen.getByText(/Device zones:/)).toBeInTheDocument();
  });

  it("leaves out the device zones caption for a ride the head unit did not cut", () => {
    show({ zoneSeconds: [60, 120, 180, 240, 300] });

    expect(screen.queryByText(/Device zones:/)).not.toBeInTheDocument();
  });

  it("leaves out the device zones caption for an empty zone table", () => {
    show({ zoneSeconds: [60, 120, 180, 240, 300], deviceZoneSeconds: [] });

    expect(screen.queryByText(/Device zones:/)).not.toBeInTheDocument();
  });

  // A figure served above the withhold threshold still held less than the
  // whole ride, and the reader is owed the share, not just the pass/fail.
  it("marks a heart-rate figure served below full coverage", () => {
    show({ averageHeartRateBpm: 124.6, heartRateCoverage: 0.92, trimp: 42.4 });

    expect(screen.getAllByText("92% sensor coverage")).toHaveLength(2);
  });

  it("marks a power figure served below full coverage", () => {
    show({
      averagePowerWatts: 196.2,
      powerCoverage: 0.85,
      normalizedPowerWatts: 214,
      powerTss: 73.2,
    });

    expect(screen.getAllByText("85% sensor coverage")).toHaveLength(3);
  });

  it("leaves out the coverage mark at full coverage", () => {
    show({ averageHeartRateBpm: 142.4, heartRateCoverage: 1 });

    expect(screen.queryByText(/sensor coverage/)).not.toBeInTheDocument();
  });

  it("leaves the estimate's own figure unmarked by the meter's coverage", () => {
    show({ estimatedPowerWatts: 187.4, powerCoverage: 0.5 });

    expect(screen.queryByText(/sensor coverage/)).not.toBeInTheDocument();
  });

  // A share of 99.6% is still not the whole ride: rounding it to "100%" would
  // print the one word this caption exists to rule out.
  it("floors the coverage share rather than rounding it up to 100%", () => {
    show({ averageHeartRateBpm: 142.4, heartRateCoverage: 0.996 });

    expect(screen.getByText("99% sensor coverage")).toBeInTheDocument();
    expect(screen.queryByText("100% sensor coverage")).not.toBeInTheDocument();
  });

  // Zones are withheld below the threshold, but a served zone bar can still
  // hold less than the whole ride, and is owed the same mark every other
  // heart-rate figure gets.
  it("marks the zone bar with the heart-rate coverage it was served at", () => {
    show({ zoneSeconds: [60, 120, 180, 240, 300], heartRateCoverage: 0.93 });

    expect(screen.getByText("93% sensor coverage")).toBeInTheDocument();
  });

  it("leaves the zone bar unmarked at full coverage", () => {
    show({ zoneSeconds: [60, 120, 180, 240, 300], heartRateCoverage: 1 });

    expect(screen.queryByText(/sensor coverage/)).not.toBeInTheDocument();
  });
});
