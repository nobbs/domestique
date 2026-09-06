/**
 * The entry page: a map of everything, and one panel over it.
 *
 * The page owns everything the map and the panel have to agree on — what the
 * library is, what the search left of it, which route is selected, which one is
 * open, and every question asked of an open route. Neither the map nor the
 * panel holds a copy, because they are two views of one answer: pointing at the
 * route marks the chart, scrubbing the chart marks the route, and a chip pressed
 * in the panel lights the same ground on both.
 *
 * There is one MapLibre instance for the life of the page. Opening a route adds
 * a stack of layers over the library rather than mounting a second map, so the
 * ground the reader was already looking at is never thrown away and the style is
 * never downloaded twice.
 *
 * Geometry is fetched per route rather than taken from the listing, because the
 * listing carries no bounding box: the entry map needs a line for every route
 * and the glyphs need the same points, so one query serves both — and the route
 * that is opened is served from the same cache, with no second request.
 */

import type { UseQueryResult } from "@tanstack/react-query";
import { useQueries, useQuery } from "@tanstack/react-query";
import { useCallback, useEffect, useMemo, useState } from "react";
import { useSearchParams } from "react-router";
import { routeGeometryQuery, routesQuery, statusQuery, webUIConfigQuery } from "../../api/queries";
import type { BoundingBox, Position, RouteGeometry, SurfaceKind } from "../../api/types";
import { routeKey } from "../../api/types";
import { Layout } from "../../components/Layout";
import { Alert, AlertDescription, AlertTitle } from "../../components/ui/alert";
import { basemapFor, useBasemapChoice, usePrefersDarkScheme } from "../../lib/basemap";
import { ROUTE_MAX_ZOOM, WINDOW_MAX_ZOOM } from "../../lib/cartography";
import type { LibraryFilters } from "../../lib/filters";
import { EMPTY_FILTERS, matchesFilters } from "../../lib/filters";
import { formatReadTime } from "../../lib/format";
import { matchingRoutes } from "../../lib/library";
import { useOverlayInsets } from "../../lib/overlayInsets";
import { coordinateRange, rangeBounds } from "../../lib/profile";
import { useSeenRoutes } from "../../lib/seenRoutes";
import { useStartTime } from "../../lib/startTime";
import { boxAround, LOCATION_ZOOM, useStartupLocation } from "../../lib/startupLocation";
import type { ThemeChoice } from "../../lib/theme";
import { resolvesDark } from "../../lib/theme";
import { useEscapeKey } from "../../lib/useEscapeKey";
import { LibraryMap, type MapLine } from "./LibraryMap";
import { RouteDock } from "./RouteDock";
import { RouteOverlay } from "./RouteOverlay";
import { RoutePanel } from "./RoutePanel";
import type { RouteShape } from "./SearchPanel";
import { SearchPanel } from "./SearchPanel";
import { useOpenRoute } from "./useOpenRoute";

/** The smallest box every drawn route fits inside, or null for no geometry yet. */
function unionOf(boxes: BoundingBox[]): BoundingBox | null {
  const first = boxes[0];
  if (!first) {
    return null;
  }

  return boxes.reduce<BoundingBox>(
    (total, box) => [
      Math.min(total[0], box[0]),
      Math.min(total[1], box[1]),
      Math.max(total[2], box[2]),
      Math.max(total[3], box[3]),
    ],
    first,
  );
}

/**
 * The three parts a `routeKey` is made of, or null for anything that is not one.
 *
 * The address carries every part of the identity the service serves a route
 * under even though nothing on the page ever shows the last two: a library
 * with more than one provider or more than one route under a source route
 * would otherwise have routes that cannot be linked to.
 *
 * The two-part form this address had before a second provider existed is still
 * read, and means the provider it always meant. This is the address the app
 * itself handed out — a bookmarked or shared `?route=12%2F1` predates the
 * provider entirely — so refusing it here would strand exactly the links this
 * change is supposed to keep working, the same way the Go handler keeps the
 * two-segment paths resolving.
 */
function parseRouteKey(
  value: string | null,
): { provider: string; sourceRouteId: number; stageOrder: number } | null {
  const parts = (value ?? "").split("/");
  const [provider, left, right] = parts.length === 2 ? ["veloplanner", ...parts] : parts;
  if (
    parts.length > 3 ||
    !provider ||
    !left ||
    !right ||
    !/^\d+$/.test(left) ||
    !/^\d+$/.test(right)
  ) {
    return null;
  }
  const sourceRouteId = Number.parseInt(left, 10);
  const stageOrder = Number.parseInt(right, 10);

  return sourceRouteId > 0 && stageOrder > 0 ? { provider, sourceRouteId, stageOrder } : null;
}

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
  const status = useQuery(statusQuery());
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
  const { changeOf, markSeen } = useSeenRoutes();
  // What the panels are standing on, so the camera frames a route in the part
  // of the map the reader can actually see.
  const insets = useOverlayInsets();

  const [query, setQuery] = useState("");
  const [filters, setFilters] = useState<LibraryFilters>(EMPTY_FILTERS);
  const [filtersExpanded, setFiltersExpanded] = useState(false);
  const [pickedKey, setPickedKey] = useState<string | null>(null);

  /*
   * The open route lives in the address rather than in state, so one is still a
   * link somebody can send: everything else on this page — what was typed, which
   * row is expanded, whether the chart is up — is a way of getting to a route
   * rather than a thing worth linking to.
   */
  const [params, setParams] = useSearchParams();
  const opened = parseRouteKey(params.get("route"));
  const openKey = opened ? `${opened.provider}/${opened.sourceRouteId}/${opened.stageOrder}` : null;

  const library = useMemo(() => routes.data ?? [], [routes.data]);

  /*
   * One request per route, in parallel, and each of them cached for as long as
   * the geometry query says. It is the cost of a map of everything: the listing
   * has no bounding box to draw from, and adding one would be a change to the
   * service's wire contract for the sake of a first paint.
   */
  /*
   * Combined here rather than in a memo over the results, because `useQueries`
   * hands back a new array on every render and a memo keyed on it would rebuild
   * the collection every time — and the map would be given new lines to upload
   * every time with it. `combine` is memoised against the results themselves,
   * so the map is handed the same lines until a geometry actually arrives.
   */
  const combine = useCallback(
    (results: Array<UseQueryResult<RouteGeometry>>) => {
      const lines: MapLine[] = [];
      const shapes = new Map<string, RouteShape>();
      const boxes = new Map<string, BoundingBox>();
      // Ground classes actually present on each route, once its geometry has
      // arrived and the enrichment pass has classified it — read from the same
      // fetch the map already makes, not a second one for the filter's sake.
      const surfaces = new Map<string, Set<SurfaceKind>>();
      library.forEach((route, index) => {
        const geometry = results[index]?.data;
        if (!geometry) {
          return;
        }
        const key = routeKey(route);
        const coordinates: Position[] = geometry.coordinates;
        lines.push({ key, coordinates });
        shapes.set(key, { coordinates });
        boxes.set(key, geometry.bbox);
        const surface = geometry.surface;
        // The distinct kinds a filter checks against, not a share of the
        // route. This runs for every already-arrived route each time another
        // one's geometry lands, so it deliberately skips `summariseSurface`'s
        // walk over every coordinate to measure lengths this has no use for.
        if (surface && surface.matchedMetres > 0 && surface.ranges.length > 0) {
          surfaces.set(key, new Set(surface.ranges.map((range) => range.kind)));
        }
      });

      return { lines, shapes, boxes, surfaces };
    },
    [library],
  );

  const drawn = useQueries({
    queries: library.map((route) =>
      routeGeometryQuery(route.provider, route.sourceRouteId, route.stageOrder),
    ),
    combine,
  });

  // Matched and sorted on its own memo, so adjusting a filter — which leaves
  // the query untouched — does not repeat a sort over the whole library for
  // every keystroke in the filter panel.
  const queried = useMemo(() => matchingRoutes(library, query), [library, query]);
  const shown = useMemo(
    () => queried.filter((route) => matchesFilters(route, filters)),
    [queried, filters],
  );

  // The open route's geometry, which the library pass has already fetched under
  // exactly this key: asking for it by name is a read of the cache rather than
  // a second request, and it is how the panel's own surface classification —
  // richer than the filter's kind-only reading of the same fetch — reaches it.
  const openRoute = library.find((route) => routeKey(route) === openKey) ?? null;
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
  const openCoordinates = useMemo(
    () => (openKey ? (drawn.shapes.get(openKey)?.coordinates ?? []) : []),
    [drawn.shapes, openKey],
  );

  /*
   * A route the library holds whose geometry did not arrive. Nothing can be
   * drawn, framed, or profiled from it, so the panel would be a title over an
   * empty page and the chart an axis with no line under it: the page says what
   * happened instead, and leaves the search standing as the way on.
   */
  const openFailed = openRoute !== null && openGeometry.isError;
  const shownRoute = openFailed ? null : openRoute;

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
  } = useOpenRoute(openCoordinates, openGeometry, startAt);
  /*
   * What the reader has put away, and it sticks across routes: someone who
   * folded the dock did so to see more map, not to see more of one route's map.
   */
  const [dockOpen, setDockOpen] = useState(true);
  const [panelCollapsed, setPanelCollapsed] = useState(false);

  const open = useCallback(
    (key: string) => {
      forget();
      setParams((current) => {
        const next = new URLSearchParams(current);
        next.set("route", key);

        return next;
      });
    },
    [forget, setParams],
  );

  /**
   * The route standing out on the map: the one that is open, or the one picked.
   *
   * Keyed rather than indexed, because geometry arrives one request at a time
   * and a position in a list of what has arrived is not a position in the
   * library.
   */
  const focusKey = openKey ?? pickedKey;

  /**
   * A route picked off the map, by pointing at where it goes.
   *
   * The map is the library, so a line on it is the route itself rather than a
   * picture of one: pointing at where a ride goes is the most direct way there
   * is of asking about it, and it takes no column at all.
   *
   * The same two steps the column has, in the same order: the first click shows
   * the route's card, the second opens it. The lines cross and the reader is
   * panning across them, so a map where one click swapped the whole panel would
   * be a minefield — the card is what says which route was hit before anything
   * is committed to.
   *
   * An opened route keeps the map to itself: its hit target is gone once the
   * overlay is up, and a pick before that is still not a way out of the route.
   */
  const pick = useCallback(
    (key: string) => {
      if (openKey !== null) {
        return;
      }
      if (key === pickedKey) {
        open(key);

        return;
      }
      // The search is one way to a route and the map is another, so a route
      // picked off the map is the answer to whatever was typed: a card that
      // stayed hidden behind a query it does not match would be a selection the
      // reader can see on the ground and nowhere else.
      if (!shown.some((route) => routeKey(route) === key)) {
        setQuery("");
      }
      setPickedKey(key);
    },
    [open, openKey, pickedKey, shown],
  );

  const close = useCallback(() => {
    forget();
    setPickedKey(null);
    setParams((current) => {
      const next = new URLSearchParams(current);
      next.delete("route");

      return next;
    });
  }, [forget, setParams]);

  // Escape leaves one thing at a time, and the stretch on show is the innermost:
  // the overlay answers that one, so this only fires once there is nothing left
  // between the reader and the library.
  useEscapeKey(openKey !== null && shownWindow === null, close);

  /*
   * What the camera frames, in the order the reader asked for it: the stretch
   * on show, then the route they opened or picked out of the column, then the
   * whole library. Each of them is memoised, because the camera moves when the
   * framing changes and a fresh box every render would be a new flight every
   * keystroke — the map snapping back from wherever it had been panned.
   */
  const windowBounds = useMemo(() => {
    const range = shownWindow
      ? coordinateRange(openCoordinates, shownWindow.startMetres, shownWindow.endMetres)
      : null;

    return range ? rangeBounds(openCoordinates, range) : null;
  }, [openCoordinates, shownWindow]);
  const focusBox = focusKey ? (drawn.boxes.get(focusKey) ?? null) : null;
  const libraryBounds = useMemo(() => unionOf([...drawn.boxes.values()]), [drawn.boxes]);
  // Decided once, at mount, from the URL alone: a deep link keeps framing its
  // route once the geometry arrives, rather than flying off to a position.
  const [locationEnabled] = useState(() => openKey === null);
  const location = useStartupLocation(locationEnabled);
  const locationBox = useMemo(() => (location ? boxAround(location) : null), [location]);
  const bounds = windowBounds ?? focusBox ?? locationBox ?? libraryBounds;

  const basemap = config.data ? basemapFor(config.data, resolvedDark, basemapChoice) : null;
  const readAt = status.data?.sync.phases.source?.lastCompletedAt;

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
            lines={drawn.lines}
            pickedKey={focusKey}
            bounds={bounds}
            insets={insets}
            maxZoom={
              windowBounds
                ? WINDOW_MAX_ZOOM
                : focusBox === null && locationBox !== null
                  ? LOCATION_ZOOM
                  : ROUTE_MAX_ZOOM
            }
            onPick={pick}
            inertKey={openKey}
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
      <h1 className="visually-hidden">Route library</h1>
      {routes.isError ? loadFailure("the route library", routes.error) : null}
      {config.isError ? loadFailure("the map configuration", config.error) : null}
      {/*
       * Nothing while the library is on its way: the map with no traces on it is
       * the loading state, and a panel saying so would cover the ground it is
       * waiting to draw.
       */}
      {routes.isSuccess && library.length === 0 ? (
        <Alert role="status">
          <AlertTitle>No routes yet.</AlertTitle>
          <AlertDescription>
            Routes appear here after the first successful read of the library.
          </AlertDescription>
        </Alert>
      ) : null}
      {/*
       * An address naming a route this library does not have. It is the one
       * failure this page can arrive in rather than fall into, so it says what
       * happened instead of silently showing the library.
       */}
      {routes.isSuccess && library.length > 0 && openKey !== null && openRoute === null ? (
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
            The library still lists it, so this is worth retrying; search below for another route in
            the meantime.
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
          libraryCount={library.length}
          onClose={close}
          sourceBaseUrls={config.data?.sourceBaseUrls ?? {}}
        />
      ) : library.length > 0 ? (
        <SearchPanel
          shown={shown}
          library={library}
          query={query}
          onQueryChange={(next) => {
            setQuery(next);
            // A search that no longer holds the open route would leave the card
            // expanded off the bottom of a column it is not in.
            setPickedKey(null);
          }}
          filters={filters}
          onFiltersChange={(next) => {
            setFilters(next);
            // Same reasoning as a changed search: a filter that no longer holds
            // the open route must not leave its card expanded regardless.
            setPickedKey(null);
          }}
          filtersExpanded={filtersExpanded}
          onFiltersExpandedChange={setFiltersExpanded}
          pickedKey={pickedKey}
          onSelect={setPickedKey}
          onOpen={open}
          shapes={drawn.shapes}
          readAt={readAt ? formatReadTime(readAt) : null}
          changeOf={changeOf}
        />
      ) : null}
    </Layout>
  );
}
