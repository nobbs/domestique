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
 *
 * The list scrolls rather than growing the sheet. A route with seven climbs
 * has more to say than one with three, but it should not therefore cover more
 * of the map: the brackets on the axis show every col whatever the column can
 * fit, so nothing is hidden, only queued.
 */

import { ElevationProfile } from "../../../components/route/ElevationProfile";
import { FilmstripBand } from "../../../components/route/forecast-spike/FilmstripBand";
import { formatAscent, formatDistance, formatGradient } from "../../../lib/format";
import { PADDING } from "../../../lib/plotAxis";
import { useElementWidth } from "../../../lib/useElementWidth";
import { weatherIcon } from "../../../lib/weather";
import { groundSegments } from "../panel-spike/shared";
import { ClimbMarkers } from "./ClimbMarkers";
import { LabelledRibbon } from "./LabelledRibbon";
import type { SheetProps } from "./shared";
import { cellAt, clockAt, RideWindow, Sheet } from "./shared";
import type { WeatherFrameVariant } from "./WeatherFrame";
import { WeatherFrame } from "./WeatherFrame";

export function StackClimbsSheet({
  weatherFrame = "card",
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
  onHighlightChange,
  unitSystem,
}: SheetProps & { weatherFrame?: WeatherFrameVariant }) {
  const { ref, width } = useElementWidth<HTMLDivElement>();

  return (
    <Sheet>
      <div className="flex items-stretch gap-4">
        <div className="min-w-0 flex-1">
          <div className="mb-2">
            <RideWindow startAt={startAt} samples={samples} />
          </div>
          <div ref={ref} className="grid gap-1.5">
            <div className="relative">
              <ElevationProfile
                profile={profile}
                title={route.title}
                surface={surface}
                activeMetres={activeMetres}
                onActiveChange={onActiveChange}
                highlight={highlight}
                unitSystem={unitSystem}
              />
              <ClimbMarkers
                climbs={climbs}
                totalMetres={route.distanceMetres}
                onSelect={onActiveChange}
              />
            </div>
            {/*
             * Ground only: the chart above already paints its area by
             * steepness band, and a gradient ribbon here would be the same
             * fact drawn twice a row apart.
             */}
            <div style={{ paddingLeft: PADDING.left, paddingRight: PADDING.right }}>
              <LabelledRibbon
                segments={groundSegments(surface)}
                surface={surface}
                highlight={highlight}
                onHighlightChange={onHighlightChange}
              />
            </div>
            <WeatherFrame variant={weatherFrame} caption="Forecast · every 30 min of riding">
              <FilmstripBand
                cells={cells}
                width={width}
                startMetres={0}
                endMetres={route.distanceMetres}
                unitSystem={unitSystem}
              />
            </WeatherFrame>
          </div>
        </div>
        {/*
         * The list is taken out of the flow so the lanes decide how tall the
         * sheet is. Seven climbs are a taller column than the chart, the
         * ribbon and the filmstrip put together, and left in the flow they
         * grow the panel by ninety pixels of map — which puts the amount of
         * terrain a reader loses in the hands of how lumpy the route is.
         *
         * Absolute rather than a max-height, because the height to match is
         * the lanes' own and that is not a number anything here knows.
         */}
        <div className="relative w-[16.5rem] shrink-0 border-l border-[var(--rule)] pl-4">
          <div className="absolute inset-y-0 right-0 left-4 overflow-y-auto">
            <h3 className="mb-1.5 text-[11px] font-semibold tracking-[0.06em] text-[var(--ink-2)] uppercase">
              What happens
            </h3>
            {climbs.length === 0 ? (
              <p className="text-xs text-[var(--ink-2)]">
                Nothing sustained enough to call a climb.
              </p>
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
      </div>
    </Sheet>
  );
}
