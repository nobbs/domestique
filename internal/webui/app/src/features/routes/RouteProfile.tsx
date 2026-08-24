/**
 * The profile, inside the route's own card.
 *
 * It used to float across the foot of the map on a panel of its own. It does
 * not any more: the climb is one of the things this route *is*, alongside the
 * four figures above it and the two mixes below, and a panel on the far side of
 * the map made a reader look in two places to read one route. Inside the card
 * it sits between the figures it elaborates and the gradient bar it explains,
 * and the foot of the map goes back to being map.
 *
 * The chart inside is the same instrument it has always been — scrub, drag to
 * zoom, arrow keys, readout — and this only puts a header row over it.
 *
 * The chevron in that row folds the chart away, leaving the row: collapsed, it
 * still carries the two figures the chart existed to give at a glance, between
 * which heights the route runs and how much climbing that comes to. The choice
 * sticks as the reader moves between routes, because a reader who put the chart
 * away did so to see more of the card, not to see more of one route's card.
 *
 * The start-time control and the forecast strip live in the same row of the
 * card as the chart, immediately under it — and fold away with it, since a
 * forecast for a chart the reader has already put away is answering a
 * question they are not asking any more.
 */

import type { Position } from "../../api/types";
import { ElevationProfile } from "../../components/ElevationProfile";
import { ForecastStrip } from "../../components/ForecastStrip";
import { StartTimePicker } from "../../components/StartTimePicker";
import type { ForecastSample } from "../../lib/forecastSamples";
import { formatAscent, formatElevation } from "../../lib/format";
import type { Highlight } from "../../lib/highlight";
import { useCoarsePointer } from "../../lib/mediaQuery";
import type { DistanceWindow, Profile } from "../../lib/profile";
import { isWithinForecastWindow } from "../../lib/startTime";
import type { SurfaceSummary } from "../../lib/surface";
import type { UnitSystem } from "../../lib/units";
import { distanceUnitLabel, distanceValue } from "../../lib/units";

export interface RouteProfileProps {
  /** The stretch on show: the whole route, or the window the reader chose. */
  profile: Profile | null;
  title: string;
  /** How much climbing the whole route has, for the collapsed row. */
  ascentMetres: number;
  surface: SurfaceSummary | null;
  activeMetres: number | null;
  onActiveChange: (metres: number | null) => void;
  zoomWindow: DistanceWindow | null;
  onZoomChange: (window: DistanceWindow | null) => void;
  highlight: Highlight | null;
  collapsed: boolean;
  onCollapsedChange: (collapsed: boolean) => void;
  /** The units the figures, the summary, and the chart itself report in. */
  unitSystem: UnitSystem;
  /** When the reader means to set off, or null while nothing has been chosen. */
  startAt: Date | null;
  onStartAtChange: (next: Date | null) => void;
  /** The forecast samples for the whole route — see `forecastSamples.ts`. */
  samples: ForecastSample[];
  /** The whole route's own geometry, which the forecast strip's wind reading is measured against. */
  coordinates: Position[];
  /**
   * The stage's whole predicted moving time, or undefined for a stage nothing
   * has predicted one for. Absent is a state the page has to say out loud
   * rather than draw as an empty space: the reader has asked for a forecast
   * and is owed the reason they are not getting one.
   */
  rideSeconds?: number | undefined;
  /**
   * Whether the stage's prediction is settled — that is, whether its geometry
   * has actually been answered for.
   *
   * `rideSeconds` alone cannot say: it is undefined both while the geometry is
   * still being fetched and when the answer came back without a prediction. A
   * remembered start time makes that difference visible, because the page
   * would otherwise announce "no predicted moving time" for the second or two
   * a fetch takes, on a stage that has one.
   */
  predictionKnown?: boolean;
}

export function RouteProfile({
  profile,
  title,
  ascentMetres,
  surface,
  activeMetres,
  onActiveChange,
  zoomWindow,
  onZoomChange,
  highlight,
  collapsed,
  onCollapsedChange,
  unitSystem,
  startAt,
  onStartAtChange,
  samples,
  coordinates,
  rideSeconds,
  predictionKnown = true,
}: RouteProfileProps) {
  /*
   * A finger cannot hover, and a card that scrolls cannot give every downward
   * swipe over the chart to the chart. So on a touch pointer the gesture is
   * armed by holding rather than by landing, and the hint says which of the two
   * this reader has.
   */
  const coarse = useCoarsePointer();
  /*
   * A remembered start time is checked again here, not only where it was
   * typed. It can go stale on a page left open past the endpoint's 24-hour
   * allowance, and a time that fits a short stage can put a long one's finish
   * past the 16-day horizon — the picker's own bounds say nothing about a
   * value it was handed rather than asked for. Sending it anyway would earn a
   * `400` that the strip can only report as the provider being unavailable,
   * which blames Open-Meteo for arithmetic done here.
   */
  const startFits =
    startAt !== null &&
    isWithinForecastWindow(startAt) &&
    isWithinForecastWindow(new Date(startAt.getTime() + Math.max(rideSeconds ?? 0, 0) * 1000));
  const range = profile
    ? `${formatElevation(profile.minElevationMetres, unitSystem)}–${formatElevation(profile.maxElevationMetres, unitSystem)}`
    : "";
  // Collapsed, the row is the summary; open, the same line is the hint that
  // says what the chart will do if it is dragged across.
  const summary = collapsed
    ? [range, ascentMetres > 0 ? `${formatAscent(ascentMetres, unitSystem)} up` : ""]
        .filter(Boolean)
        .join(" · ")
    : zoomWindow
      ? `${distanceValue(zoomWindow.startMetres, unitSystem).toFixed(1)}–${distanceValue(zoomWindow.endMetres, unitSystem).toFixed(1)} ${distanceUnitLabel(unitSystem)} shown · Escape returns`
      : range === ""
        ? ""
        : `${range} · ${coarse ? "press and hold to look closer" : "drag across to look closer"}`;

  return (
    <section className="route-profile" aria-labelledby="elevation-heading">
      <div className="route-profile__header">
        <h3 id="elevation-heading">Elevation</h3>
        {summary === "" ? null : <span className="route-profile__summary">{summary}</span>}
        <button
          className="route-profile__collapse"
          type="button"
          aria-expanded={!collapsed}
          // The words the button used to carry. A chevron at the end of a header
          // row is read as "this folds away" by anyone who can see it, and the
          // sentence is still there for anyone who cannot.
          aria-label={collapsed ? "Show the profile" : "Hide the profile"}
          // Only while there is a plot to point at: the chart is unmounted when
          // the section is a row, and a control naming an element that is not in
          // the document is a dangling reference a screen reader cannot follow.
          {...(collapsed ? {} : { "aria-controls": "elevation-plot" })}
          onClick={() => onCollapsedChange(!collapsed)}
        >
          {/*
           * One chevron, turned rather than swapped: the same mark rotating
           * through the change says the chart folded away, where two paths
           * cutting from one to the other would only say it is gone.
           */}
          <svg
            className="route-profile__chevron"
            viewBox="0 0 12 12"
            width="12"
            height="12"
            fill="none"
            stroke="currentColor"
            strokeWidth="1.6"
            strokeLinecap="round"
            strokeLinejoin="round"
            aria-hidden="true"
            focusable="false"
          >
            <polyline points="2.5,4.5 6,8 9.5,4.5" />
          </svg>
        </button>
      </div>
      {/*
       * Unmounted rather than hidden when the section is a row: the chart holds
       * a pointer listener over its whole plot area, and a plot nobody can see
       * must not be a plot a stray drag can still select a stretch of.
       */}
      {collapsed ? null : (
        <>
          <div id="elevation-plot">
            <ElevationProfile
              profile={profile}
              title={title}
              surface={surface}
              activeMetres={activeMetres}
              onActiveChange={onActiveChange}
              zoomWindow={zoomWindow}
              onZoomChange={onZoomChange}
              highlight={highlight}
              unitSystem={unitSystem}
            />
          </div>
          <StartTimePicker value={startAt} onChange={onStartAtChange} rideSeconds={rideSeconds} />
          {startAt && predictionKnown && rideSeconds === undefined ? (
            <p className="route-profile__unpredicted">
              This stage has no predicted moving time, so there is no timeline to hang a forecast
              on.
            </p>
          ) : null}
          {startAt && rideSeconds !== undefined && !startFits ? (
            <p className="route-profile__unpredicted">
              That start time is outside the 16-day forecast window for this stage — this ride would
              finish past it. Choose another to see a forecast.
            </p>
          ) : null}
          {startAt && rideSeconds !== undefined && startFits ? (
            <ForecastStrip
              samples={samples}
              coordinates={coordinates}
              startMetres={profile?.startMetres ?? 0}
              /*
               * Without a profile there is no chart to share an axis with, so
               * the strip falls back to the whole route — the last sample sits
               * at the finish, so it carries that distance. Zero would be a
               * window nothing overlaps, and every cell would be dropped as
               * off-screen: a stage with a timeline but no terrain is exactly
               * the case that is supposed to still get a forecast.
               */
              endMetres={profile?.endMetres ?? samples[samples.length - 1]?.distanceMetres ?? 0}
              unitSystem={unitSystem}
            />
          ) : null}
        </>
      )}
    </section>
  );
}
