/**
 * The catalogue: the whole library written out and ranked.
 *
 * It asks the service for the listing and one geometry per route, under the
 * same keys the route page and the ⌘K jump use. The listing carries no
 * coordinates, so a row's glyph — the shape that says at a glance whether a ride
 * is a loop or an out-and-back — has nowhere else to come from. Rows render
 * without geometry and gain their glyph and mix bars as it lands, so a cold
 * catalogue is readable before any of it arrives. An admin on a deployment that
 * plans also reads the plan listing and each draft, for the Drafts shelf.
 *
 * On a wide screen the sidebar carries a map of one route: the row last pointed
 * at, or the first row until one is. A map of every route at once is a tangle
 * past a dozen of them.
 *
 * Opening a route leads to its own page at `/routes/…`: there is one place a
 * route is read, and this is the way into it.
 */

import {
  IconAdjustmentsHorizontal,
  IconArrowDown,
  IconArrowUp,
  IconBooks,
  IconChartBar,
  IconClockEdit,
  IconSearch,
  IconX,
} from "@tabler/icons-react";
import type { UseQueryResult } from "@tanstack/react-query";
import { useQueries, useQuery } from "@tanstack/react-query";
import { useCallback, useEffect, useMemo, useRef, useState } from "react";
import { Link, useSearchParams } from "react-router";
import { routeGeometryQuery, routesQuery, statusQuery, webUIConfigQuery } from "../../api/queries";
import type { BoundingBox, Position, Route, RouteGeometry, SurfaceRange } from "../../api/types";
import { routeKey } from "../../api/types";
import { PageShell } from "../../components/Layout";
import { Panel } from "../../components/PanelHeading";
import { RouteGlyph } from "../../components/RouteGlyph";
import type { SegmentedItem } from "../../components/Segmented";
import { Segmented } from "../../components/Segmented";
import { Alert, AlertDescription, AlertTitle } from "../../components/ui/alert";
import { basemapFor, useBasemapChoice, usePrefersDarkScheme } from "../../lib/basemap";
import type { CatalogueView, SortColumn } from "../../lib/catalogue";
import {
  initialDirection,
  readView,
  SORT_COLUMNS,
  sortRoutes,
  writeView,
} from "../../lib/catalogue";
import { activeFilterCount, hasActiveFilters, matchesFilters } from "../../lib/filters";
import {
  formatAscent,
  formatCount,
  formatDistance,
  formatGradient,
  formatMovingTime,
  formatMovingTimeUncertainty,
  formatReadTime,
  formatTimestamp,
} from "../../lib/format";
import { useEffectiveAdmin } from "../../lib/identity";
import { matchesText, matchingRoutes, type RouteVisit, routePath } from "../../lib/library";
import { useMediaQuery, useNarrowViewport } from "../../lib/mediaQuery";
import { bandLabel, bandVariable, surfaceLabel, surfaceVariable } from "../../lib/mix";
import { gradientBand, gradientShares, haversineMetres, rangeBounds } from "../../lib/profile";
import type { RouteChange } from "../../lib/seenRoutes";
import { useSeenRoutes } from "../../lib/seenRoutes";
import { useStartupLocation } from "../../lib/startupLocation";
import { summariseSurface } from "../../lib/surface";
import { resolvesDark, type ThemeChoice } from "../../lib/theme";
import { LibraryMap, type MapLine } from "../routes/LibraryMap";
import { RouteChangeBadge } from "../routes/RouteChangeBadge";
import { CatalogueFilters } from "./CatalogueFilters";
import { DraftList, draftKey, EditPlanButton, planEditLink, useDrafts } from "./Drafts";
import type { ThinBarSegment } from "./ThinBar";
import { ThinBar } from "./ThinBar";

/** The source route this one came off, where the title does not already say it. */
function secondName(route: Route): string | null {
  return route.sourceRouteName !== route.title ? route.sourceRouteName : null;
}

const SORT_ITEMS: ReadonlyArray<SegmentedItem<SortColumn>> = [
  { key: "title", label: "Name" },
  { key: "distance", label: "Distance" },
  { key: "ascent", label: "Ascent" },
  { key: "movingTime", label: "Time" },
  { key: "gradient", label: "Steepest" },
];
const START_ITEM: SegmentedItem<SortColumn> = { key: "start", label: "Nearest" };

/** Focuses the search field on ⌘K or Ctrl+K, as its hint says. */
function useShortcut(ref: React.RefObject<HTMLInputElement | null>) {
  useEffect(() => {
    const onKey = (event: KeyboardEvent) => {
      if ((event.metaKey || event.ctrlKey) && event.key.toLowerCase() === "k") {
        event.preventDefault();
        ref.current?.focus();
      }
    };
    window.addEventListener("keydown", onKey);
    return () => window.removeEventListener("keydown", onKey);
  }, [ref]);
}

const Kbd = ({ children }: { children: React.ReactNode }) => (
  <kbd className="rounded-[7px] bg-[var(--muted)] px-2 py-0.5 font-sans text-[var(--ink-2)] text-xs">
    {children}
  </kbd>
);

/**
 * A search field that stays lit — white against the muted header — while it
 * is focused or holds text, so a search still in force reads as one at a
 * glance even once the reader has looked away.
 */
function SoftSearch({ typed, onChange }: { typed: string; onChange: (value: string) => void }) {
  const field = useRef<HTMLInputElement>(null);
  useShortcut(field);
  const lit = typed !== "";
  return (
    <label
      data-lit={lit || undefined}
      className="flex h-10 w-72 items-center gap-2 rounded-[11px] bg-[var(--muted)] px-3 focus-within:bg-[var(--panel)] focus-within:shadow-[0_0_0_1px_var(--rule),var(--shadow)] data-lit:bg-[var(--panel)] data-lit:shadow-[0_0_0_1px_var(--rule),var(--shadow)]"
    >
      <IconSearch
        size={16}
        stroke={1.8}
        className={lit ? "text-[var(--ink)]" : "text-[var(--ink-2)]"}
        aria-hidden="true"
      />
      <input
        ref={field}
        type="search"
        value={typed}
        onChange={(event) => onChange(event.target.value)}
        placeholder="Search the catalogue"
        aria-label="Search the route library"
        className="min-w-0 flex-1 bg-transparent text-sm outline-none placeholder:text-[var(--ink-2)] [&::-webkit-search-cancel-button]:appearance-none"
      />
      {lit ? (
        <button
          type="button"
          aria-label="Clear search"
          onClick={() => onChange("")}
          className="text-[var(--ink-2)] hover:text-[var(--ink)]"
        >
          <IconX size={14} />
        </button>
      ) : (
        <Kbd>⌘K</Kbd>
      )}
    </label>
  );
}

function FiltersToggle({
  open,
  onOpen,
  count,
}: {
  open: boolean;
  onOpen: (open: boolean) => void;
  count: number;
}) {
  return (
    <button
      type="button"
      aria-expanded={open}
      onClick={() => onOpen(!open)}
      className={`flex h-10 items-center gap-2 rounded-[11px] px-3 text-sm ${open ? "bg-[var(--panel)] text-[var(--ink)] shadow-[0_0_0_1px_var(--rule),var(--shadow)]" : count > 0 ? "bg-[var(--ink)] text-[var(--panel)]" : "bg-[var(--muted)] text-[var(--ink-2)] hover:text-[var(--ink)]"}`}
    >
      <IconAdjustmentsHorizontal size={16} stroke={1.8} aria-hidden="true" />
      Filters
      {count > 0 ? (
        <span
          className={`grid size-5 place-items-center rounded-[6px] font-semibold text-xs ${open ? "bg-[var(--ink)] text-[var(--panel)]" : "bg-[var(--panel)] text-[var(--ink)]"}`}
        >
          {count}
        </span>
      ) : null}
    </button>
  );
}

function SortControl({
  view,
  sortBy,
  nearby,
}: {
  view: CatalogueView;
  sortBy: (column: SortColumn) => void;
  /** Whether the reader's position is known, so distance to start can rank. */
  nearby: boolean;
}) {
  return (
    <span className="flex flex-wrap items-center gap-1.5">
      <Segmented
        label="Sort by"
        size="sm"
        items={nearby ? [...SORT_ITEMS, START_ITEM] : SORT_ITEMS}
        value={view.sort}
        onChange={sortBy}
      />
      <button
        type="button"
        aria-label={view.direction === "asc" ? "Ascending" : "Descending"}
        onClick={() => sortBy(view.sort)}
        className="grid size-8 place-items-center rounded-[9px] bg-[var(--muted)] text-[var(--ink-2)] hover:text-[var(--ink)]"
      >
        {view.direction === "asc" ? <IconArrowUp size={14} /> : <IconArrowDown size={14} />}
      </button>
    </span>
  );
}

/** The two mixes one route draws in its row, off geometry already fetched for the glyphs. */
function RouteMixBars({
  coordinates,
  surface,
}: {
  coordinates: Position[];
  surface: SurfaceRange[] | undefined;
}) {
  // Nothing is said until the geometry is in hand: a route drawn before its
  // shape arrives shows no bars rather than a premature "not classified".
  if (coordinates.length === 0) {
    return null;
  }
  const summary = surface ? summariseSurface(coordinates, surface) : null;
  const bands = gradientShares(coordinates);
  const surfaceSegments: ThinBarSegment[] =
    summary?.shares.map((entry) => ({
      key: entry.kind,
      label: surfaceLabel(entry.kind),
      colour: surfaceVariable(entry.kind),
      share: entry.share,
    })) ?? [];
  const gradientSegments: ThinBarSegment[] = bands.map((entry) => ({
    key: `${entry.band}`,
    label: bandLabel(entry.band),
    colour: bandVariable(entry.band),
    share: entry.share,
  }));
  if (surfaceSegments.length === 0 && gradientSegments.length === 0) {
    return null;
  }

  return (
    <span className="flex max-w-72 flex-col gap-1">
      {surfaceSegments.length > 0 ? <ThinBar segments={surfaceSegments} label="Surface" /> : null}
      {gradientSegments.length > 0 ? (
        <ThinBar segments={gradientSegments} label="Gradient" />
      ) : null}
    </span>
  );
}

/** How far a route's start is: null hides the figure, undefined is not known yet. */
type StartDistance = number | undefined | null;

function formatStart(start: StartDistance): string {
  if (start === undefined || start === null) {
    return "–";
  }
  // formatDistance reads zero as missing data; standing on the start is not that.
  return start === 0 ? "0 m" : formatDistance(start);
}

/** What a row reports back to the map it shares the page with. */
interface RowLink {
  active: boolean;
  onActivate: (key: string) => void;
}

/** One route, as an inset ledger row: shape, name and mixes, then its figures. */
function LedgerRow({
  route,
  coordinates,
  surface,
  change,
  to,
  planner,
  start,
  link,
  visit,
}: {
  route: Route;
  coordinates: Position[];
  surface: SurfaceRange[] | undefined;
  change: RouteChange;
  to: string;
  /** Whether the reader edits plans; every row then keeps room for a published plan's pencil. */
  planner: boolean;
  start: StartDistance;
  link: RowLink;
  visit: RouteVisit;
}) {
  const key = routeKey(route);
  return (
    <li
      data-active={link.active || undefined}
      onMouseEnter={() => link.onActivate(key)}
      className="group relative border-[var(--panel)] border-b-2 last:border-b-0 data-active:bg-[color-mix(in_oklab,var(--accent)_12%,transparent)]"
    >
      <Link
        to={to}
        state={visit}
        onFocus={() => link.onActivate(key)}
        className={`${planner ? "pr-12 " : ""}relative grid ${start === null ? "grid-cols-[2.5rem_minmax(0,1fr)_5.5rem_5.5rem_5rem_4rem]" : "grid-cols-[2.5rem_minmax(0,1fr)_5.5rem_5.5rem_5rem_4rem_5rem]"} items-center gap-x-4 px-3 py-2.5 text-sm tabular-nums before:absolute before:inset-1 before:rounded-[7px] hover:before:bg-[color-mix(in_oklab,var(--ink-2)_8%,transparent)]`}
      >
        <span className="relative block size-10">
          <RouteGlyph
            coordinates={coordinates}
            title={route.title}
            band={gradientBand(route.maxGradientPercent)}
          />
        </span>
        <span className="relative flex min-w-0 flex-col gap-1.5">
          <span className="truncate font-semibold">
            {route.title} {change === null ? null : <RouteChangeBadge change={change} />}
          </span>
          <RouteMixBars coordinates={coordinates} surface={surface} />
        </span>
        <span className="relative text-right font-semibold">
          {formatDistance(route.distanceMetres)}
        </span>
        <span className="relative text-right" title={formatMovingTimeUncertainty(route.validation)}>
          {formatMovingTime(route.movingSeconds)}
        </span>
        <span className="relative text-right">{formatAscent(route.ascentMetres)}</span>
        <span className="relative text-right text-[var(--ink-2)]">
          {formatGradient(route.maxGradientPercent)}
        </span>
        {start === null ? null : (
          <span className="relative text-right text-[var(--ink-2)]" title="Distance to start">
            {formatStart(start)}
          </span>
        )}
      </Link>
      {planner ? (
        <EditPlanButton route={route} className="absolute top-1/2 right-2 z-10 -translate-y-1/2" />
      ) : null}
    </li>
  );
}

/**
 * One route, as a card, where a ledger row will not fit.
 *
 * Not the atlas's own search row: that row's verb is "select", which is a step
 * this page does not have, and its glyph column would stand empty without the
 * geometry this page does not fetch.
 */
function CatalogueCard({
  route,
  coordinates,
  change,
  planner,
  start,
  visit,
}: {
  route: Route;
  coordinates: Position[];
  change: RouteChange;
  planner: boolean;
  start: StartDistance;
  visit: RouteVisit;
}) {
  const where = secondName(route);

  return (
    <li className="group relative">
      <Link
        to={routePath(route)}
        state={visit}
        className={`${planner && planEditLink(route) !== null ? "pr-12 " : ""}flex items-start gap-3 rounded-lg border border-[var(--rule)] p-3 hover:bg-[var(--base)] focus-visible:outline-2 focus-visible:outline-offset-2 focus-visible:outline-[var(--accent)]`}
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
          {change === null && where === null ? null : (
            <span className="mt-0.5 flex flex-wrap items-center gap-x-2 text-xs text-[var(--ink-2)]">
              <RouteChangeBadge change={change} />
              {where}
            </span>
          )}
          <span className="mt-2 flex flex-wrap gap-x-3 gap-y-1 text-xs text-[var(--ink-2)] tabular-nums">
            <span>{formatDistance(route.distanceMetres)}</span>
            <span>{formatAscent(route.ascentMetres)}</span>
            <span>{formatGradient(route.maxGradientPercent)}</span>
            <span>{formatMovingTime(route.movingSeconds)}</span>
            {start === null ? null : <span>{formatStart(start)} to start</span>}
          </span>
        </span>
      </Link>
      {planner ? <EditPlanButton route={route} className="absolute top-2 right-2" /> : null}
    </li>
  );
}

/** Routes, distance, and the two records worth naming, over the whole library. */
function Totals({ library }: { library: Route[] }) {
  if (library.length === 0) {
    return null;
  }
  const distanceMetres = library.reduce((sum, route) => sum + route.distanceMetres, 0);
  const longest = [...library].sort((a, b) => b.distanceMetres - a.distanceMetres)[0];
  const hilliest = [...library].sort((a, b) => b.ascentMetres - a.ascentMetres)[0];
  const rows: Array<[string, string]> = [
    ["Routes", String(library.length)],
    ["Distance in all", formatDistance(distanceMetres)],
    ["Longest", longest ? `${longest.title} · ${formatDistance(longest.distanceMetres)}` : "–"],
    [
      "Most climbing",
      hilliest ? `${hilliest.title} · ${formatAscent(hilliest.ascentMetres)}` : "–",
    ],
  ];

  return (
    <Panel icon={<IconChartBar size={18} stroke={1.8} aria-hidden="true" />} title="The library">
      <dl className="flex flex-col divide-y divide-[var(--rule)] text-sm">
        {rows.map(([name, value]) => (
          <div key={name} className="flex justify-between gap-3 py-2 first:pt-0 last:pb-0">
            <dt className="text-[var(--ink-2)]">{name}</dt>
            <dd className="truncate text-right font-medium tabular-nums">{value}</dd>
          </div>
        ))}
      </dl>
    </Panel>
  );
}

/** The four routes revised most recently, newest first, where a revision date parses at all. */
function Recent({
  library,
  shapeOf,
  changeOf,
  visit,
}: {
  library: Route[];
  shapeOf: (route: Route) => Position[];
  changeOf: (route: Route) => RouteChange;
  visit: RouteVisit;
}) {
  const recent = useMemo(
    () =>
      library
        .filter((route) => !Number.isNaN(Date.parse(route.sourceRevision)))
        .sort((a, b) => Date.parse(b.sourceRevision) - Date.parse(a.sourceRevision))
        .slice(0, 4),
    [library],
  );
  if (recent.length === 0) {
    return null;
  }

  return (
    <Panel
      icon={<IconClockEdit size={18} stroke={1.8} aria-hidden="true" />}
      title="Recently updated"
    >
      <ul className="flex flex-col overflow-hidden rounded-[11px] bg-[color-mix(in_oklab,var(--ink-2)_7%,transparent)]">
        {recent.map((route) => {
          const change = changeOf(route);
          return (
            <li
              key={routeKey(route)}
              className="flex items-center gap-3 border-[var(--panel)] border-b-2 px-3 py-2 last:border-b-0"
            >
              <span className="block size-8 shrink-0">
                <RouteGlyph
                  coordinates={shapeOf(route)}
                  title={route.title}
                  band={gradientBand(route.maxGradientPercent)}
                />
              </span>
              <span className="flex min-w-0 flex-1 flex-col text-sm">
                <Link
                  to={routePath(route)}
                  state={visit}
                  className="truncate font-medium hover:underline"
                >
                  {route.title}
                </Link>
                <span className="text-[var(--ink-2)] text-xs">
                  {change === "new"
                    ? "New"
                    : change === "updated"
                      ? "Updated"
                      : formatTimestamp(route.sourceRevision)}
                </span>
              </span>
            </li>
          );
        })}
      </ul>
    </Panel>
  );
}

export interface CataloguePageProps {
  /** The reader's colour-scheme pick. Held by `App` — see there for why. */
  themeChoice?: ThemeChoice;
}

export function CataloguePage({ themeChoice = "system" }: CataloguePageProps) {
  const routes = useQuery(routesQuery());
  const status = useQuery(statusQuery());
  const config = useQuery(webUIConfigQuery());
  // Only an admin on a planning deployment has drafts, and edits published plans.
  const planner = useEffectiveAdmin() && config.data?.planning === true;
  const [shelf, setShelf] = useState<"library" | "drafts">("library");
  const drafted = useDrafts(planner);
  const onDrafts = planner && shelf === "drafts";
  // Read, never written: a row is not an opened route, so nothing here marks a
  // stage seen. Only the atlas does, from the moment a route's own panel shows.
  const { changeOf } = useSeenRoutes();
  const narrow = useNarrowViewport();
  // Tailwind's `lg`, where the sidebar stands beside the table; below it the map is not mounted at all.
  const wide = useMediaQuery("(min-width: 64rem)");
  // Open by default on a wide screen, closed on a narrow one; the reader's own
  // later toggling is never revisited when the viewport itself changes.
  const [filtersOpen, setFiltersOpen] = useState(() => !narrow);
  const prefersDark = usePrefersDarkScheme();
  const [basemapChoice] = useBasemapChoice();
  // Never stored or sent: it only measures how far each start is from here.
  const location = useStartupLocation(true);
  const [chosenKey, setChosenKey] = useState<string | null>(null);

  const [params, setParams] = useSearchParams();
  const view = useMemo(() => readView(params), [params]);
  const visit = useMemo<RouteVisit>(() => ({ catalogue: `?${params}` }), [params]);

  /*
   * What is in the search field, held here as well as in the address. See the
   * effect below: the field reads from here, which changes as fast as it is
   * typed, and the address follows rather than leads — a keystroke reaching
   * the address through the router and back would drop letters typed faster
   * than that round trip.
   */
  const [typed, setTyped] = useState(view.query);
  useEffect(() => {
    setTyped(view.query);
  }, [view.query]);

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
   * so whichever page the reader opens first pays for both.
   */
  const combine = useCallback(
    (results: Array<UseQueryResult<RouteGeometry>>) => {
      const shapes = new Map<string, Position[]>();
      const ranges = new Map<string, SurfaceRange[]>();
      const boxes = new Map<string, BoundingBox>();
      library.forEach((route, index) => {
        const geometry = results[index]?.data;
        if (!geometry) {
          return;
        }
        const key = routeKey(route);
        shapes.set(key, geometry.coordinates);
        boxes.set(key, geometry.bbox);
        if (geometry.surface && geometry.surface.matchedMetres > 0) {
          ranges.set(key, geometry.surface.ranges);
        }
      });

      return { shapes, ranges, boxes };
    },
    [library],
  );

  const drawn = useQueries({
    queries: library.map((route) =>
      routeGeometryQuery(route.provider, route.sourceRouteId, route.stageOrder),
    ),
    combine,
  });
  const shapeOf = useCallback(
    (route: Route) => drawn.shapes.get(routeKey(route)) ?? [],
    [drawn.shapes],
  );

  const startOf = useCallback(
    (route: Route): StartDistance => {
      if (location === null) {
        return null;
      }
      const first = drawn.shapes.get(routeKey(route))?.[0];
      return first ? haversineMetres(location, first) : undefined;
    },
    [location, drawn.shapes],
  );

  const shown = useMemo(
    () =>
      sortRoutes(
        matchingRoutes(library, view.query).filter((route) => matchesFilters(route, view.filters)),
        view.sort,
        view.direction,
        (route) => startOf(route) ?? undefined,
      ),
    [library, view.query, view.filters, view.sort, view.direction, startOf],
  );

  const shownDrafts = drafted.drafts.filter(({ plan }) => matchesText(plan.name, view.query));
  // The row pointed at last, while the shelf still shows it; else its first row.
  const candidates: Array<{ key: string; coordinates: Position[] | undefined }> = onDrafts
    ? shownDrafts.map(({ plan, coordinates }) => ({ key: draftKey(plan.id), coordinates }))
    : shown.map((route) => ({
        key: routeKey(route),
        coordinates: drawn.shapes.get(routeKey(route)),
      }));
  const active =
    candidates.find((candidate) => candidate.key === chosenKey) ?? candidates[0] ?? null;
  const activeKey = active?.key ?? null;
  const activeCoordinates = active?.coordinates;
  const lines = useMemo<MapLine[]>(
    () =>
      activeKey && activeCoordinates && activeCoordinates.length > 1
        ? [{ key: activeKey, coordinates: activeCoordinates }]
        : [],
    [activeKey, activeCoordinates],
  );
  // A draft's listing carries no box, so its line is measured; memoised, or every render would fly the camera.
  const bounds = useMemo(
    () =>
      activeKey === null
        ? null
        : (drawn.boxes.get(activeKey) ??
          (activeCoordinates
            ? rangeBounds(activeCoordinates, {
                startIndex: 0,
                endIndex: activeCoordinates.length - 1,
              })
            : null)),
    [activeKey, activeCoordinates, drawn.boxes],
  );
  const basemap = config.data
    ? basemapFor(config.data, resolvesDark(themeChoice, prefersDark), basemapChoice)
    : null;
  const linkOf = (route: Route): RowLink => ({
    active: routeKey(route) === activeKey,
    onActivate: setChosenKey,
  });

  const hasQuery = view.query.trim() !== "";
  const filtersActive = hasActiveFilters(view.filters);
  const sortedLabel = SORT_COLUMNS.find((entry) => entry.column === view.sort)?.label ?? "Route";
  const narrowed = shown.length !== library.length;
  const readAt = status.data?.sync.phases.source?.lastCompletedAt;
  const counted = narrowed
    ? `${shown.length} of ${formatCount(library.length, "route")}`
    : formatCount(library.length, "route");
  const subtitle = readAt ? `${counted} · read ${formatReadTime(readAt)}` : counted;

  return (
    <PageShell>
      <div className="mx-auto flex w-full max-w-[1400px] flex-col gap-5">
        <header className="flex flex-wrap items-center justify-between gap-3">
          <h1 className="font-semibold text-2xl tracking-tight">Catalogue</h1>
          <span className="flex items-center gap-2">
            <SoftSearch
              typed={typed}
              onChange={(value) => {
                setTyped(value);
                update(() => ({ query: value }));
              }}
            />
            {/* The filters measure routes; a draft carries none of what they bound. */}
            {onDrafts ? null : (
              <FiltersToggle
                open={filtersOpen}
                onOpen={setFiltersOpen}
                count={activeFilterCount(view.filters)}
              />
            )}
          </span>
        </header>
        <div className="flex flex-col gap-5 lg:grid lg:items-start lg:grid-cols-[minmax(0,1fr)_22rem]">
          <div className="order-2 flex min-w-0 flex-col gap-4 lg:order-1">
            <Panel
              icon={<IconBooks size={18} stroke={1.8} aria-hidden="true" />}
              title={onDrafts ? "Drafts" : "Library"}
              subtitle={onDrafts ? formatCount(shownDrafts.length, "draft") : subtitle}
              aside={
                <span className="flex flex-wrap items-center justify-end gap-2">
                  {onDrafts ? null : (
                    <SortControl view={view} sortBy={sortBy} nearby={location !== null} />
                  )}
                  {planner ? (
                    <Segmented
                      label="Shelf"
                      size="sm"
                      items={[
                        { key: "library", label: "Library" },
                        { key: "drafts", label: `Drafts · ${drafted.drafts.length}` },
                      ]}
                      value={shelf}
                      onChange={setShelf}
                    />
                  ) : null}
                </span>
              }
            >
              {onDrafts ? (
                <>
                  {drafted.isError ? (
                    <Alert variant="destructive">
                      <AlertTitle>Could not load the drafts.</AlertTitle>
                    </Alert>
                  ) : drafted.isPending ? null : (
                    <DraftList
                      drafts={shownDrafts}
                      searched={shownDrafts.length < drafted.drafts.length}
                      narrow={narrow}
                      activeKey={activeKey}
                      onActivate={setChosenKey}
                    />
                  )}
                </>
              ) : (
                <>
                  <p className="sr-only">
                    {`The route library, ranked by ${sortedLabel.toLowerCase()}, ${
                      view.direction === "asc" ? "ascending" : "descending"
                    }`}
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
                    <p className="text-[var(--ink-2)] text-sm">
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
                          coordinates={shapeOf(route)}
                          change={changeOf(route)}
                          planner={planner}
                          start={startOf(route)}
                          visit={visit}
                        />
                      ))}
                    </ul>
                  ) : (
                    <ul className="flex flex-col overflow-hidden rounded-[11px] bg-[color-mix(in_oklab,var(--ink-2)_7%,transparent)]">
                      {shown.map((route) => (
                        <LedgerRow
                          key={routeKey(route)}
                          route={route}
                          coordinates={shapeOf(route)}
                          surface={drawn.ranges.get(routeKey(route))}
                          change={changeOf(route)}
                          to={routePath(route)}
                          planner={planner}
                          start={startOf(route)}
                          link={linkOf(route)}
                          visit={visit}
                        />
                      ))}
                    </ul>
                  )}
                </>
              )}
            </Panel>
          </div>
          <div className="order-1 flex flex-col gap-5 lg:order-2 lg:sticky lg:top-20">
            {basemap && wide && lines.length > 0 ? (
              <div className="h-64 overflow-hidden rounded-2xl shadow-[var(--shadow)]">
                <LibraryMap
                  styleUrl={basemap.styleUrl}
                  darkBasemap={basemap.dark}
                  lines={lines}
                  pickedKey={activeKey}
                  bounds={bounds}
                  controls={false}
                />
              </div>
            ) : null}
            {filtersOpen && !onDrafts ? (
              <CatalogueFilters
                library={library}
                filters={view.filters}
                onFiltersChange={(next) => update(() => ({ filters: next }))}
              />
            ) : null}
            <Totals library={library} />
            <Recent library={library} shapeOf={shapeOf} changeOf={changeOf} visit={visit} />
          </div>
        </div>
      </div>
    </PageShell>
  );
}
