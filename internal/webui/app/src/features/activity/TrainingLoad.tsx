/**
 * How hard one ride was, in the two vocabularies this service keeps side by
 * side: time in heart-rate zones, and training load on two scales.
 *
 * The two load scales are shown together rather than reconciled — they answer
 * different questions and neither converts to the other — and each is named, so
 * a number is never a bare figure the reader has to guess the meaning of.
 * Anything the ride's sensors or the rider's profile did not allow is left out
 * rather than shown as a zero.
 */

import type { ActivityMetrics } from "../../api/types";
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

/** One bar per zone, all on the scale the longest zone sets. */
function ZoneBars({
  zoneSeconds,
  zoneBounds,
}: {
  zoneSeconds: number[];
  zoneBounds: number[] | undefined;
}) {
  const longest = Math.max(...zoneSeconds);
  if (longest <= 0) {
    return null;
  }
  const ranges = zoneBounds ? zoneRanges(zoneBounds) : [];

  return (
    <ul className="flex flex-col gap-1.5">
      {zoneSeconds.map((seconds, zone) => (
        // Zones are a fixed ordered set of five, so the name is their identity.
        <li key={ZONE_NAMES[zone]} className="grid grid-cols-[7.5rem_1fr_auto] items-center gap-3">
          <span className="flex flex-col text-[var(--ink-2)]">
            <span className="text-xs">{ZONE_NAMES[zone]}</span>
            {ranges[zone] ? (
              <span className="text-[10px] tabular-nums opacity-70">{ranges[zone]}</span>
            ) : null}
          </span>
          {/* The time beside it says the same thing, so the bar is decoration. */}
          <span aria-hidden="true" className="flex h-2.5 rounded-full bg-black/5">
            <span
              className="h-full rounded-full"
              style={{
                width: `${(seconds / longest) * 100}%`,
                // Easiest to hardest across the accent, so the five bars read as
                // one scale rather than five unrelated colours.
                backgroundColor: `color-mix(in oklab, var(--accent) ${20 + zone * 20}%, var(--panel))`,
              }}
            />
          </span>
          <span className="text-sm tabular-nums">{formatDuration(seconds)}</span>
        </li>
      ))}
    </ul>
  );
}

export function TrainingLoad({ metrics }: { metrics: ActivityMetrics | undefined }) {
  if (!metrics) {
    return null;
  }
  const figures: Scale[] = [
    { label: "TRIMP", scale: "Banister", value: metrics.trimp },
    { label: "hrTSS", scale: "heart rate", value: metrics.heartRateTss },
    { label: "TSS", scale: "power", value: metrics.powerTss },
    { label: "Normalized power", scale: "watts", value: metrics.normalizedPowerWatts },
    { label: "Intensity", scale: "of threshold", value: metrics.intensityFactor, decimals: 2 },
  ];
  const shown = figures.filter((figure) => figure.value !== undefined);
  if (shown.length === 0 && !metrics.zoneSeconds) {
    return null;
  }

  return (
    <section
      className="flex flex-col gap-4 rounded-xl bg-[var(--panel)] p-4 ring-1 ring-black/5"
      aria-label="Training load"
    >
      <h2 className="font-medium text-sm">Training load</h2>
      {metrics.zoneSeconds ? (
        <ZoneBars zoneSeconds={metrics.zoneSeconds} zoneBounds={metrics.zoneBoundsBpm} />
      ) : null}
      {shown.length > 0 ? (
        <div className="flex flex-wrap gap-x-8 gap-y-3">
          {shown.map((figure) => (
            <Figure key={figure.label} {...figure} />
          ))}
        </div>
      ) : null}
    </section>
  );
}
