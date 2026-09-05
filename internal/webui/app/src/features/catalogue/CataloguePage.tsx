/**
 * The catalogue: the whole library written out and ranked.
 *
 * The atlas answers where a ride goes, and its column is a way to one route the
 * reader already has in mind. Neither answers "which of these is about eighty
 * kilometres with under a thousand metres of climbing", because answering that
 * means comparing every route against every other one — which is a table, and a
 * table needs the width the atlas spends on cartography.
 *
 * It asks the service for exactly what the atlas asks for, under the same keys:
 * the listing, and one geometry per route. The listing carries no coordinates,
 * so a row's glyph — the shape that says at a glance whether a ride is a loop or
 * an out-and-back — has nowhere else to come from. Arriving from the atlas those
 * requests are already answered; arriving here first answers them for the atlas
 * in turn. Rows render without geometry and gain their glyph as it lands, so a
 * cold catalogue is readable before any of it arrives.
 *
 * Having it is also what lets the surface filter stand here: a route's ground
 * classes ride with its geometry rather than with the listing.
 *
 * Opening a route hands it to the atlas at `/?route=…` rather than showing it
 * here. There is one place a route is read, and this is a way into it.
 */

import { IconArrowDown, IconArrowUp, IconSearch, IconSelector } from "@tabler/icons-react";
import type { UseQueryResult } from "@tanstack/react-query";
import { useQueries, useQuery } from "@tanstack/react-query";
import { useCallback, useEffect, useMemo, useState } from "react";
import { Link, useSearchParams } from "react-router";
import { routeGeometryQuery, routesQuery, statusQuery } from "../../api/queries";
import type { Position, Route, RouteGeometry, SurfaceKind, SurfaceRange } from "../../api/types";
import { routeKey } from "../../api/types";
import { PageShell } from "../../components/Layout";
import { RouteGlyph } from "../../components/RouteGlyph";
import { Alert, AlertDescription, AlertTitle } from "../../components/ui/alert";
import { InputGroup, InputGroupAddon, InputGroupInput } from "../../components/ui/input-group";
import type { CatalogueView, SortColumn } from "../../lib/catalogue";
import {
  initialDirection,
  readView,
  SORT_COLUMNS,
  sortRoutes,
  writeView,
} from "../../lib/catalogue";
import { hasActiveFilters, matchesFilters } from "../../lib/filters";
import {
  formatAscent,
  formatCount,
  formatDistance,
  formatGradient,
  formatMovingTime,
  formatMovingTimeUncertainty,
  formatReadTime,
} from "../../lib/format";
import { matchingRoutes } from "../../lib/library";
import { useNarrowViewport } from "../../lib/mediaQuery";
import { gradientBand } from "../../lib/profile";
import type { RouteChange } from "../../lib/seenRoutes";
import { useSeenRoutes } from "../../lib/seenRoutes";
import { FilterPanel } from "../routes/FilterPanel";
import { RouteChangeBadge } from "../routes/RouteChangeBadge";
import { CatalogueHeader } from "./CatalogueHeader";
import { CatalogueRow } from "./CatalogueRow";

/** The address the atlas reads a route back off. */
function atlasLink(route: Route): string {
  return `/?route=${encodeURIComponent(routeKey(route))}`;
}

/**
 * The figures every row states, in the order the columns stand.
 *
 * Moving time carries a qualifier where the loaded ride model has measured its
 * own error, which is set apart rather than run together with the figure: it
 * says how much to trust the number beside it, not a second number.
 */
function measures(
  route: Route,
): Array<{ key: string; figure: string; qualifier?: string | undefined }> {
  return [
    { key: "distance", figure: formatDistance(route.distanceMetres) },
    { key: "ascent", figure: formatAscent(route.ascentMetres) },
    { key: "gradient", figure: formatGradient(route.maxGradientPercent) },
    {
      key: "movingTime",
      figure: formatMovingTime(route.movingSeconds),
      qualifier:
        route.movingSeconds === undefined
          ? undefined
          : formatMovingTimeUncertainty(route.validation),
    },
  ];
}

/** The source route this one came off, where the title does not already say it. */
function secondName(route: Route): string | null {
  return route.sourceRouteName !== route.title ? route.sourceRouteName : null;
}

/**
 * One column heading, and the control that ranks by it.
 *
 * `aria-sort` on the header rather than a mark inside the button: it is the
 * column that is sorted, and a reader moving by column hears which one is in
 * force without having to land on the control.
 */
function SortHeader({
  column,
  label,
  view,
  onSort,
  numeric,
}: {
  column: SortColumn;
  label: string;
  view: CatalogueView;
  onSort: (column: SortColumn) => void;
  numeric: boolean;
}) {
  const active = view.sort === column;
  const ascending = view.direction === "asc";

  return (
    <th
      scope="col"
      aria-sort={active ? (ascending ? "ascending" : "descending") : "none"}
      className={`p-0 font-semibold ${numeric ? "text-right" : "text-left"}`}
    >
      <button
        type="button"
        onClick={() => onSort(column)}
        className={`flex w-full items-center gap-1 rounded-md px-3 py-2 hover:bg-[var(--base)] focus-visible:outline-2 focus-visible:outline-offset-[-2px] focus-visible:outline-[var(--accent)] ${
          numeric ? "justify-end" : "justify-start"
        }`}
      >
        {label}
        {active ? (
          ascending ? (
            <IconArrowUp size={14} stroke={2} aria-hidden="true" />
          ) : (
            <IconArrowDown size={14} stroke={2} aria-hidden="true" />
          )
        ) : (
          <IconSelector
            size={14}
            stroke={2}
            aria-hidden="true"
            className="text-[var(--ink-2)] opacity-50"
          />
        )}
      </button>
    </th>
  );
}

/**
 * One route where a table will not fit.
 *
 * Not `ResultRow`: that row's verb is "select", which is a step this page does
 * not have, and its glyph column would stand empty without the geometry this
 * page does not fetch.
 */
function CatalogueCard({
  route,
  coordinates,
  change,
}: {
  route: Route;
  coordinates: Position[];
  change: RouteChange;
}) {
  const where = secondName(route);

  return (
    <li>
      <Link
        to={atlasLink(route)}
        className="flex items-start gap-3 rounded-lg border border-[var(--rule)] p-3 hover:bg-[var(--base)] focus-visible:outline-2 focus-visible:outline-offset-2 focus-visible:outline-[var(--accent)]"
      >
        <span className="mt-0.5 block size-10 shrink-0">
          <RouteGlyph
            coordinates={coordinates}
            title={route.title}
            band={gradientBand(route.maxGradientPercent)}
          />
        </span>
        <span className="min-w-0 flex-1">
          <span className="block font-semibold">{route.title}</span>
          {/*
           * The badge rides on the second line rather than after the title: a
           * card is as wide as the phone, titles here run to two lines of it,
           * and a mark chasing the end of a wrapped title lands alone in the
           * middle of the card.
           */}
          {change === null && where === null ? null : (
            <span className="mt-0.5 flex flex-wrap items-center gap-x-2 text-xs text-[var(--ink-2)]">
              <RouteChangeBadge change={change} />
              {where}
            </span>
          )}
          <span className="mt-2 flex flex-wrap gap-x-3 gap-y-1 text-xs text-[var(--ink-2)] tabular-nums">
            {measures(route).map(({ key, figure }) => (
              <span key={key}>{figure}</span>
            ))}
          </span>
        </span>
      </Link>
    </li>
  );
}

export function CataloguePage() {
  const routes = useQuery(routesQuery());
  const status = useQuery(statusQuery());
  // Read, never written: a row is not an opened route, so nothing here marks a
  // stage seen. Only the atlas does, from the moment a route's own panel shows.
  const { changeOf } = useSeenRoutes();
  const narrow = useNarrowViewport();
  const [filtersExpanded, setFiltersExpanded] = useState(false);

  const [params, setParams] = useSearchParams();
  const view = useMemo(() => readView(params), [params]);

  /*
   * What is in the search field, held here as well as in the address.
   *
   * The field cannot be driven by the address alone. A keystroke reaches the
   * address through the router and comes back a render later, and typing
   * faster than that round trip leaves the input showing the value it had
   * before the last letter — so the letter after that is typed onto a stale
   * string and the ones before it are lost. Typed quickly enough, an eight
   * letter search arrives as its last letter alone.
   *
   * So the field reads from here, which changes as fast as it is typed, and
   * the address follows. The effect below carries the traffic the other way,
   * for an address that changed without this field: the reader pressing Back,
   * or a link opened into the page.
   */
  const [typed, setTyped] = useState(view.query);
  useEffect(() => {
    setTyped(view.query);
  }, [view.query]);

  /*
   * Replaced rather than pushed. Every keystroke in the search field is a
   * change of view, and a history with one entry per letter typed would take a
   * dozen presses of Back to leave the page — while one entry holding the
   * latest view is exactly what makes coming back from an opened route land on
   * the table the reader left.
   *
   * The change is asked for as a function of the address being changed, rather
   * than merged into the view this render happened to read. Two keystrokes
   * inside one frame both see that render's view, so the second would write
   * the first back out: type quickly and the field keeps only the last letter.
   * Reading the address inside the updater means each change applies to the
   * one before it, however fast they arrive.
   */
  const update = useCallback(
    (next: (current: CatalogueView) => Partial<CatalogueView>) => {
      setParams(
        (params) => {
          const current = readView(params);

          return writeView({ ...current, ...next(current) });
        },
        { replace: true },
      );
    },
    [setParams],
  );

  // The first press of a heading ranks by it; pressing the one already in force
  // turns the ranking around.
  const sortBy = useCallback(
    (column: SortColumn) => {
      update((current) =>
        column === current.sort
          ? { direction: current.direction === "asc" ? "desc" : "asc" }
          : { sort: column, direction: initialDirection(column) },
      );
    },
    [update],
  );

  const library = useMemo(() => routes.data ?? [], [routes.data]);

  /*
   * One request per route, in parallel, under the same keys the atlas uses —
   * so whichever page the reader opens first pays for both. Combined against
   * the results rather than in a memo over them, for the reason the atlas
   * gives: `useQueries` hands back a new array every render, and a memo keyed
   * on it would rebuild the collection each time.
   */
  const combine = useCallback(
    (results: Array<UseQueryResult<RouteGeometry>>) => {
      const shapes = new Map<string, Position[]>();
      const surfaces = new Map<string, Set<SurfaceKind>>();
      const ranges = new Map<string, SurfaceRange[]>();
      library.forEach((route, index) => {
        const geometry = results[index]?.data;
        if (!geometry) {
          return;
        }
        const key = routeKey(route);
        shapes.set(key, geometry.coordinates);
        // The distinct kinds a filter checks against, not a share of the
        // route — the same reading the atlas takes off the same fetch.
        const surface = geometry.surface;
        if (surface && surface.matchedMetres > 0 && surface.ranges.length > 0) {
          surfaces.set(key, new Set(surface.ranges.map((range) => range.kind)));
          ranges.set(key, surface.ranges);
        }
      });

      return { shapes, surfaces, ranges };
    },
    [library],
  );

  const drawn = useQueries({
    queries: library.map((route) =>
      routeGeometryQuery(route.provider, route.sourceRouteId, route.stageOrder),
    ),
    combine,
  });

  const shown = useMemo(
    () =>
      sortRoutes(
        matchingRoutes(library, view.query).filter((route) => matchesFilters(route, view.filters)),
        view.sort,
        view.direction,
      ),
    [library, view.query, view.filters, view.sort, view.direction],
  );

  const hasQuery = view.query.trim() !== "";
  const filtersActive = hasActiveFilters(view.filters);
  const sortedLabel = SORT_COLUMNS.find((entry) => entry.column === view.sort)?.label ?? "Route";
  const narrowed = shown.length !== library.length;
  const readAt = status.data?.sync.phases.source?.lastCompletedAt;
  const counted = narrowed
    ? `${shown.length} of ${formatCount(library.length, "route")}`
    : formatCount(library.length, "route");

  return (
    <PageShell>
      <div className="mx-auto flex w-full max-w-5xl flex-col gap-4">
        <h1 className="text-2xl font-semibold tracking-tight">Catalogue</h1>
        <div className="flex items-center gap-2">
          <InputGroup className="bg-[var(--panel)]">
            <InputGroupAddon>
              <IconSearch size={16} stroke={1.6} aria-hidden="true" />
            </InputGroupAddon>
            <InputGroupInput
              type="search"
              value={typed}
              onChange={(event) => {
                const { value } = event.target;
                setTyped(value);
                update(() => ({ query: value }));
              }}
              placeholder="Search the catalogue"
              aria-label="Search the route library"
            />
          </InputGroup>
          <FilterPanel
            library={library}
            filters={view.filters}
            onFiltersChange={(filters) => update(() => ({ filters }))}
            expanded={filtersExpanded}
            onExpandedChange={setFiltersExpanded}
          />
        </div>
        <p className="text-sm text-[var(--ink-2)]">
          {counted}
          {readAt ? ` · read ${formatReadTime(readAt)}` : ""}
        </p>
        {routes.isError ? (
          <Alert variant="destructive">
            <AlertTitle>Could not load the route library.</AlertTitle>
            {routes.error instanceof Error ? (
              <AlertDescription>{routes.error.message}</AlertDescription>
            ) : null}
          </Alert>
        ) : null}
        {routes.isSuccess && library.length === 0 ? (
          <Alert role="status">
            <AlertTitle>No routes yet.</AlertTitle>
            <AlertDescription>
              Routes appear here after the first successful read of the library.
            </AlertDescription>
          </Alert>
        ) : null}
        {library.length > 0 && shown.length === 0 ? (
          <p className="text-sm text-[var(--ink-2)]">
            {/*
             * Whichever of the two actually narrowed the library to nothing,
             * said the way the atlas says it: blaming a filter for what a
             * misremembered name caused points the reader at the wrong control.
             */}
            {hasQuery && filtersActive
              ? "Nothing here matches this search and these filters."
              : filtersActive
                ? "Nothing here matches these filters."
                : "Nothing here is called that."}
          </p>
        ) : null}
        {shown.length === 0 ? null : narrow ? (
          <ul className="grid gap-2">
            {shown.map((route) => (
              <CatalogueCard
                key={routeKey(route)}
                route={route}
                coordinates={drawn.shapes.get(routeKey(route)) ?? []}
                change={changeOf(route)}
              />
            ))}
          </ul>
        ) : (
          <div className="overflow-x-auto rounded-lg border border-[var(--rule)] bg-[var(--panel)]">
            <table className="w-full text-sm">
              {/*
               * The table's own name, which is also where the ranking is
               * stated in words: `aria-sort` says which column to a reader
               * moving through the headings, and this says it to one who
               * lands in the body.
               */}
              <caption className="sr-only">
                {`The route library, ranked by ${sortedLabel.toLowerCase()}, ${
                  view.direction === "asc" ? "ascending" : "descending"
                }`}
              </caption>
              <thead>
                <CatalogueHeader view={view} onSort={sortBy}>
                  <SortHeader
                    column="title"
                    label="Route"
                    view={view}
                    onSort={sortBy}
                    numeric={false}
                  />
                </CatalogueHeader>
              </thead>
              <tbody>
                {shown.map((route) => (
                  <CatalogueRow
                    key={routeKey(route)}
                    route={route}
                    coordinates={drawn.shapes.get(routeKey(route)) ?? []}
                    surface={drawn.ranges.get(routeKey(route))}
                    change={changeOf(route)}
                    to={atlasLink(route)}
                  />
                ))}
              </tbody>
            </table>
          </div>
        )}
      </div>
    </PageShell>
  );
}
