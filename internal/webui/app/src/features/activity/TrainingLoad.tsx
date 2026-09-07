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

/**
 * A zone's time, exactly as long as it was. Unlike a predicted moving time this
 * is measured, so it is not rounded at all: a rider who spent forty seconds at
 * VO₂ max deserves to be told forty seconds, and one who spent ninety there is
 * not told two minutes.
 */
function formatZoneTime(seconds: number): string {
  // Floored at every step, never rounded: a minute and a half is a minute and
  // a half, not two minutes. Seconds are dropped once there is an hour to show,
  // which shortens the label without ever overstating it.
  const whole = Math.floor(seconds);
  const hours = Math.floor(whole / 3600);
  const minutes = Math.floor((whole % 3600) / 60);
  const rest = whole % 60;
  if (hours > 0) {
    return minutes === 0 ? `${hours} h` : `${hours} h ${minutes} min`;
  }
  if (minutes > 0) {
    return rest === 0 ? `${minutes} min` : `${minutes} min ${rest} s`;
  }

  return `${rest} s`;
}

/** The five zones, easiest first, as a rider reading a training app knows them. */
const ZONE_NAMES = ["Recovery", "Endurance", "Tempo", "Threshold", "VO₂ max"];

/** One load figure: what it is called, and the scale it is on. */
interface Scale {
  label: string;
  scale: string;
  value: number | undefined;
  decimals?: number;
}

function Figure({ label, scale, value, decimals = 0 }: Scale) {
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

/** The zone bar, each zone as wide as the share of the ride it held. */
function ZoneBar({ zoneSeconds }: { zoneSeconds: number[] }) {
  const total = zoneSeconds.reduce((sum, seconds) => sum + seconds, 0);
  if (total <= 0) {
    return null;
  }

  return (
    <div className="flex flex-col gap-2">
      <div className="flex h-3 overflow-hidden rounded-full" role="presentation">
        {zoneSeconds.map((seconds, zone) => (
          <div
            // Zones are a fixed ordered set of five, so the index is their identity.
            key={ZONE_NAMES[zone]}
            className="h-full"
            style={{
              width: `${(seconds / total) * 100}%`,
              // Easiest to hardest across the accent, so the bar reads as one
              // gradient rather than five unrelated colours.
              backgroundColor: `color-mix(in oklab, var(--accent) ${20 + zone * 20}%, var(--panel))`,
            }}
          />
        ))}
      </div>
      <ul className="grid grid-cols-2 gap-x-4 gap-y-1 sm:grid-cols-5">
        {zoneSeconds.map((seconds, zone) => (
          <li key={ZONE_NAMES[zone]} className="flex flex-col">
            <span className="text-[var(--ink-2)] text-xs">{ZONE_NAMES[zone]}</span>
            <span className="text-sm tabular-nums">{formatZoneTime(seconds)}</span>
          </li>
        ))}
      </ul>
    </div>
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
      {metrics.zoneSeconds ? <ZoneBar zoneSeconds={metrics.zoneSeconds} /> : null}
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
