/**
 * One ride by the kilometre: how long each took, how fast it was, and what it
 * climbed.
 *
 * The service cuts the stretches, from the bicycle's own odometer, so the table
 * agrees with the distance the ride is listed at. A column with nothing to put
 * in it is left out rather than ruled down the page as dashes: heart rate and
 * power where no stretch carried the sensor, ascent where no stretch climbed —
 * the served figure being nought for a flat ride and for one that measured no
 * altitude alike, which is why the rule there is what was climbed rather than
 * what was fitted.
 */

import type { ActivitySplit } from "../../api/types";
import { formatAscent, formatDuration, formatKilometres } from "../../lib/format";

/**
 * A stretch's speed in kilometres per hour. A stretch the odometer never
 * advanced over — a gap in the recording — was not ridden slowly, so it has no
 * speed rather than one of nought.
 */
function speedKmh(split: ActivitySplit): number | undefined {
  return split.movingSeconds > 0 ? (split.distanceMetres / split.movingSeconds) * 3.6 : undefined;
}

export function RideSplits({ splits }: { splits: ActivitySplit[] | undefined }) {
  if (!splits || splits.length === 0) {
    return null;
  }
  const speeds = splits.map(speedKmh);
  // Nought where no stretch has a moving time to be fast over, which the bar
  // has to divide by rather than against.
  const fastest = Math.max(...speeds.map((speed) => speed ?? 0));
  const showAscent = splits.some((split) => split.ascentMetres > 0);
  const showHeartRate = splits.some((split) => split.heartRateBpm !== undefined);
  const showPower = splits.some((split) => split.powerWatts !== undefined);
  let covered = 0;

  return (
    <section
      className="flex flex-col gap-3 rounded-xl bg-[var(--panel)] p-4 ring-1 ring-black/5"
      aria-label="Splits"
    >
      <h2 className="font-medium text-sm">Splits</h2>
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
                // A split's place in the ride is its identity: the list is
                // ordered, fixed, and never reordered or filtered.
                // biome-ignore lint/suspicious/noArrayIndexKey: the index is the split
                <tr key={index} className="border-[var(--rule)] border-t">
                  <td className="py-1 tabular-nums">{formatKilometres(covered)}</td>
                  <td className="py-1 text-right tabular-nums">
                    {formatDuration(split.movingSeconds)}
                  </td>
                  <td className="py-1 text-right tabular-nums">
                    <span className="flex items-center justify-end gap-2">
                      {/* The figure beside it says the same thing, so the bar is decoration. */}
                      <span
                        aria-hidden="true"
                        className="hidden h-1.5 w-24 rounded-full bg-black/5 sm:flex"
                      >
                        <span
                          className="h-full rounded-full bg-[var(--accent)]"
                          style={{
                            width: fastest > 0 ? `${((speed ?? 0) / fastest) * 100}%` : "0%",
                          }}
                        />
                      </span>
                      <span>{speed === undefined ? "—" : `${speed.toFixed(1)} km/h`}</span>
                    </span>
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
    </section>
  );
}
