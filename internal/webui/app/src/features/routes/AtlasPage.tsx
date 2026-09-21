/**
 * One route's page: the route on a map, its panel, and the dock along the foot.
 *
 * The page owns everything the map and the panel have to agree on — the open
 * route and every question asked of it. Neither holds a copy, because they are
 * two views of one answer: pointing at the route marks the chart, scrubbing the
 * chart marks the route, and a chip pressed in the panel lights the same ground
 * on both.
 *
 * Choosing a route happens elsewhere, in the search palette. A map of
 * every route at once was a tangle past a dozen of them.
 */

import { useQuery } from "@tanstack/react-query";
import { useCallback, useEffect, useMemo, useState } from "react";
import { useLocation, useNavigate, useParams } from "react-router";
import {
  activitiesQuery,
  routeClimbsQuery,
  routeGeometryQuery,
  routesQuery,
  webUIConfigQuery,
} from "../../api/queries";
import type { Position } from "../../api/types";
import { Layout } from "../../components/Layout";
import { Alert, AlertDescription, AlertTitle } from "../../components/ui/alert";
import { basemapFor, useBasemapChoice, usePrefersDarkScheme } from "../../lib/basemap";
import { ROUTE_MAX_ZOOM, WINDOW_MAX_ZOOM } from "../../lib/cartography";
import { parseRouteKey } from "../../lib/library";
import { useOverlayInsets } from "../../lib/overlayInsets";
import { coordinateRange, rangeBounds } from "../../lib/profile";
import { riddenOn } from "../../lib/rideHistory";
import { useSeenRoutes } from "../../lib/seenRoutes";
import { useStartTime } from "../../lib/startTime";
import type { ThemeChoice } from "../../lib/theme";
import { resolvesDark } from "../../lib/theme";
import { useEscapeKey } from "../../lib/useEscapeKey";
import { type PlannerSeed, plannerSeedFrom } from "../plan/planner";
import { LibraryMap } from "./LibraryMap";
import { RouteDock } from "./RouteDock";
import { RouteOverlay } from "./RouteOverlay";
import { RoutePanel } from "./RoutePanel";
import { useOpenRoute } from "./useOpenRoute";

const NO_LINES: [] = [];

export interface AtlasPageProps {
  /** The reader's colour-scheme pick. Held by `App` — see there for why. */
  themeChoice: ThemeChoice;
}

/**
 * A query that did not arrive, said the same way wherever one does not. The
 * detail is the failure's own message when it has one, since a reader deciding
 * whether to retry is better served by what went wrong than by that it did.
 */
function loadFailure(what: string, error: unknown) {
  const detail = error instanceof Error ? error.message : null;

  return (
    <Alert variant="destructive">
      <AlertTitle>Could not load {what}.</AlertTitle>
      {detail === null ? null : <AlertDescription>{detail}</AlertDescription>}
    </Alert>
  );
}

export function AtlasPage({ themeChoice }: AtlasPageProps) {
  const routes = useQuery(routesQuery());
  const config = useQuery(webUIConfigQuery());
  const prefersDark = usePrefersDarkScheme();
  // The scheme actually in force: the system's own, unless the reader has
  // overridden it. This is what the map's own dark/light decision has to
  // follow — the page's CSS follows the same choice through `[data-theme]`,
  // which `App` applies for every page, not only this one.
  const resolvedDark = resolvesDark(themeChoice, prefersDark);
  /*
   * Which ground the reader asked for, and where it is remembered. Held here
   * rather than in the map, because the style URL is worked out here and the
   * map is handed a style rather than a choice.
   */
  const [basemapChoice, chooseBasemap] = useBasemapChoice();
  // Remembered across visits, and the next half hour where nothing is. See
  // lib/startTime.ts.
  const [startAt, setStartAt] = useStartTime();
  const { markSeen } = useSeenRoutes();
  // What the panels are standing on, so the camera frames a route in the part
  // of the map the reader can actually see.
  const insets = useOverlayInsets();

  const params = useParams();
  const opened = parseRouteKey(`${params.provider}/${params.sourceRouteId}/${params.stageOrder}`);
  const openKey = opened ? `${opened.provider}/${opened.sourceRouteId}/${opened.stageOrder}` : null;

  const library = useMemo(() => routes.data ?? [], [routes.data]);

  const openRoute = opened
    ? (library.find(
        (route) =>
          route.provider === opened.provider &&
          route.sourceRouteId === opened.sourceRouteId &&
          route.stageOrder === opened.stageOrder,
      ) ?? null)
    : null;
  const openGeometry = useQuery({
    ...routeGeometryQuery(
      opened?.provider ?? "",
      opened?.sourceRouteId ?? 0,
      opened?.stageOrder ?? 0,
    ),
    // Only for a route the library actually holds: an address naming one it does
    // not is answered by saying so, not by asking the service about it.
    enabled: openRoute !== null,
  });
  const openCoordinates = useMemo<Position[]>(
    () => openGeometry.data?.coordinates ?? [],
    [openGeometry.data],
  );

  /*
   * A route the library holds whose geometry did not arrive. Nothing can be
   * drawn, framed, or profiled from it, so the panel would be a title over an
   * empty page and the chart an axis with no line under it: the page says what
   * happened instead.
   */
  const openFailed = openRoute !== null && openGeometry.isError;
  const shownRoute = openFailed ? null : openRoute;
  const copySeed = useMemo<PlannerSeed | null>(() => {
    if (!shownRoute) {
      return null;
    }
    return plannerSeedFrom(shownRoute.title, openCoordinates);
  }, [openCoordinates, shownRoute]);

  // The rides matched to the open route, off the query the activity pages share.
  const activities = useQuery({ ...activitiesQuery(), enabled: shownRoute !== null });
  // The rider's own attempts at the open route's climbs.
  const routeClimbs = useQuery({
    ...routeClimbsQuery(
      shownRoute?.provider ?? "",
      shownRoute?.sourceRouteId ?? 0,
      shownRoute?.stageOrder ?? 0,
    ),
    enabled: shownRoute !== null,
  });
  const openRides = useMemo(
    () => (shownRoute ? riddenOn(activities.data ?? [], shownRoute) : []),
    [activities.data, shownRoute],
  );

  // The deterministic trigger for "seen": the route's own panel is shown, however
  // it was opened. Never from rendering it in the list, and never a network call:
  // it writes only to this reader's own browser.
  useEffect(() => {
    if (shownRoute) {
      markSeen(shownRoute);
    }
  }, [shownRoute, markSeen]);

  /*
   * Everything asked of the open route. It lives on this page rather than in
   * either view because both views answer it: the hovered position marks the
   * chart and the map, the stretch on show dims one and frames the other, and
   * a class picked out of the chips lights the same ground on both. The
   * questions themselves are gathered in `useOpenRoute`.
   */
  const {
    activeMetres,
    setActiveMetres,
    highlight,
    setHighlight,
    measure,
    setMeasure,
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
  } = useOpenRoute(openCoordinates, openGeometry, startAt, routeClimbs.data?.climbs ?? []);
  /*
   * What the reader has put away, and it sticks across routes: someone who
   * folded the dock did so to see more map, not to see more of one route's map.
   */
  const [dockOpen, setDockOpen] = useState(true);
  const [panelCollapsed, setPanelCollapsed] = useState(false);

  const navigate = useNavigate();
  const location = useLocation();
  // Back to wherever the route was opened from inside the app, else the landing page.
  const close = useCallback(() => {
    forget();
    if (location.key === "default") {
      navigate("/activities");
    } else {
      navigate(-1);
    }
  }, [forget, location.key, navigate]);

  // Escape leaves one thing at a time, and the stretch on show is the innermost:
  // the overlay answers that one, so this only fires once there is nothing left
  // between the reader and the page they came from.
  useEscapeKey(openKey !== null && shownWindow === null, close);

  // The stretch on show, else the whole route. Memoised: a fresh box every render
  // would be a new camera flight every render.
  const windowBounds = useMemo(() => {
    const range = shownWindow
      ? coordinateRange(openCoordinates, shownWindow.startMetres, shownWindow.endMetres)
      : null;

    return range ? rangeBounds(openCoordinates, range) : null;
  }, [openCoordinates, shownWindow]);
  const bounds = windowBounds ?? openGeometry.data?.bbox ?? null;

  const basemap = config.data ? basemapFor(config.data, resolvedDark, basemapChoice) : null;
  return (
    <Layout
      map={
        basemap ? (
          <LibraryMap
            styleUrl={basemap.styleUrl}
            darkBasemap={basemap.dark}
            basemaps={config.data?.basemaps ?? []}
            selectedBasemap={basemap.name}
            onBasemapChange={chooseBasemap}
            lines={NO_LINES}
            pickedKey={openKey}
            bounds={bounds}
            insets={insets}
            maxZoom={windowBounds ? WINDOW_MAX_ZOOM : ROUTE_MAX_ZOOM}
          >
            {shownRoute && openCoordinates.length > 1 ? (
              <RouteOverlay
                coordinates={openCoordinates}
                surface={surfaceSummary ? surface?.ranges : undefined}
                surfaceSummary={surfaceSummary}
                samples={samples}
                profile={routeProfile}
                activeProfile={windowed ?? routeProfile}
                activeMetres={activeMetres}
                onActiveChange={setActiveMetres}
                profileCollapsed={!dockOpen}
                zoomWindow={shownWindow}
                onZoomChange={onZoomChange}
                highlight={highlight}
                measure={measure}
              />
            ) : null}
          </LibraryMap>
        ) : null
      }
      dock={
        shownRoute ? (
          <RouteDock
            title={shownRoute.title}
            profile={windowed ?? routeProfile}
            distanceMetres={shownRoute.distanceMetres}
            ascentMetres={shownRoute.ascentMetres}
            surface={surfaceSummary}
            climbs={climbs}
            onSelectClimb={selectClimb}
            coordinates={openCoordinates}
            samples={samples}
            startAt={startAt}
            onStartAtChange={setStartAt}
            movingSeconds={movingSeconds}
            activeMetres={activeMetres}
            onActiveChange={setActiveMetres}
            zoomWindow={shownWindow}
            onZoomChange={onZoomChange}
            highlight={highlight}
            onHighlightChange={scopeHighlight}
            measure={measure}
            onMeasureChange={setMeasure}
            rides={openRides}
            open={dockOpen}
            onOpenChange={setDockOpen}
          />
        ) : undefined
      }
    >
      {/*
       * The page's own name. The wordmark below says what the application is
       * rather than what this page shows, so without this the document has no
       * top-level heading at all and a reader navigating by heading is dropped
       * into the middle of a hierarchy. It is not drawn: the map is the title.
       */}
      <h1 className="visually-hidden">{shownRoute?.title ?? "Route"}</h1>
      {routes.isError ? loadFailure("the route library", routes.error) : null}
      {config.isError ? loadFailure("the map configuration", config.error) : null}
      {/*
       * An address naming a route this library does not have. Nothing while the
       * library is on its way: the empty map is the loading state.
       */}
      {routes.isSuccess && openRoute === null ? (
        <Alert variant="destructive">
          <AlertTitle>No route at that address.</AlertTitle>
          <AlertDescription>
            It may have been removed from the library since the link was made.
          </AlertDescription>
        </Alert>
      ) : null}
      {openFailed ? (
        <Alert variant="destructive">
          <AlertTitle>Could not load that route's geometry.</AlertTitle>
          <AlertDescription>
            The library still lists it, so this is worth retrying.
          </AlertDescription>
        </Alert>
      ) : null}
      {shownRoute ? (
        <RoutePanel
          route={shownRoute}
          movingSecondsOverride={selectionMovingSeconds}
          highestMetres={routeProfile ? routeProfile.maxElevationMetres : null}
          lowestMetres={routeProfile ? routeProfile.minElevationMetres : null}
          gradients={gradients}
          surface={surfaceSummary}
          surfaceAbsence={
            surface
              ? "No OpenStreetMap surface data along this route."
              : "Surface not classified yet."
          }
          bands={gradient}
          highlight={highlight}
          onHighlightChange={scopeHighlight}
          onHighlightClear={() => setHighlight(null)}
          collapsed={panelCollapsed}
          onCollapsedChange={setPanelCollapsed}
          onClose={close}
          sourceBaseUrls={config.data?.sourceBaseUrls ?? {}}
          copySeed={copySeed}
        />
      ) : null}
    </Layout>
  );
}
