/**
 * How hard one ride was: its time in heart-rate zones as one bar, beside what
 * its sensors averaged and the load it came to on every scale the ride allowed.
 *
 * The load scales are shown together rather than reconciled — they answer
 * different questions and neither converts to the other — and each is named,
 * so a number is never a bare figure the reader has to guess the meaning of.
 * Anything the ride's sensors or the rider's profile did not allow is left out
 * rather than shown as a zero.
 */

import type { Activity, ActivityMetrics } from "../../api/types";
import { formatDuration } from "../../lib/format";

/** The five zones, easiest first, as a rider reading a training app knows them. */
const ZONE_NAMES = ["Recovery", "Endurance", "Tempo", "Threshold", "VO₂ max"];

/** One figure: what it is called, and the scale it is on. */
export interface Scale {
  label: string;
  scale: string;
  value: number | undefined;
  decimals?: number;
}

export function Figure({ label, scale, value, decimals = 0 }: Scale) {
  if (value === undefined) {
    return null;
  }

  return (
    <div className="flex flex-col gap-0.5">
      <span className="text-[var(--ink-2)] text-xs">{label}</span>
      <span className="font-semibold text-lg tabular-nums">{value.toFixed(decimals)}</span>
      <span className="text-[var(--ink-2)] text-xs">{scale}</span>
    </div>
  );
}

/**
 * The heart rates each zone covers, easiest first. Open at both ends — the
 * easiest zone has nothing below it and the hardest nothing above — so neither
 * is given a limit the profile never said. A bound cut from a percentage lands
 * between two beats, and a sample below it is still the easier zone, so both
 * edges take the ceiling rather than the nearer beat.
 */
function zoneRanges(bounds: number[]): string[] {
  const ranges: string[] = [];
  let low: number | undefined;
  for (const bound of bounds) {
    const edge = Math.ceil(bound);
    ranges.push(low === undefined ? `below ${edge} bpm` : `${low}–${edge - 1} bpm`);
    low = edge;
  }
  if (low !== undefined) {
    ranges.push(`${low} bpm and up`);
  }

  return ranges;
}

/** Easiest to hardest on the severity ramp the gradient bands wear. */
function zoneColour(zone: number): string {
  return `var(--grade-${zone})`;
}

/** One bar, five segments: the ride's time as a whole, each zone its share of it. */
function ZoneStack({
  zoneSeconds,
  zoneBounds,
}: {
  zoneSeconds: number[];
  zoneBounds: number[] | undefined;
}) {
  const total = zoneSeconds.reduce((sum, seconds) => sum + seconds, 0);
  if (total <= 0) {
    return null;
  }
  const ranges = zoneBounds ? zoneRanges(zoneBounds) : [];

  return (
    <div className="flex flex-col gap-3">
      {/* The legend beside it says the same thing, so the bar is decoration. */}
      <div aria-hidden="true" className="flex h-3 overflow-hidden rounded-full bg-black/5">
        {zoneSeconds.map((seconds, zone) => (
          <span
            key={ZONE_NAMES[zone]}
            className="h-full"
            style={{ width: `${(seconds / total) * 100}%`, backgroundColor: zoneColour(zone) }}
          />
        ))}
      </div>
      <ul className="grid grid-cols-5 gap-2">
        {zoneSeconds.map((seconds, zone) => (
          // Zones are a fixed ordered set of five, so the name is their identity.
          <li key={ZONE_NAMES[zone]} className="flex flex-col gap-0.5">
            <span
              aria-hidden="true"
              className="h-1 rounded-full"
              style={{ backgroundColor: zoneColour(zone) }}
            />
            <span className="text-[var(--ink-2)] text-xs">{ZONE_NAMES[zone]}</span>
            {ranges[zone] ? (
              <span className="text-[10px] text-[var(--ink-2)] tabular-nums opacity-70">
                {ranges[zone]}
              </span>
            ) : null}
            <span className="text-sm tabular-nums">{formatDuration(seconds)}</span>
          </li>
        ))}
      </ul>
    </div>
  );
}

/**
 * The ride's average speed in kilometres per hour, from the summary totals.
 *
 * Distance over moving time, so it is there even for a ride whose recorded
 * file was never readable; a ride whose moving time is nought has no speed
 * rather than an infinite one.
 */
function averageSpeedKmh(ride: Activity): number | undefined {
  if (!Number.isFinite(ride.distanceMetres) || !(ride.movingSeconds > 0)) {
    return undefined;
  }

  return (ride.distanceMetres / ride.movingSeconds) * 3.6;
}

function figuresFor(ride: Activity, metrics: ActivityMetrics | undefined): Scale[] {
  return [
    { label: "Speed", scale: "km/h average", value: averageSpeedKmh(ride), decimals: 1 },
    { label: "Max speed", scale: "km/h", value: metrics?.maxSpeedKmh, decimals: 1 },
    { label: "Heart rate", scale: "bpm average", value: metrics?.averageHeartRateBpm },
    { label: "Max heart rate", scale: "bpm", value: metrics?.maxHeartRateBpm },
    { label: "Cadence", scale: "rpm average", value: metrics?.averageCadenceRpm },
    { label: "Power", scale: "watts average", value: metrics?.averagePowerWatts },
    // Never beside a measured average: the service serves one or the other, and
    // the label carries the estimate's provenance so it cannot read as a reading.
    {
      label: "Estimated power",
      scale: "watts, from the track",
      value: metrics?.estimatedPowerWatts,
    },
    {
      label: "Estimate steadiness",
      scale: "lag-1 correlation",
      value: metrics?.estimateQuality?.autocorrelation,
      decimals: 2,
    },
    {
      label: "Estimate jitter",
      scale: "watts change per second",
      value: metrics?.estimateQuality?.meanAbsDeltaWattsPerSecond,
      decimals: 1,
    },
    {
      label: "Clamp bias",
      scale: "watts the zero clamp added",
      value: metrics?.estimateQuality?.clipBiasWatts,
      decimals: 1,
    },
    { label: "Normalized power", scale: "watts", value: metrics?.normalizedPowerWatts },
    { label: "Intensity", scale: "of threshold", value: metrics?.intensityFactor, decimals: 2 },
    { label: "TSS", scale: "power", value: metrics?.powerTss },
    { label: "hrTSS", scale: "heart rate", value: metrics?.heartRateTss },
    { label: "TRIMP", scale: "Banister", value: metrics?.trimp },
    // Positive is the usual direction, and the scale says so: the reader is
    // told what the number measures rather than sold what it means.
    {
      label: "Decoupling",
      scale: "% of ratio lost over the second half",
      value: metrics?.decouplingPercent,
      decimals: 1,
    },
    // The pair is one figure and its condition: the beats, at the degrees they
    // were held at. One ride is a point, not a trend.
    {
      label: "Heat drift",
      scale:
        metrics?.heatDrift === undefined
          ? ""
          : `bpm in the endurance band at ${Math.round(metrics.heatDrift.temperatureCelsius)} °C`,
      value: metrics?.heatDrift?.heartRateBpm,
    },
  ].filter((figure) => figure.value !== undefined);
}

export function TrainingLoad({ ride }: { ride: Activity | undefined }) {
  if (!ride) {
    return null;
  }
  const metrics = ride.metrics;
  const zones = metrics?.zoneSeconds?.some((seconds) => seconds > 0) ? metrics.zoneSeconds : null;
  const figures = figuresFor(ride, metrics);
  if (!zones && figures.length === 0) {
    return null;
  }

  return (
    <section
      className="flex flex-col gap-4 rounded-xl bg-[var(--panel)] p-4 ring-1 ring-black/5"
      aria-label="Effort"
    >
      <h2 className="font-medium text-sm">Effort</h2>
      <div className={zones ? "grid gap-6 md:grid-cols-2" : ""}>
        {zones ? <ZoneStack zoneSeconds={zones} zoneBounds={metrics?.zoneBoundsBpm} /> : null}
        {figures.length > 0 ? (
          <div className="grid grid-cols-3 gap-x-4 gap-y-3">
            {figures.map((figure) => (
              <Figure key={figure.label} {...figure} />
            ))}
          </div>
        ) : null}
      </div>
    </section>
  );
}
