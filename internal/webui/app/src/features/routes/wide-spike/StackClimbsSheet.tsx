/**
 * **5 — Stack, with what happens.** The axis from 2, the join from 3.
 *
 * The stack's argument holds: height, ground and weather are all functions of
 * how far along the route you are, so one axis lets them align themselves and
 * a reader can see that the rain arrives on the second col and the col is
 * gravel. What it never said is anything a rider could *name*. Three cols are
 * three humps on a chart; they are not a list of things that happen, and half
 * of planning a ride is that list.
 *
 * So the climbs sit in a column beside the lanes, as they do in the split.
 * The worry about that was width — width is what makes the stack work, and a
 * sidebar takes some — but a narrow one leaves the lanes around a thousand
 * pixels at laptop size, which is more than the filmstrip had in the story
 * that decided it was worth using. Underneath, the same list was a row of
 * cards that pushed the sheet to nearly half the page.
 *
 * What this adds over the split is the axis. Each col is bracketed where it
 * actually falls, numbered, and the row in the sidebar carries the same
 * number — so the list and the chart are one reading rather than two that
 * happen to be about the same ride.
 */

import { ElevationProfile } from "../../../components/route/ElevationProfile";
import { FilmstripBand } from "../../../components/route/forecast-spike/FilmstripBand";
import { formatAscent, formatDistance, formatGradient } from "../../../lib/format";
import { PADDING } from "../../../lib/plotAxis";
import { useElementWidth } from "../../../lib/useElementWidth";
import { weatherIcon } from "../../../lib/weather";
import { groundSegments, Ribbon } from "../panel-spike/shared";
import type { SheetProps } from "./shared";
import { cellAt, clockAt, RideWindow, Sheet } from "./shared";

export function StackClimbsSheet({
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
  const { ref, width } = useElementWidth<HTMLDivElement>();

  return (
    <Sheet>
      <div className="flex items-start gap-4">
        <div className="min-w-0 flex-1">
          <div className="mb-2">
            <RideWindow startAt={startAt} samples={samples} />
          </div>
          <div ref={ref} className="grid gap-1.5">
            <ElevationProfile
              profile={profile}
              title={route.title}
              surface={surface}
              activeMetres={activeMetres}
              onActiveChange={onActiveChange}
              highlight={highlight}
              unitSystem={unitSystem}
            />
            <div style={{ paddingLeft: PADDING.left, paddingRight: PADDING.right }}>
              {/*
               * The cols bracketed on the shared axis, carrying the chart's own
               * gutters. A bracket a few pixels off from the hump above it would
               * put the col in the wrong place subtly, which is worse than
               * obviously.
               */}
              <div className="relative mb-1 h-3.5">
                {climbs.map((climb, index) => (
                  <span
                    key={climb.startMetres}
                    className="absolute top-2 h-1 rounded-full bg-[var(--ink-2)]"
                    style={{
                      left: `${(climb.startMetres / route.distanceMetres) * 100}%`,
                      width: `${((climb.endMetres - climb.startMetres) / route.distanceMetres) * 100}%`,
                    }}
                  >
                    <span className="absolute -top-2.5 left-0 text-[10px] leading-none font-semibold text-[var(--ink-2)] tabular-nums">
                      {index + 1}
                    </span>
                  </span>
                ))}
              </div>
              {/*
               * Ground only: the chart above already paints its area by steepness
               * band, and a gradient ribbon here would be the same fact drawn
               * twice a row apart.
               */}
              <Ribbon segments={groundSegments(surface)} className="h-3" highlight={highlight} />
            </div>
            <FilmstripBand
              cells={cells}
              width={width}
              startMetres={0}
              endMetres={route.distanceMetres}
              unitSystem={unitSystem}
            />
          </div>
        </div>
        <div className="w-[16.5rem] shrink-0 border-l border-[var(--rule)] pl-4">
          <h3 className="mb-1.5 text-[11px] font-semibold tracking-[0.06em] text-[var(--ink-2)] uppercase">
            What happens
          </h3>
          {climbs.length === 0 ? (
            <p className="text-xs text-[var(--ink-2)]">Nothing sustained enough to call a climb.</p>
          ) : (
            <ol className="grid gap-0.5">
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
                      className="w-full rounded-lg px-1.5 py-1 text-left hover:bg-[var(--base)] focus-visible:outline-2 focus-visible:outline-offset-[-2px] focus-visible:outline-[var(--accent)]"
                    >
                      <span className="flex items-baseline gap-1.5">
                        {/*
                         * The same ordinal the bracket over the chart carries.
                         * It is what makes this a reading of the axis beside it
                         * rather than a second list about the same ride.
                         */}
                        <span className="text-xs font-semibold tabular-nums">{index + 1}</span>
                        <span className="text-xs tabular-nums">
                          {formatDistance(climb.distanceMetres, unitSystem)} at{" "}
                          {formatGradient(climb.averageGradePercent)}
                        </span>
                        {cell === null ? null : (
                          <span className="ml-auto flex items-center gap-1 text-xs tabular-nums">
                            {Glyph === null ? null : (
                              <Glyph size={14} stroke={1.8} aria-hidden="true" />
                            )}
                            <span className="font-semibold">
                              {Math.round(cell.point.temperatureCelsius)}°
                            </span>
                          </span>
                        )}
                      </span>
                      <span className="mt-0.5 block text-[11px] text-[var(--ink-2)] tabular-nums">
                        {formatAscent(climb.ascentMetres, unitSystem)} · from{" "}
                        {formatDistance(climb.startMetres, unitSystem)}
                        {cell === null ? null : ` · ${clockAt(cell.sample.arrivalAt)}`}
                      </span>
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
