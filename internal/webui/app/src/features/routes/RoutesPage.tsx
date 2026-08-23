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
import { RouteOverlay, SURFACE_ATTRIBUTION } from "../../components/RouteOverlay";
import { ErrorMessage, StatusMessage } from "../../components/StatusMessage";
import { Wordmark } from "../../components/Wordmark";
import { basemapFor, useBasemapChoice, usePrefersDarkScheme } from "../../lib/basemap";
import type { Climb } from "../../lib/climbs";
import { findClimbs } from "../../lib/climbs";
import type { LibraryFilters } from "../../lib/filters";
import { EMPTY_FILTERS, matchesFilters } from "../../lib/filters";
import { formatReadTime } from "../../lib/format";
import type { Highlight } from "../../lib/highlight";
import { matchingRoutes } from "../../lib/library";
import { useOverlayInsets } from "../../lib/overlayInsets";
import type { DistanceWindow } from "../../lib/profile";
import {
  buildProfile,
  buildWindowedProfile,
  coordinateRange,
  gradientShares,
  rangeBounds,
} from "../../lib/profile";
import { useSeenStages } from "../../lib/seenStages";
import { widened } from "../../lib/selection";
import { summariseSurface } from "../../lib/surface";
import { useEscapeKey } from "../../lib/useEscapeKey";
import type { LibraryLine } from "./LibraryMap";
import { LibraryMap } from "./LibraryMap";
import { RoutePanel } from "./RoutePanel";
import { RouteProfile } from "./RouteProfile";
import type { RouteShape } from "./SearchPanel";
import { SearchPanel } from "./SearchPanel";

/**
 * How close the camera will go to the library, or to one whole route.
 *
 * A short route would otherwise open at street level, which says nothing about
 * where the ride goes.
 */
const ROUTE_MAX_ZOOM = 14;

/**
 * And how close it may come to the stretch the chart is showing.
 *
 * Higher, because that framing was asked for: the shortest window the chart
 * allows is 200 m, and holding it to the whole-route cap would answer a request
 * to look closer by barely moving.
 */
const WINDOW_MAX_ZOOM = 17;

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
 * with more than one provider or more than one stage under a route would
 * otherwise have routes that cannot be linked to.
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
): { provider: string; routeId: number; stageOrder: number } | null {
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
  const routeId = Number.parseInt(left, 10);
  const stageOrder = Number.parseInt(right, 10);

  return routeId > 0 && stageOrder > 0 ? { provider, routeId, stageOrder } : null;
}

/**
 * How the card says every account holds the library.
 *
 * Counted, because an operator with two Wahoo accounts reads "on both accounts"
 * as a statement about their own setup, and "on every account" as a statement
 * about a set they have to remember the size of.
 */
function convergedPhrase(targetCount: number): string {
  if (targetCount === 1) {
    return "on the account";
  }

  return targetCount === 2 ? "on both accounts" : "on every account";
}

export function RoutesPage() {
  const routes = useQuery(routesQuery());
  const config = useQuery(webUIConfigQuery());
  const status = useQuery(statusQuery());
  const prefersDark = usePrefersDarkScheme();
  /*
   * Which ground the reader asked for, and where it is remembered. Held here
   * rather than in the map, because the style URL is worked out here and the
   * map is handed a style rather than a choice.
   */
  const [basemapChoice, chooseBasemap] = useBasemapChoice();
  const { changeOf, markSeen } = useSeenStages();
  // What the panels are standing on, so the camera frames a route in the part
  // of the map the reader can actually see.
  const insets = useOverlayInsets();

  const [query, setQuery] = useState("");
  const [filters, setFilters] = useState<LibraryFilters>(EMPTY_FILTERS);
  const [filtersExpanded, setFiltersExpanded] = useState(false);
  const [selectedKey, setSelectedKey] = useState<string | null>(null);

  /*
   * The open route lives in the address rather than in state, so one is still a
   * link somebody can send: everything else on this page — what was typed, which
   * row is expanded, whether the chart is up — is a way of getting to a route
   * rather than a thing worth linking to.
   */
  const [params, setParams] = useSearchParams();
  const opened = parseRouteKey(params.get("route"));
  const openKey = opened ? `${opened.provider}/${opened.routeId}/${opened.stageOrder}` : null;

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
      const lines: LibraryLine[] = [];
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
        const summary =
          surface && surface.matchedMetres > 0
            ? summariseSurface(coordinates, surface.ranges)
            : null;
        if (summary) {
          surfaces.set(key, new Set(summary.shares.map((share) => share.kind)));
        }
      });

      return { lines, shapes, boxes, surfaces };
    },
    [library],
  );

  const drawn = useQueries({
    queries: library.map((route) =>
      routeGeometryQuery(route.provider, route.routeId, route.stageOrder),
    ),
    combine,
  });

  const shown = useMemo(
    () =>
      matchingRoutes(library, query).filter((route) =>
        matchesFilters(route, filters, drawn.surfaces.get(routeKey(route))),
      ),
    [library, query, filters, drawn.surfaces],
  );

  // The open route's geometry, which the library pass has already fetched under
  // exactly this key: asking for it by name is a read of the cache rather than
  // a second request, and it is how the panel's own surface classification —
  // richer than the filter's kind-only reading of the same fetch — reaches it.
  const openRoute = library.find((route) => routeKey(route) === openKey) ?? null;
  const openGeometry = useQuery({
    ...routeGeometryQuery(opened?.provider ?? "", opened?.routeId ?? 0, opened?.stageOrder ?? 0),
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

  // The deterministic trigger for "seen": the stage's own panel is actually
  // shown, whether that came from pressing "Open route", picking it off the
  // map, or opening a link straight to it. Never from rendering it in the
  // list, and never a network call — it only ever writes to this reader's own
  // browser.
  useEffect(() => {
    if (shownRoute) {
      markSeen(shownRoute);
    }
  }, [shownRoute, markSeen]);

  /*
   * Everything asked of the open route. It lives here rather than in either
   * view because both views answer it: the hovered position marks the chart and
   * the map, the stretch on show dims one and frames the other, and a class
   * picked out of the chips lights the same ground on both.
   */
  const [activeMetres, setActiveMetres] = useState<number | null>(null);
  const [zoomWindow, setZoomWindow] = useState<DistanceWindow | null>(null);
  const [highlight, setHighlight] = useState<Highlight | null>(null);
  const [chartCollapsed, setChartCollapsed] = useState(false);

  const routeProfile = useMemo(() => buildProfile(openCoordinates), [openCoordinates]);
  // Rebuilt from the original geometry rather than from the last window, so
  // zooming inside a zoom compounds no rounding error and needs no stack.
  const windowed = useMemo(
    () => (zoomWindow ? buildWindowedProfile(openCoordinates, zoomWindow) : null),
    [openCoordinates, zoomWindow],
  );
  // A window that built nothing is a slip, not a view: the map must not dim
  // around a stretch the chart is not showing.
  const shownWindow = windowed ? zoomWindow : null;

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
  const focusKey = openKey ?? selectedKey;

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
   * With a route already open the step is skipped: there is no column for a card
   * to be in, so a pick opens the line it landed on. The open route itself is
   * left alone rather than reopened, which would throw away everything asked of
   * it since — the stretch the chart is zoomed into most of all, picked by
   * dragging along that very line. The map makes the same promise by giving that
   * one line no pointer cursor.
   */
  const pick = useCallback(
    (key: string) => {
      if (openKey !== null) {
        if (key !== openKey) {
          open(key);
        }

        return;
      }
      if (key === selectedKey) {
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
      setSelectedKey(key);
    },
    [open, openKey, selectedKey, shown],
  );

  const close = useCallback(() => {
    forget();
    setSelectedKey(null);
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

  // The route's steepness, classified from the coordinates the service stored
  // rather than from any resampling of them, and totalled per band. Held here so
  // the chips do not re-run the classification on every hover.
  const gradient = useMemo(() => gradientShares(openCoordinates), [openCoordinates]);

  // The route's sustained climbs, from the same stored coordinates.
  const climbs = useMemo(() => findClimbs(openCoordinates), [openCoordinates]);

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

  // A classification that snapped to nothing is left unpainted rather than drawn
  // as unsurveyed from end to end: greying out the whole route to say nothing is
  // known says it less clearly than one sentence does.
  const surface = openGeometry.data?.surface;
  const surfaceSummary = useMemo(
    () =>
      surface && surface.matchedMetres > 0
        ? summariseSurface(openCoordinates, surface.ranges)
        : null,
    [openCoordinates, surface],
  );

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
  const bounds = windowBounds ?? focusBox ?? libraryBounds;

  const basemap = config.data ? basemapFor(config.data, prefersDark, basemapChoice) : null;
  const readAt = status.data?.sync.phases.source?.lastCompletedAt;

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
  const subtitle = [
    openRoute && openRoute.routeName !== openRoute.title ? openRoute.routeName : null,
    readAt ? `read ${formatReadTime(readAt)}` : null,
    status.data?.converged ? convergedPhrase(status.data.targets.length) : null,
  ]
    .filter(Boolean)
    .join(" · ");

  return (
    <Layout
      expanded={query.trim() !== "" || selectedKey !== null || openKey !== null}
      // Only an open route: the results column is deliberately no wider than a
      // line worth reading, and the extra width is the profile's, not the list's.
      wide={openKey !== null}
      map={
        basemap ? (
          <LibraryMap
            styleUrl={basemap.styleUrl}
            darkBasemap={basemap.dark}
            basemaps={config.data?.basemaps ?? []}
            selectedBasemap={basemap.name}
            onBasemapChange={chooseBasemap}
            lines={drawn.lines}
            selectedKey={focusKey}
            bounds={bounds}
            insets={insets}
            maxZoom={windowBounds ? WINDOW_MAX_ZOOM : ROUTE_MAX_ZOOM}
            extraCredit={shownRoute ? SURFACE_ATTRIBUTION : undefined}
            onPick={pick}
            inertKey={openKey}
            overlay={
              shownRoute && openCoordinates.length > 1 ? (
                <RouteOverlay
                  darkBasemap={basemap.dark}
                  coordinates={openCoordinates}
                  surface={surfaceSummary ? surface?.ranges : undefined}
                  surfaceSummary={surfaceSummary}
                  profile={routeProfile}
                  activeProfile={windowed ?? routeProfile}
                  activeMetres={activeMetres}
                  onActiveChange={setActiveMetres}
                  profileCollapsed={chartCollapsed}
                  zoomWindow={shownWindow}
                  onZoomChange={onZoomChange}
                  highlight={highlight}
                />
              ) : null
            }
          />
        ) : null
      }
    >
      {/*
       * The page's own name. The wordmark below says what the application is
       * rather than what this page shows, so without this the document has no
       * top-level heading at all and a reader navigating by heading is dropped
       * into the middle of a hierarchy. It is not drawn: the map is the title.
       */}
      <h1 className="visually-hidden">Route library</h1>
      <Wordmark />
      {routes.isError ? <ErrorMessage what="the route library" error={routes.error} /> : null}
      {config.isError ? <ErrorMessage what="the map configuration" error={config.error} /> : null}
      {/*
       * Nothing while the library is on its way: the map with no traces on it is
       * the loading state, and a panel saying so would cover the ground it is
       * waiting to draw.
       */}
      {routes.isSuccess && library.length === 0 ? (
        <StatusMessage
          title="No routes yet."
          detail="Routes appear here after the first successful read of the library."
        />
      ) : null}
      {/*
       * An address naming a route this library does not have. It is the one
       * failure this page can arrive in rather than fall into, so it says what
       * happened instead of silently showing the library.
       */}
      {routes.isSuccess && library.length > 0 && openKey !== null && openRoute === null ? (
        <StatusMessage
          tone="error"
          title="No route at that address."
          detail="It may have been removed from the library since the link was made."
        />
      ) : null}
      {openFailed ? (
        <StatusMessage
          tone="error"
          title="Could not load that route's geometry."
          detail="The library still lists it, so this is worth retrying; search below for another route in the meantime."
        />
      ) : null}
      {shownRoute ? (
        <RoutePanel
          route={shownRoute}
          profile={
            <RouteProfile
              profile={windowed ?? routeProfile}
              title={shownRoute.title}
              ascentMetres={shownRoute.ascentMetres}
              surface={surfaceSummary}
              activeMetres={activeMetres}
              onActiveChange={setActiveMetres}
              zoomWindow={shownWindow}
              onZoomChange={onZoomChange}
              highlight={highlight}
              collapsed={chartCollapsed}
              onCollapsedChange={setChartCollapsed}
            />
          }
          highestMetres={routeProfile ? routeProfile.maxElevationMetres : null}
          subtitle={subtitle}
          surface={surfaceSummary}
          surfaceAbsence={
            surface
              ? "No OpenStreetMap surface data along this route."
              : "Surface not classified yet."
          }
          bands={gradient}
          highlight={highlight}
          onHighlightChange={setHighlight}
          climbs={climbs}
          onSelectClimb={selectClimb}
          libraryCount={library.length}
          onClose={close}
          sourceBaseUrls={config.data?.sourceBaseUrls ?? {}}
        />
      ) : library.length > 0 ? (
        <SearchPanel
          shown={shown}
          total={library.length}
          query={query}
          onQueryChange={(next) => {
            setQuery(next);
            // A search that no longer holds the open route would leave the card
            // expanded off the bottom of a column it is not in.
            setSelectedKey(null);
          }}
          filters={filters}
          onFiltersChange={(next) => {
            setFilters(next);
            // Same reasoning as a changed search: a filter that no longer holds
            // the open route must not leave its card expanded regardless.
            setSelectedKey(null);
          }}
          filtersExpanded={filtersExpanded}
          onFiltersExpandedChange={setFiltersExpanded}
          selectedKey={selectedKey}
          onSelect={setSelectedKey}
          onOpen={open}
          shapes={drawn.shapes}
          readAt={readAt ? formatReadTime(readAt) : null}
          changeOf={changeOf}
        />
      ) : null}
    </Layout>
  );
}
