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
 * The split answered that with a column beside the chart, which cost the lanes
 * a third of their width — and width is exactly what makes the stack work: at
 * two thirds of the sheet the filmstrip is back to tiles too narrow to hold a
 * reading. So the climbs go *underneath* instead, and become a lane of their
 * own rather than a sidebar.
 *
 * They keep the axis. Each col is bracketed where it falls, numbered, and the
 * card carrying its figures and the weather waiting on it carries the same
 * number. Numbered rather than positioned, because a card is two hundred
 * pixels wide and a col is a tenth of the route: placing cards under their own
 * brackets works for three and collides for eight, where a number works for
 * both.
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
      {climbs.length === 0 ? null : (
        <ul className="mt-2.5 grid gap-2 [grid-template-columns:repeat(auto-fill,minmax(15rem,1fr))]">
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
                  className="w-full rounded-lg bg-[var(--base)] px-2.5 py-1.5 text-left hover:ring-1 hover:ring-[var(--rule)] focus-visible:outline-2 focus-visible:outline-offset-[-2px] focus-visible:outline-[var(--accent)]"
                >
                  <span className="flex items-baseline gap-1.5">
                    <span className="text-xs font-semibold tabular-nums">{index + 1}</span>
                    <span className="text-xs font-semibold tabular-nums">
                      {formatDistance(climb.distanceMetres, unitSystem)} at{" "}
                      {formatGradient(climb.averageGradePercent)}
                    </span>
                    {cell === null ? null : (
                      <span className="ml-auto flex items-center gap-1 text-xs tabular-nums">
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
                  </span>
                  <span className="mt-0.5 block text-[11px] text-[var(--ink-2)] tabular-nums">
                    {formatAscent(climb.ascentMetres, unitSystem)} · from{" "}
                    {formatDistance(climb.startMetres, unitSystem)}
                  </span>
                </button>
              </li>
            );
          })}
        </ul>
      )}
    </Sheet>
  );
}
