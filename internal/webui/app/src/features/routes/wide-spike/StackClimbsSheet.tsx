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

import { IconChevronsRight } from "@tabler/icons-react";
import type { ReactNode } from "react";
import { useState } from "react";
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
  lead,
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
}: SheetProps & {
  weatherFrame?: WeatherFrameVariant;
  /**
   * What the route is, where a composition puts it in the dock rather than in
   * a card of its own. Absent leaves the sheet as it was.
   */
  lead?: ReactNode;
}) {
  const { ref, width } = useElementWidth<HTMLDivElement>();
  /*
   * Held here rather than by each part, and local rather than remembered. In
   * the real panel these would stick across routes for the reason
   * `RouteProfile` already gives about its chart: a reader who put something
   * away did so to see more of everything else, not more of one route's
   * everything else.
   */
  const [climbsOpen, setClimbsOpen] = useState(true);
  const [forecastOpen, setForecastOpen] = useState(true);
  const [groundLabelled, setGroundLabelled] = useState(true);

  return (
    <Sheet>
      <div className="flex items-stretch gap-4">
        {lead === undefined ? null : (
          <div className="w-[13.5rem] shrink-0 border-r border-[var(--rule)] pr-4">{lead}</div>
        )}
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
            <div className="relative">
              {/*
               * The fold sits in the chart's left gutter, which is reserved
               * for the metre axis and is empty at this row. A control on the
               * ribbon's own line would either push it out of step with the
               * chart above or cover the first kilometres of ground.
               */}
              <button
                type="button"
                aria-expanded={groundLabelled}
                aria-label={groundLabelled ? "Hide the ground key" : "Show the ground key"}
                onClick={() => setGroundLabelled(!groundLabelled)}
                className="absolute top-0 left-0 rounded p-0.5 text-[var(--ink-2)] hover:bg-[var(--base)] hover:text-[var(--ink)] focus-visible:outline-2 focus-visible:outline-offset-1 focus-visible:outline-[var(--accent)]"
              >
                <IconChevronsRight
                  size={12}
                  stroke={2}
                  aria-hidden="true"
                  className={
                    groundLabelled ? "rotate-90 transition-transform" : "transition-transform"
                  }
                />
              </button>
              <div style={{ paddingLeft: PADDING.left, paddingRight: PADDING.right }}>
                <LabelledRibbon
                  segments={groundSegments(surface)}
                  surface={surface}
                  labelled={groundLabelled}
                  highlight={highlight}
                  onHighlightChange={onHighlightChange}
                />
              </div>
            </div>
            <WeatherFrame
              variant={weatherFrame}
              caption="Forecast · every 30 min of riding"
              open={forecastOpen}
              onOpenChange={setForecastOpen}
            >
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
        {/*
         * The rule between the lanes and the list, with the control sitting on
         * it. The divider is the seam the column folds along, so the handle
         * belongs on the seam rather than inside either side — in the heading
         * it disappeared along with the thing it opens, which put the way back
         * somewhere else entirely.
         *
         * A column of its own, so it stays put whether the list is there or
         * not: the rule is the one part of this that does not move.
         */}
        <div className="relative w-px shrink-0 self-stretch bg-[var(--rule)]">
          <button
            type="button"
            aria-expanded={climbsOpen}
            aria-label={climbsOpen ? "Hide what happens" : `Show ${climbs.length} climbs`}
            title={climbsOpen ? "Hide what happens" : "What happens"}
            onClick={() => setClimbsOpen(!climbsOpen)}
            className="absolute top-1/2 left-1/2 -translate-x-1/2 -translate-y-1/2 rounded-full border border-[var(--rule)] bg-[var(--panel)] p-0.5 text-[var(--ink-2)] hover:bg-[var(--base)] hover:text-[var(--ink)] focus-visible:outline-2 focus-visible:outline-offset-1 focus-visible:outline-[var(--accent)]"
          >
            {/* Pointing the way the column will go: right to push it away, left to pull it back. */}
            <IconChevronsRight
              size={12}
              stroke={2}
              aria-hidden="true"
              className={climbsOpen ? "transition-transform" : "rotate-180 transition-transform"}
            />
          </button>
        </div>
        {!climbsOpen ? null : (
          <div className="relative w-[16.5rem] shrink-0">
            <div className="absolute inset-0 overflow-y-auto">
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
        )}
      </div>
    </Sheet>
  );
}
