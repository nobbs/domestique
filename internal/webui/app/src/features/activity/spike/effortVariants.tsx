/**
 * Five positions on how a ride's time in heart-rate zones should read, all fed
 * the same ride. Storybook only; nothing here is imported by the application.
 */

import type { ReactNode } from "react";
import { formatDuration } from "../../../lib/format";

const ZONE_NAMES = ["Recovery", "Endurance", "Tempo", "Threshold", "VO₂ max"];
const BOUNDS = [120, 134, 140, 148];
const ZONES = [6360, 7440, 1948, 1012, 178];
const DEVICE = [1688, 8520, 5340, 1417, 2];
const TOTAL = ZONES.reduce((sum, seconds) => sum + seconds, 0);

const colour = (zone: number) => `var(--grade-${zone})`;

function range(zone: number): string {
  const low = BOUNDS[zone - 1];
  const high = BOUNDS[zone];
  if (low === undefined) {
    return `below ${high} bpm`;
  }
  return high === undefined ? `${low} bpm and up` : `${low}–${high - 1} bpm`;
}

function share(seconds: number, total = TOTAL): string {
  const percent = (seconds / total) * 100;
  return percent > 0 && percent < 1 ? "<1%" : `${Math.round(percent)}%`;
}

function Card({ children, note }: { children: ReactNode; note?: string }) {
  return (
    <section className="flex flex-col gap-4 rounded-xl bg-[var(--panel)] p-4 ring-1 ring-black/5">
      <div className="flex items-baseline justify-between gap-4">
        <h2 className="font-medium text-sm">Effort</h2>
        {note ? <span className="text-[var(--ink-2)] text-xs">{note}</span> : null}
      </div>
      {children}
    </section>
  );
}

/** A · One row per zone, bar length its time; the device's cut is a hairline under each bar, so the two compare zone by zone. */
export function RowsEffort() {
  const longest = Math.max(...ZONES, ...DEVICE);
  return (
    <Card note={`${formatDuration(TOTAL)} with heart rate`}>
      <ul className="grid grid-cols-[minmax(7rem,auto)_1fr_auto] items-center gap-x-4 gap-y-3">
        {ZONES.map((seconds, zone) => (
          <li key={ZONE_NAMES[zone]} className="contents">
            <div className="flex flex-col leading-tight">
              <span className="text-sm">{ZONE_NAMES[zone]}</span>
              <span className="text-[10px] text-[var(--ink-2)] tabular-nums">{range(zone)}</span>
            </div>
            <div className="flex flex-col gap-1">
              <span
                className="h-2.5 min-w-0.5 rounded-full"
                style={{ width: `${(seconds / longest) * 100}%`, backgroundColor: colour(zone) }}
              />
              <span
                title={`Device: ${formatDuration(DEVICE[zone])}`}
                className="h-px min-w-0.5 bg-[var(--ink-2)] opacity-50"
                style={{ width: `${((DEVICE[zone] ?? 0) / longest) * 100}%` }}
              />
            </div>
            <div className="flex items-baseline justify-end gap-2 text-sm tabular-nums">
              <span>{formatDuration(seconds)}</span>
              <span className="w-9 text-right text-[var(--ink-2)] text-xs">{share(seconds)}</span>
            </div>
          </li>
        ))}
      </ul>
      <p className="flex items-center gap-2 text-[var(--ink-2)] text-xs">
        <span className="h-px w-5 bg-[var(--ink-2)] opacity-50" /> the head unit's own zones
      </p>
    </Card>
  );
}

/** B · A histogram: five columns side by side, the device's cut a hollow twin beside each. */
export function ColumnsEffort() {
  const longest = Math.max(...ZONES, ...DEVICE);
  return (
    <Card>
      <div className="grid grid-cols-5 gap-3">
        {ZONES.map((seconds, zone) => (
          <div key={ZONE_NAMES[zone]} className="flex flex-col gap-1">
            <div className="flex h-40 items-end justify-center gap-1">
              <div
                className="flex w-8 min-h-0.5 flex-col"
                style={{ height: `${(seconds / longest) * 100}%` }}
              >
                <span className="-mt-5 h-5 text-center text-[var(--ink-2)] text-xs tabular-nums">
                  {share(seconds)}
                </span>
                <span
                  className="min-h-0.5 flex-1 rounded-t"
                  style={{ backgroundColor: colour(zone) }}
                />
              </div>
              <span
                title={`Device: ${formatDuration(DEVICE[zone])}`}
                className="w-3 min-h-0.5 rounded-t border border-[var(--ink-2)] border-b-0 opacity-40"
                style={{ height: `${((DEVICE[zone] ?? 0) / longest) * 100}%` }}
              />
            </div>
            <span className="h-1 rounded-full" style={{ backgroundColor: colour(zone) }} />
            <span className="text-xs">{ZONE_NAMES[zone]}</span>
            <span className="text-[10px] text-[var(--ink-2)] tabular-nums">{range(zone)}</span>
            <span className="text-sm tabular-nums">{formatDuration(seconds)}</span>
          </div>
        ))}
      </div>
    </Card>
  );
}

/** C · The polarised reading first: easy, moderate, hard as three large shares, the five zones as their small print. */
export function BandsEffort() {
  const bands = [
    { name: "Easy", zones: [0, 1] },
    { name: "Moderate", zones: [2] },
    { name: "Hard", zones: [3, 4] },
  ].map((band) => ({ ...band, seconds: band.zones.reduce((sum, z) => sum + (ZONES[z] ?? 0), 0) }));

  return (
    <Card>
      <div className="flex overflow-hidden rounded-full">
        {ZONES.map((seconds, zone) => (
          <span
            key={ZONE_NAMES[zone]}
            className="h-3 border-[var(--panel)] border-r-2 last:border-r-0"
            style={{ width: `${(seconds / TOTAL) * 100}%`, backgroundColor: colour(zone) }}
          />
        ))}
      </div>
      <div className="grid grid-cols-3 gap-4">
        {bands.map((band) => (
          <div key={band.name} className="flex flex-col gap-1">
            <span className="text-[var(--ink-2)] text-xs">{band.name}</span>
            <span className="font-semibold text-3xl tabular-nums leading-none">
              {share(band.seconds)}
            </span>
            <span className="text-sm tabular-nums">{formatDuration(band.seconds)}</span>
            <ul className="mt-1 flex flex-col gap-0.5">
              {band.zones.map((zone) => (
                <li key={zone} className="flex items-center gap-1.5 text-[var(--ink-2)] text-xs">
                  <span className="size-2 rounded-full" style={{ backgroundColor: colour(zone) }} />
                  <span>{ZONE_NAMES[zone]}</span>
                  <span className="ml-auto tabular-nums">{formatDuration(ZONES[zone])}</span>
                </li>
              ))}
            </ul>
          </div>
        ))}
      </div>
    </Card>
  );
}

/** D · A ring: the total at its centre, a legend table beside it carrying range, time and share. */
export function RingEffort() {
  let offset = 0;
  return (
    <Card>
      <div className="flex flex-wrap items-center gap-6">
        <div className="relative size-40 shrink-0">
          <svg viewBox="0 0 42 42" className="-rotate-90 size-full" aria-hidden="true">
            {ZONES.map((seconds, zone) => {
              const length = (seconds / TOTAL) * 100;
              const dash = (
                <circle
                  key={ZONE_NAMES[zone]}
                  cx="21"
                  cy="21"
                  r="15.9155"
                  fill="none"
                  stroke={colour(zone)}
                  strokeWidth="5"
                  strokeDasharray={`${Math.max(length - 0.6, 0.3)} ${100 - length + 0.6}`}
                  strokeDashoffset={-offset}
                />
              );
              offset += length;
              return dash;
            })}
          </svg>
          <div className="absolute inset-0 flex flex-col items-center justify-center">
            <span className="font-semibold text-lg tabular-nums">{formatDuration(TOTAL)}</span>
            <span className="text-[var(--ink-2)] text-xs">in zones</span>
          </div>
        </div>
        <table className="min-w-60 flex-1 text-sm tabular-nums">
          <tbody>
            {ZONES.map((seconds, zone) => (
              <tr key={ZONE_NAMES[zone]} className="border-black/5 border-b last:border-0">
                <td className="py-1.5 pr-2">
                  <span
                    className="inline-block size-2.5 rounded-full align-middle"
                    style={{ backgroundColor: colour(zone) }}
                  />
                </td>
                <td className="py-1.5">{ZONE_NAMES[zone]}</td>
                <td className="py-1.5 text-[var(--ink-2)] text-xs">{range(zone)}</td>
                <td className="py-1.5 text-right">{formatDuration(seconds)}</td>
                <td className="w-10 py-1.5 text-right text-[var(--ink-2)] text-xs">
                  {share(seconds)}
                </td>
              </tr>
            ))}
          </tbody>
        </table>
      </div>
    </Card>
  );
}

/** A deterministic heart-rate trace, one sample per 30 s, standing in for the ride's fetched series. */
function trace(): number[] {
  let seed = 7;
  const noise = () => {
    seed = (seed * 16807) % 2147483647;
    return seed / 2147483647 - 0.5;
  };
  return Array.from({ length: Math.round(TOTAL / 30) }, (_, i) => {
    const climb = i % 90 > 78 ? 16 : 0;
    const hr = 122 + 9 * Math.sin(i / 41) + 6 * Math.sin(i / 9) + climb + noise() * 10;
    return Math.max(92, Math.min(168, Math.round(hr)));
  });
}

const zoneOf = (bpm: number) => BOUNDS.filter((bound) => bpm >= bound).length;

/** E · From the series: where the beats sat (a 2-bpm histogram under the zone edges), and when (a ribbon of the ride's clock). */
export function SeriesEffort() {
  const samples = trace();
  const low = 90;
  const bins = Array.from({ length: 40 }, () => 0);
  for (const bpm of samples) {
    const bin = Math.floor((bpm - low) / 2);
    bins[bin] = (bins[bin] ?? 0) + 1;
  }
  const tallest = Math.max(...bins);
  const counts = ZONES.map((_, zone) => samples.filter((bpm) => zoneOf(bpm) === zone).length);
  const x = (bpm: number) => ((bpm - low) / 80) * 400;

  return (
    <Card note="needs the heart-rate series, fetched on demand">
      <svg viewBox="0 0 400 130" className="w-full" role="img" aria-label="Heart-rate histogram">
        {bins.map((count, bin) => {
          const height = (count / tallest) * 100;
          return (
            <rect
              // biome-ignore lint/suspicious/noArrayIndexKey: bins are a fixed bpm order
              key={bin}
              x={x(low + bin * 2) + 0.5}
              y={105 - height}
              width={9}
              height={height}
              fill={colour(zoneOf(low + bin * 2))}
            />
          );
        })}
        {BOUNDS.map((bound) => (
          <g key={bound}>
            <line
              x1={x(bound)}
              x2={x(bound)}
              y1={0}
              y2={105}
              stroke="var(--ink-2)"
              strokeDasharray="2 2"
              strokeWidth={0.5}
            />
            <text x={x(bound)} y={118} textAnchor="middle" fontSize={8} fill="var(--ink-2)">
              {bound}
            </text>
          </g>
        ))}
        <text x={0} y={128} fontSize={8} fill="var(--ink-2)">
          bpm
        </text>
      </svg>
      <div className="flex flex-col gap-1">
        <svg
          viewBox={`0 0 ${samples.length} 1`}
          preserveAspectRatio="none"
          className="h-5 w-full rounded"
          shapeRendering="crispEdges"
          aria-hidden="true"
        >
          {samples.map((bpm, i) => (
            // biome-ignore lint/suspicious/noArrayIndexKey: samples are a fixed clock order
            <rect key={i} x={i} y={0} width={1.05} height={1} fill={colour(zoneOf(bpm))} />
          ))}
        </svg>
        <div className="flex justify-between text-[10px] text-[var(--ink-2)] tabular-nums">
          <span>start</span>
          <span>{formatDuration(TOTAL)}</span>
        </div>
      </div>
      <ul className="flex flex-wrap gap-x-4 gap-y-1 text-xs">
        {counts.map((count, zone) => (
          <li key={ZONE_NAMES[zone]} className="flex items-center gap-1.5 tabular-nums">
            <span className="size-2 rounded-full" style={{ backgroundColor: colour(zone) }} />
            {ZONE_NAMES[zone]}{" "}
            <span className="text-[var(--ink-2)]">{share(count, samples.length)}</span>
          </li>
        ))}
      </ul>
    </Card>
  );
}
