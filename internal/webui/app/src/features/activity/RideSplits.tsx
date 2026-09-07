/**
 * One ride by the kilometre: a bar per stretch, its height the speed and its
 * colour how steep the stretch was, with the table behind a toggle for the
 * reader who wants the figures.
 *
 * The service cuts the stretches, from the bicycle's own odometer, so the bars
 * agree with the distance the ride is listed at. A table column with nothing
 * to put in it is left out rather than ruled down the page as dashes: heart
 * rate and power where no stretch carried the sensor, ascent where no stretch
 * climbed — the served figure being nought for a flat ride and for one that
 * measured no altitude alike, which is why the rule there is what was climbed
 * rather than what was fitted.
 */

import { IconChevronDown, IconChevronUp } from "@tabler/icons-react";
import { useCallback, useRef, useState } from "react";
import type { ActivitySplit } from "../../api/types";
import { formatAscent, formatDuration, formatKilometres } from "../../lib/format";
import { gradientBand } from "../../lib/profile";

/**
 * A stretch's speed in kilometres per hour. A stretch the odometer never
 * advanced over — a gap in the recording — was not ridden slowly, so it has no
 * speed rather than one of nought.
 */
function speedKmh(split: ActivitySplit): number | undefined {
  return split.movingSeconds > 0 ? (split.distanceMetres / split.movingSeconds) * 3.6 : undefined;
}

/** The bars' own units: any width, since the drawing is stretched to fit. */
const LANE = { width: 1000, height: 100 } as const;

export interface RideSplitsProps {
  splits: ActivitySplit[] | undefined;
  /** The position shared with the map and the profile, in metres from the start. */
  activeMetres?: number | null;
  onActiveChange?: (metres: number | null) => void;
  /**
   * How long the shared position's axis is — the profile's, measured from
   * positions — where the splits, cut from the odometer, add up to something
   * a little different. Absent leaves the splits' own sum as the axis.
   */
  axisMetres?: number;
}

export function RideSplits({
  splits,
  activeMetres = null,
  onActiveChange,
  axisMetres,
}: RideSplitsProps) {
  const [table, setTable] = useState(false);
  const plot = useRef<HTMLDivElement>(null);
  const total = splits?.reduce((sum, split) => sum + split.distanceMetres, 0) ?? 0;
  const axis = axisMetres ?? total;
  const onPointerMove = useCallback(
    (event: React.PointerEvent) => {
      const rect = plot.current?.getBoundingClientRect();
      if (!onActiveChange || axis <= 0 || !rect || rect.width === 0) {
        return;
      }
      const fraction = Math.min(Math.max((event.clientX - rect.left) / rect.width, 0), 1);
      onActiveChange(fraction * axis);
    },
    [onActiveChange, axis],
  );
  const onPointerLeave = useCallback(() => onActiveChange?.(null), [onActiveChange]);
  if (!splits || splits.length === 0) {
    return null;
  }
  const speeds = splits.map(speedKmh);
  // Nought where no stretch has a moving time to be fast over, which the bar
  // has to divide by rather than against.
  const fastest = Math.max(...speeds.map((speed) => speed ?? 0));
  // The shared position is on the axis; the splits are on the odometer.
  const active = activeSplit(
    splits,
    activeMetres === null || axis <= 0 ? null : (activeMetres / axis) * total,
  );
  // Nought where every stretch is of no length, which the bars must not divide by.
  const perMetre = total > 0 ? LANE.width / total : 0;
  let covered = 0;

  return (
    <section
      className="flex flex-col gap-3 rounded-xl bg-[var(--panel)] p-4 ring-1 ring-black/5"
      aria-label="Splits"
    >
      <div className="flex items-baseline justify-between gap-3">
        <h2 className="font-medium text-sm">By the kilometre</h2>
        <button
          type="button"
          onClick={() => setTable(!table)}
          aria-expanded={table}
          className="flex items-center gap-1 text-[var(--ink-2)] text-xs hover:text-[var(--ink)]"
        >
          {table ? "Hide the table" : "Show the table"}
          {table ? <IconChevronUp size={14} /> : <IconChevronDown size={14} />}
        </button>
      </div>
      {/* The table below says the same thing, so the bars are decoration. */}
      <div
        ref={plot}
        onPointerMove={onPointerMove}
        onPointerLeave={onPointerLeave}
        aria-hidden="true"
      >
        <svg
          viewBox={`0 0 ${LANE.width} ${LANE.height}`}
          preserveAspectRatio="none"
          className="block h-24 w-full"
        >
          {splits.map((split, index) => {
            const start = covered;
            covered += split.distanceMetres;
            const x = start * perMetre;
            const width = split.distanceMetres * perMetre;
            const bar = fastest > 0 ? ((speeds[index] ?? 0) / fastest) * LANE.height : 0;

            return (
              <rect
                // A split's place in the ride is its identity: the list is
                // ordered, fixed, and never reordered or filtered.
                // biome-ignore lint/suspicious/noArrayIndexKey: the index is the split
                key={index}
                x={x + 1}
                y={LANE.height - bar}
                width={Math.max(width - 2, 0)}
                height={bar}
                rx={1}
                fill={`var(--grade-${gradientBand(gradientPercent(split))})`}
                opacity={active === null || active === index ? 1 : 0.45}
              />
            );
          })}
        </svg>
      </div>
      <p className="text-[var(--ink-2)] text-xs tabular-nums">
        {active === null
          ? "Speed by the kilometre, coloured by how steep it was."
          : readout(splits, active)}
      </p>
      {table ? <SplitsTable splits={splits} speeds={speeds} /> : null}
    </section>
  );
}

/** The climb over the stretch as a gradient, which the bar's colour is read from. */
function gradientPercent(split: ActivitySplit): number {
  return split.distanceMetres > 0 ? (split.ascentMetres / split.distanceMetres) * 100 : 0;
}

/** Which stretch the shared position is on, or null for none. */
function activeSplit(splits: ActivitySplit[], metres: number | null): number | null {
  if (metres === null) {
    return null;
  }
  let covered = 0;
  for (const [index, split] of splits.entries()) {
    covered += split.distanceMetres;
    if (metres < covered) {
      return index;
    }
  }

  return splits.length - 1;
}

/** One stretch's figures in a line, for the position under the cursor. */
function readout(splits: ActivitySplit[], index: number): string {
  const split = splits[index];
  if (!split) {
    return "";
  }
  const end = splits.slice(0, index + 1).reduce((sum, one) => sum + one.distanceMetres, 0);
  const speed = speedKmh(split);
  const parts = [formatKilometres(end), speed === undefined ? "—" : `${speed.toFixed(1)} km/h`];
  // The table's own rule: ascent is said only where some stretch climbed.
  if (splits.some((one) => one.ascentMetres > 0)) {
    parts.push(formatAscent(split.ascentMetres));
  }
  if (split.heartRateBpm !== undefined) {
    parts.push(`${Math.round(split.heartRateBpm)} bpm`);
  }
  if (split.powerWatts !== undefined) {
    parts.push(`${Math.round(split.powerWatts)} W`);
  }

  return parts.join(" · ");
}

function SplitsTable({
  splits,
  speeds,
}: {
  splits: ActivitySplit[];
  speeds: (number | undefined)[];
}) {
  const showAscent = splits.some((split) => split.ascentMetres > 0);
  const showHeartRate = splits.some((split) => split.heartRateBpm !== undefined);
  const showPower = splits.some((split) => split.powerWatts !== undefined);
  let covered = 0;

  return (
    <div className="overflow-x-auto">
      <table className="w-full border-collapse text-sm">
        <thead>
          <tr className="text-[var(--ink-2)] text-xs">
            <th className="pb-1 text-left font-normal">At</th>
            <th className="pb-1 text-right font-normal">Time</th>
            <th className="pb-1 text-right font-normal">Speed</th>
            {showAscent ? <th className="pb-1 text-right font-normal">Ascent</th> : null}
            {showHeartRate ? <th className="pb-1 text-right font-normal">Heart rate</th> : null}
            {showPower ? <th className="pb-1 text-right font-normal">Power</th> : null}
          </tr>
        </thead>
        <tbody>
          {splits.map((split, index) => {
            covered += split.distanceMetres;
            const speed = speeds[index];

            return (
              // biome-ignore lint/suspicious/noArrayIndexKey: the index is the split
              <tr key={index} className="border-[var(--rule)] border-t">
                <td className="py-1 tabular-nums">{formatKilometres(covered)}</td>
                <td className="py-1 text-right tabular-nums">
                  {formatDuration(split.movingSeconds)}
                </td>
                <td className="py-1 text-right tabular-nums">
                  {speed === undefined ? "—" : `${speed.toFixed(1)} km/h`}
                </td>
                {showAscent ? (
                  <td className="py-1 text-right tabular-nums">
                    {formatAscent(split.ascentMetres)}
                  </td>
                ) : null}
                {showHeartRate ? (
                  <td className="py-1 text-right tabular-nums">
                    {split.heartRateBpm === undefined
                      ? "—"
                      : `${Math.round(split.heartRateBpm)} bpm`}
                  </td>
                ) : null}
                {showPower ? (
                  <td className="py-1 text-right tabular-nums">
                    {split.powerWatts === undefined ? "—" : `${Math.round(split.powerWatts)} W`}
                  </td>
                ) : null}
              </tr>
            );
          })}
        </tbody>
      </table>
    </div>
  );
}
