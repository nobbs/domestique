/**
 * One route: the map, the four figures, and the profile under them.
 *
 * Read-only by design. There are deliberately no editing affordances here, and
 * the service writes nothing back to VeloPlanner — the two actions in the header
 * either leave for the provider or ask this service to work the route out again.
 *
 * The profile is always on the page. It used to be a `<details>` with the choice
 * remembered, which made the elevation of a ride something a reader could put
 * away and then have to remember they had put away; the figures and the climb
 * are the same answer, so they are shown together.
 */

import { useQuery } from "@tanstack/react-query";
import { lazy, Suspense, useCallback, useMemo, useState } from "react";
import { Link, useParams } from "react-router";
import { ApiError } from "../../api/client";
import { routeGeometryQuery, statusQuery, webUIConfigQuery } from "../../api/queries";
import type { Position, Route, RouteSurface } from "../../api/types";
import { ElevationProfile } from "../../components/ElevationProfile";
import { RouteKey } from "../../components/RouteKey";
import { SourceRouteLink } from "../../components/SourceRouteLink";
import { ErrorMessage, LoadingMessage, StatusMessage } from "../../components/StatusMessage";
import { Wordmark } from "../../components/Wordmark";
import { type Basemap, basemapFor, usePrefersDarkScheme } from "../../lib/basemap";
import {
  formatAscent,
  formatDistance,
  formatElevation,
  formatGradient,
  formatTimestamp,
} from "../../lib/format";
import type { Highlight } from "../../lib/highlight";
import { highlightLabel } from "../../lib/highlight";
import type { DistanceWindow } from "../../lib/profile";
import {
  buildProfile,
  buildWindowedProfile,
  gradientRanges,
  presentBands,
} from "../../lib/profile";
import { summariseSurface } from "../../lib/surface";
import { ReprocessButton } from "./ReprocessButton";

// MapLibre is by far the heaviest dependency, and the entry page loads it too —
// but as a separate chunk, so a deep link straight to a route fetches it while
// the rest of this page is already being read.
const RouteMap = lazy(async () => ({
  default: (await import("../../components/RouteMap")).RouteMap,
}));

function positiveInteger(value: string | undefined): number | undefined {
  if (value === undefined || !/^\d+$/.test(value)) {
    return undefined;
  }
  const parsed = Number.parseInt(value, 10);

  return parsed > 0 ? parsed : undefined;
}

/**
 * The page's frame: the wordmark, the way back, and the two quiet actions.
 *
 * Everything the route page can be — the route itself, a failure, a route that
 * has no geometry yet — is this header with something different under it, so
 * none of those states loses the way back to the map.
 */
function RoutePage({
  actions,
  children,
}: {
  actions?: React.ReactNode;
  children: React.ReactNode;
}) {
  return (
    <main className="route-page">
      <header className="route-page__header">
        <Wordmark />
        <Link to="/" className="route-page__back">
          ← All routes
        </Link>
        {actions ? <div className="route-page__actions">{actions}</div> : null}
      </header>
      <div className="route-page__body">{children}</div>
    </main>
  );
}

export function RouteDetail() {
  const params = useParams();
  const routeId = positiveInteger(params.routeId);
  const stageOrder = positiveInteger(params.stage);
  const enabled = routeId !== undefined && stageOrder !== undefined;

  const config = useQuery(webUIConfigQuery());
  const prefersDark = usePrefersDarkScheme();
  const geometry = useQuery({
    ...routeGeometryQuery(routeId ?? 0, stageOrder ?? 0),
    enabled,
  });

  if (!enabled) {
    return (
      <RoutePage>
        <StatusMessage tone="error" title="That is not a valid route address." />
      </RoutePage>
    );
  }
  if (geometry.isError && geometry.error instanceof ApiError && geometry.error.isNotFound) {
    return (
      <RoutePage>
        <StatusMessage
          title="No geometry for this route yet."
          detail="It appears after the next successful read of the library stores it."
        />
      </RoutePage>
    );
  }
  if (geometry.isError) {
    return (
      <RoutePage>
        <ErrorMessage what="the route geometry" error={geometry.error} />
      </RoutePage>
    );
  }
  if (config.isError) {
    return (
      <RoutePage>
        <ErrorMessage what="the map configuration" error={config.error} />
      </RoutePage>
    );
  }
  if (geometry.isPending || config.isPending) {
    return (
      <RoutePage>
        <LoadingMessage what="the route" />
      </RoutePage>
    );
  }

  const { stage, coordinates, bbox, surface } = geometry.data;

  return (
    <RouteView
      route={stage}
      routeId={routeId}
      coordinates={coordinates}
      bbox={bbox}
      surface={surface}
      basemap={basemapFor(config.data, prefersDark)}
      sourceBaseUrl={config.data?.sourceBaseUrl}
    />
  );
}

/**
 * The map and the profile over one shared position.
 *
 * The hovered position lives here rather than in either view, because the two
 * are one instrument: pointing at the route marks the chart, and scrubbing the
 * chart marks the route. It is a distance in metres from the start of the route
 * — the one unit that means the same ground to a map, to the whole profile, and
 * to a chart showing two kilometres of it.
 *
 * The zoomed stretch and the highlighted class live here for the same reason:
 * both are questions asked of the ride rather than of either view.
 */
function RouteView({
  route,
  routeId,
  coordinates,
  bbox,
  surface,
  basemap,
  sourceBaseUrl,
}: {
  route: Route;
  routeId: number;
  coordinates: Position[];
  bbox: [number, number, number, number];
  surface: RouteSurface | undefined;
  basemap: Basemap;
  /** The provider's web application, for the way back to the source route. */
  sourceBaseUrl: string | undefined;
}) {
  const status = useQuery(statusQuery());
  const routeProfile = useMemo(() => buildProfile(coordinates), [coordinates]);
  const [activeMetres, setActiveMetres] = useState<number | null>(null);
  const [zoomWindow, setZoomWindow] = useState<DistanceWindow | null>(null);
  const [highlight, setHighlight] = useState<Highlight | null>(null);

  // Rebuilt from the original geometry rather than from the last window, so
  // zooming inside a zoom compounds no rounding error and needs no stack.
  const windowed = useMemo(
    () => (zoomWindow ? buildWindowedProfile(coordinates, zoomWindow) : null),
    [coordinates, zoomWindow],
  );
  // A window that built nothing is a slip, not a view: the map must not dim
  // around a stretch the chart is not showing.
  const shownWindow = windowed ? zoomWindow : null;

  // The position was chosen against the view being left, so it goes with it.
  // Both instruments hand a window over the same way: a drag across the chart
  // and a drag along the route are one question asked in two places.
  const onZoomChange = useCallback((next: DistanceWindow | null) => {
    setZoomWindow(next);
    setActiveMetres(null);
  }, []);

  // The route's steepness, classified from the coordinates the service stored
  // rather than from any resampling of them. Held here so the key does not
  // re-run the classification on every hover.
  const gradient = useMemo(() => gradientRanges(coordinates), [coordinates]);

  // A classification that snapped to nothing is left unpainted rather than drawn
  // as unsurveyed from end to end: greying out the whole route to say nothing is
  // known says it less clearly than one sentence does.
  const surfaceSummary = useMemo(
    () =>
      surface && surface.matchedMetres > 0 ? summariseSurface(coordinates, surface.ranges) : null,
    [coordinates, surface],
  );

  /*
   * Where the route is, when it was read, and whether the accounts have it.
   *
   * The service stores no locality, so "where" is the operator's own name for
   * the route wherever that is not already the title — asking a geocoder would
   * send the library's coordinates outside the Tailnet to answer a question the
   * naming already answers. The accounts are only mentioned when every one of
   * them holds the whole library, because anything short of that is a statement
   * about the library rather than about this route.
   */
  const readAt = status.data?.sync.phases.source?.lastCompletedAt;
  const subtitle = [
    route.routeName !== route.title ? route.routeName : null,
    readAt ? `read ${formatTimestamp(readAt)}` : null,
    status.data?.converged ? "on every account" : null,
  ]
    .filter(Boolean)
    .join(" · ");

  const profile = windowed ?? routeProfile;
  const hint = [
    shownWindow
      ? `${(shownWindow.startMetres / 1000).toFixed(1)}–${(shownWindow.endMetres / 1000).toFixed(1)} km shown · Escape returns`
      : profile
        ? `${formatElevation(profile.minElevationMetres)}–${formatElevation(profile.maxElevationMetres)} · drag across to look closer`
        : "",
    highlight ? `${highlightLabel(highlight)} only` : "",
  ]
    .filter(Boolean)
    .join(" · ");

  return (
    <RoutePage
      actions={
        <>
          {/*
           * The way back to the route this was made from, which is also the only
           * place anything about it can be changed. Absent when the service
           * cannot name the provider.
           */}
          <SourceRouteLink baseUrl={sourceBaseUrl} routeId={routeId} />
          <ReprocessButton routeId={routeId} stageOrder={route.stageOrder} />
        </>
      }
    >
      <div className="route-page__map">
        <Suspense fallback={<LoadingMessage what="the map" />}>
          <RouteMap
            styleUrl={basemap.styleUrl}
            darkBasemap={basemap.dark}
            coordinates={coordinates}
            bbox={bbox}
            title={route.title}
            surface={surfaceSummary ? surface?.ranges : undefined}
            profile={routeProfile}
            activeMetres={activeMetres}
            onActiveChange={setActiveMetres}
            zoomWindow={shownWindow}
            onZoomChange={onZoomChange}
            highlight={highlight}
          />
        </Suspense>
      </div>
      <div className="panel route-page__facts">
        <h1 className="route-page__title">{route.title}</h1>
        {subtitle === "" ? null : <p className="route-page__subtitle">{subtitle}</p>}
        <dl className="route-page__figures">
          <div>
            <dt>Distance</dt>
            <dd>{formatDistance(route.distanceMetres)}</dd>
          </div>
          <div>
            <dt>Climbing</dt>
            <dd>{formatAscent(route.ascentMetres)}</dd>
          </div>
          <div>
            <dt>Max gradient</dt>
            <dd>{formatGradient(route.maxGradientPercent)}</dd>
          </div>
          <div>
            <dt>Highest</dt>
            {/*
             * Of the whole route, never of the stretch on show: it is one of the
             * four things this route is, and a figure that changed while a
             * reader dragged across the chart would be a fifth instrument.
             */}
            <dd>{routeProfile ? formatElevation(routeProfile.maxElevationMetres) : "—"}</dd>
          </div>
        </dl>
      </div>
      <section className="panel route-page__panel" aria-labelledby="elevation-heading">
        <div className="route-page__panel-heading">
          <h2 id="elevation-heading">Elevation</h2>
          {hint === "" ? null : <span className="route-page__panel-hint">{hint}</span>}
        </div>
        <ElevationProfile
          profile={profile}
          title={route.title}
          surface={surfaceSummary}
          activeMetres={activeMetres}
          onActiveChange={setActiveMetres}
          zoomWindow={shownWindow}
          onZoomChange={onZoomChange}
          highlight={highlight}
        />
        {/*
         * The key sits under the marks it explains, not up beside the title: a
         * legend away from its marks is a lookup, and this one is meant to be
         * read in the same glance as the ground it names — and clicked in the
         * same glance, which a lookup would make a journey.
         */}
        <RouteKey
          surface={surfaceSummary}
          surfaceAbsence={
            surface
              ? "No OpenStreetMap surface data along this route."
              : "Surface not classified yet."
          }
          bands={presentBands(gradient)}
          highlight={highlight}
          onHighlightChange={setHighlight}
        />
      </section>
    </RoutePage>
  );
}
