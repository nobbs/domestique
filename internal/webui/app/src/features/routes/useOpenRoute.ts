/**
 * Everything asked of the open route, in one place: the map and the panel are
 * two views of one answer, so neither holds its own copy. `AtlasPage` calls it.
 */

import type { UseQueryResult } from "@tanstack/react-query";
import { useCallback, useMemo, useState } from "react";
import type { Position, RouteGeometry } from "../../api/types";
import type { Climb } from "../../lib/climbs";
import { findClimbs } from "../../lib/climbs";
import { forecastSamples } from "../../lib/forecastSamples";
import type { Highlight } from "../../lib/highlight";
import { highlightSpans, nextSpan, sameHighlight } from "../../lib/highlight";
import type { MeasureKey } from "../../lib/measures";
import type { DistanceWindow } from "../../lib/profile";
import {
  buildProfile,
  buildWindowedProfile,
  gradientShares,
  gradientSummary,
  movingSecondsForWindow,
} from "../../lib/profile";
import { widened } from "../../lib/selection";
import { summariseSurface } from "../../lib/surface";

/**
 * The questions asked of one open route, and their answers.
 *
 * @param coordinates The route's stored geometry, empty until it arrives.
 * @param geometry The same fetch the library map draws from, read for the
 *   surface classification and the predicted cumulative seconds.
 * @param startAt The reader's chosen start time, null while none is picked.
 */
export function useOpenRoute(
  coordinates: Position[],
  geometry: UseQueryResult<RouteGeometry>,
  startAt: Date | null,
) {
  const [activeMetres, setActiveMetres] = useState<number | null>(null);
  const [zoomWindow, setZoomWindow] = useState<DistanceWindow | null>(null);
  const [highlight, setHighlight] = useState<Highlight | null>(null);
  /*
   * Which forecast measure the map is washed in, and null — off — to begin
   * with: the wash covers ground, and covering it before anybody asked is an
   * overlay in the way of the map. Deliberately not cleared by `forget`,
   * unlike the highlight above it: a highlight names this route's own ground,
   * while "show me the rain" is a question about the weather that stays asked
   * when the reader opens the next route.
   */
  const [measure, setMeasure] = useState<MeasureKey | null>(null);
  const [chartCollapsed, setChartCollapsed] = useState(false);

  const routeProfile = useMemo(() => buildProfile(coordinates), [coordinates]);
  // Samples for the forecast strip: nothing until the reader has chosen a
  // start time, and nothing for a route nothing has predicted a moving time
  // for — `forecastSamples` itself returns `[]` for either, which the strip
  // reads as nothing to draw.
  const samples = useMemo(
    () => (startAt ? forecastSamples(coordinates, geometry.data?.cumulativeSeconds, startAt) : []),
    [coordinates, geometry.data?.cumulativeSeconds, startAt],
  );
  // The whole route's predicted moving time, and undefined for a route nothing
  // has predicted. It bounds how late a start the picker may offer — the
  // forecast has to reach the arrival, not just the departure — and it is what
  // tells the page to explain an absent strip rather than draw nothing.
  const cumulativeSeconds = geometry.data?.cumulativeSeconds;
  const movingSeconds = cumulativeSeconds?.[cumulativeSeconds.length - 1];
  // Rebuilt from the original geometry rather than from the last window, so
  // zooming inside a zoom compounds no rounding error and needs no stack.
  const windowed = useMemo(
    () => (zoomWindow ? buildWindowedProfile(coordinates, zoomWindow) : null),
    [coordinates, zoomWindow],
  );
  // A window that built nothing is a slip, not a view: the map must not dim
  // around a stretch the chart is not showing.
  const shownWindow = windowed ? zoomWindow : null;
  // The moving time for the stretch on show, read off the same cumulative
  // series the whole-route figure comes from. Undefined restores the
  // whole-route figure — clearing the selection, or a route nothing has
  // predicted, both fall back the same way.
  const selectionMovingSeconds = useMemo(
    () => movingSecondsForWindow(coordinates, cumulativeSeconds, shownWindow),
    [coordinates, cumulativeSeconds, shownWindow],
  );

  // The position was chosen against the view being left, so it goes with it.
  const onZoomChange = useCallback((next: DistanceWindow | null) => {
    setZoomWindow(next);
    setActiveMetres(null);
  }, []);

  /** Puts every question asked of a route away with the route itself. */
  const forget = useCallback(() => {
    setActiveMetres(null);
    setZoomWindow(null);
    setHighlight(null);
  }, []);

  // The route's steepness, classified from the coordinates the service stored
  // rather than from any resampling of them, and totalled per band. Held here
  // so the chips do not re-run the classification on every hover.
  const gradient = useMemo(() => gradientShares(coordinates), [coordinates]);

  // How steep the route is each way, from the same stored coordinates.
  const gradients = useMemo(() => gradientSummary(coordinates), [coordinates]);

  // The route's sustained climbs, from the same stored coordinates.
  const climbs = useMemo(() => findClimbs(coordinates), [coordinates]);

  // A climb picked from the list opens the same shared window the chart's own
  // drag-to-zoom gesture opens, widened the same way a short drag is: a
  // hundred-metre climb is still worth a window big enough to plot.
  const selectClimb = useCallback(
    (climb: Climb) => {
      onZoomChange(
        widened(
          { startMetres: climb.startMetres, endMetres: climb.endMetres },
          routeProfile?.totalDistanceMetres ?? 0,
        ),
      );
    },
    [onZoomChange, routeProfile],
  );

  // A classification that snapped to nothing is left unpainted rather than
  // drawn as unsurveyed from end to end: greying out the whole route to say
  // nothing is known says it less clearly than one sentence does.
  const surface = geometry.data?.surface;
  const surfaceSummary = useMemo(
    () =>
      surface && surface.matchedMetres > 0 ? summariseSurface(coordinates, surface.ranges) : null,
    [coordinates, surface],
  );

  // A plain click on a chip sets the highlight and frames its first stretch;
  // clicking the same chip again steps to the next one, wrapping past the
  // last. `null` gives the whole route back, both the highlight and the zoom.
  const scopeHighlight = useCallback(
    (next: Highlight | null) => {
      if (!next) {
        setHighlight(null);
        onZoomChange(null);
        return;
      }
      const spans = highlightSpans(coordinates, surfaceSummary, next);
      const span = nextSpan(spans, sameHighlight(highlight, next) ? zoomWindow : null);
      setHighlight(next);
      if (span) {
        // The profile is null on routes without complete elevation, where
        // surface spans still exist; a total of 0 would collapse the window.
        const total =
          routeProfile?.totalDistanceMetres ?? surfaceSummary?.totalMetres ?? span.endMetres;
        onZoomChange(widened(span, total));
      }
    },
    [coordinates, surfaceSummary, highlight, zoomWindow, onZoomChange, routeProfile],
  );

  return {
    activeMetres,
    setActiveMetres,
    highlight,
    setHighlight,
    measure,
    setMeasure,
    chartCollapsed,
    setChartCollapsed,
    routeProfile,
    samples,
    movingSeconds,
    windowed,
    shownWindow,
    selectionMovingSeconds,
    onZoomChange,
    forget,
    gradient,
    gradients,
    climbs,
    selectClimb,
    surface,
    surfaceSummary,
    scopeHighlight,
  };
}
