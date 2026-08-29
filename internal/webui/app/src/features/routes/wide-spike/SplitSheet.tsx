/**
 * **3 — Split.** A chart on the left, and the day as a list on the right.
 *
 * The wide panel is wide, and the other alternatives spend that width on one
 * continuous thing. But half of what a rider wants from a route is not
 * continuous at all — it is a handful of events. Three cols. The one where it
 * rains. Two hundred kilometres of axis is a poor way to ask "what happens on
 * this ride"; a list of six lines is a good one.
 *
 * So the left is the profile with the ground under it, and the right is the
 * ride as things that happen: each climb, when it will be reached, and the
 * weather that will be there when it is. Neither column can be read for the
 * other's question, which is why both are here.
 *
 * The join is the idea. A climb has a distance and the forecast has a distance,
 * so "col two, arriving at half past one, nineteen degrees and raining" is a
 * fact this service can already state and currently never does.
 */

import { ElevationProfile } from "../../../components/route/ElevationProfile";
import type { Cell } from "../../../components/route/forecast-spike/cells";
import { formatAscent, formatDistance, formatGradient } from "../../../lib/format";
import { PADDING } from "../../../lib/plotAxis";
import { weatherIcon } from "../../../lib/weather";
import { groundSegments, Ribbon } from "../panel-spike/shared";
import type { SheetProps } from "./shared";
import { clockAt, RideWindow, Sheet } from "./shared";

/** The reading nearest a point on the route, which is the weather a rider meets there. */
function cellAt(cells: Cell[], metres: number): Cell | null {
  return cells.reduce<Cell | null>((nearest, cell) => {
    if (nearest === null) {
      return cell;
    }

    return Math.abs(cell.sample.distanceMetres - metres) <
      Math.abs(nearest.sample.distanceMetres - metres)
      ? cell
      : nearest;
  }, null);
}

export function SplitSheet({
  route,
  profile,
  surface,
  climbs,
  cells,
  samples,
  startAt,
  activeMetres,
  onActiveChange,
  highlight,
  unitSystem,
}: SheetProps) {
  return (
    <Sheet>
      <div className="flex items-start gap-5">
        <div className="min-w-0 flex-1">
          <div className="mb-2">
            <RideWindow startAt={startAt} samples={samples} />
          </div>
          <ElevationProfile
            profile={profile}
            title={route.title}
            surface={surface}
            activeMetres={activeMetres}
            onActiveChange={onActiveChange}
            highlight={highlight}
            unitSystem={unitSystem}
          />
          <div
            className="mt-1.5 grid gap-1"
            style={{ paddingLeft: PADDING.left, paddingRight: PADDING.right }}
          >
            <Ribbon segments={groundSegments(surface)} className="h-3" highlight={highlight} />
          </div>
        </div>
        <div className="w-[19rem] shrink-0 border-l border-[var(--rule)] pl-5">
          <h3 className="mb-1.5 text-[11px] font-semibold tracking-[0.06em] text-[var(--ink-2)] uppercase">
            What happens
          </h3>
          {climbs.length === 0 ? (
            <p className="text-xs text-[var(--ink-2)]">Nothing sustained enough to call a climb.</p>
          ) : (
            <ol className="grid gap-1">
              {climbs.map((climb, index) => {
                // The middle of the climb rather than its foot: the weather a
                // rider remembers about a col is the weather on it.
                const middle = (climb.startMetres + climb.endMetres) / 2;
                const cell = cellAt(cells, middle);
                const Glyph = cell ? weatherIcon(cell.point.weatherCode) : null;

                return (
                  <li key={climb.startMetres}>
                    <button
                      type="button"
                      onClick={() => onActiveChange(middle)}
                      className="grid w-full grid-cols-[auto_1fr_auto] items-baseline gap-x-2 rounded-lg p-1.5 text-left hover:bg-[var(--base)] focus-visible:outline-2 focus-visible:outline-offset-[-2px] focus-visible:outline-[var(--accent)]"
                    >
                      <span className="text-xs font-semibold tabular-nums">Col {index + 1}</span>
                      <span className="text-xs text-[var(--ink-2)] tabular-nums">
                        {formatDistance(climb.distanceMetres, unitSystem)} ·{" "}
                        {formatAscent(climb.ascentMetres, unitSystem)} ·{" "}
                        {formatGradient(climb.averageGradePercent)}
                      </span>
                      {cell === null ? null : (
                        <span className="flex items-center gap-1 text-xs tabular-nums">
                          <span className="text-[var(--ink-2)]">
                            {clockAt(cell.sample.arrivalAt)}
                          </span>
                          {Glyph === null ? null : (
                            <Glyph size={14} stroke={1.8} aria-hidden="true" />
                          )}
                          <span className="font-semibold">
                            {Math.round(cell.point.temperatureCelsius)}°
                          </span>
                        </span>
                      )}
                    </button>
                  </li>
                );
              })}
            </ol>
          )}
        </div>
      </div>
    </Sheet>
  );
}
