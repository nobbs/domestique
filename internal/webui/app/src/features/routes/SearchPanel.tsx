/**
 * The entry page's two ways of narrowing the library: by name and by what a
 * route measures.
 *
 * At rest the pill says how many routes there are. Typed into, or filtered,
 * it grows a results column beneath itself; picking a result replaces that
 * row with the route's card, so the column never says the same route twice
 * and nothing opens beside anything else. The card's one button opens the
 * route, which swaps this panel for the route's own — there is no route page
 * to leave for.
 *
 * Both happen here in the browser, over the listing and geometry the page
 * already holds — see `lib/library.ts` and `lib/filters.ts`. Nothing a reader
 * types is sent to the service, which keeps route names out of an access log.
 */

import { IconSearch } from "@tabler/icons-react";
import { useEffect, useRef, useState } from "react";
import type { Position, Route } from "../../api/types";
import { routeKey } from "../../api/types";
import { Button } from "../../components/Button";
import { RouteGlyph } from "../../components/route/RouteGlyph";
import { InputGroup, InputGroupAddon, InputGroupInput } from "../../components/ui/input-group";
import type { LibraryFilters } from "../../lib/filters";
import { hasActiveFilters } from "../../lib/filters";
import {
  formatAscent,
  formatDistance,
  formatGradient,
  formatMovingTime,
  formatMovingTimeUncertainty,
} from "../../lib/format";
import { gradientBand, gradientMix } from "../../lib/profile";
import { providerLabel } from "../../lib/provider";
import type { StageChange } from "../../lib/seenStages";
import type { UnitSystem } from "../../lib/units";
import { FilterPanel } from "./FilterPanel";

/** The geometry a row needs, when it has arrived. Rows render without it. */
export interface RouteShape {
  coordinates: Position[];
}

export interface SearchPanelProps {
  /** What the search and the filters left, in the order it is read. */
  shown: Route[];
  /** The whole library, which is what the pill counts. */
  total: number;
  query: string;
  onQueryChange: (query: string) => void;
  filters: LibraryFilters;
  onFiltersChange: (filters: LibraryFilters) => void;
  filtersExpanded: boolean;
  onFiltersExpandedChange: (expanded: boolean) => void;
  /** The expanded route, by `routeKey`. */
  selectedKey: string | null;
  onSelect: (key: string | null) => void;
  /**
   * Opens a route, which swaps this whole panel for the route's own.
   *
   * Two steps rather than one: a row picked out of the column is a route the
   * reader is looking at on the map, and swapping the search away the moment
   * they touch a row would take the column with it — including the rows they
   * were comparing this one against.
   */
  onOpen: (key: string) => void;
  /** Route shapes by `routeKey`, for the glyphs and the mix bar. */
  shapes: Map<string, RouteShape>;
  /**
   * When the library was last read, for the card's second line.
   *
   * One timestamp for the whole library rather than one per route: the service
   * reads the library in a single pass, so every route in it was read at the
   * same moment and a per-route time would be the same time repeated.
   */
  readAt: string | null;
  /** Whether a route is new or changed since this reader last opened it. */
  changeOf: (route: Route) => StageChange;
  /** The units the figures report distance and elevation in. */
  unitSystem: UnitSystem;
}

/** New or changed since this reader last opened it. Text, never colour alone. */
function StageChangeBadge({ change }: { change: StageChange }) {
  if (!change) {
    return null;
  }

  return (
    <span
      className="rounded-full bg-[var(--base)] px-1.5 py-0.5 text-[11px] font-semibold text-[var(--ink-2)]"
      data-change={change}
    >
      {change === "new" ? "New" : "Updated"}
    </span>
  );
}

/** One route, closed: its shape, its name, and the figures that rank it. */
function ResultRow({
  route,
  shape,
  change,
  onSelect,
  unitSystem,
}: {
  route: Route;
  shape: RouteShape | undefined;
  change: StageChange;
  onSelect: () => void;
  unitSystem: UnitSystem;
}) {
  return (
    <li>
      <button
        className="grid w-full grid-cols-[2.5rem_1fr_auto] items-center gap-x-2 gap-y-1 rounded-lg p-2 text-left text-sm hover:bg-[var(--base)] focus-visible:outline-2 focus-visible:outline-offset-2 focus-visible:outline-[var(--accent)]"
        type="button"
        onClick={onSelect}
      >
        <span className="row-span-2 flex size-10 items-center justify-center">
          <RouteGlyph
            coordinates={shape?.coordinates ?? []}
            title={route.title}
            band={gradientBand(route.maxGradientPercent)}
          />
        </span>
        <span className="min-w-0 truncate font-semibold">{route.title}</span>
        <StageChangeBadge change={change} />
        <span className="col-start-2 col-end-4 flex flex-wrap gap-x-2 gap-y-1 text-xs text-[var(--ink-2)] tabular-nums">
          <span>{formatDistance(route.distanceMetres, unitSystem)}</span>
          <span>{formatAscent(route.ascentMetres, unitSystem)}</span>
          <span>{formatGradient(route.maxGradientPercent)}</span>
          <span>
            {formatMovingTime(route.movingSeconds)}
            {route.movingSeconds !== undefined && route.validation ? (
              <span className="ml-1">{formatMovingTimeUncertainty(route.validation)}</span>
            ) : null}
          </span>
        </span>
      </button>
    </li>
  );
}

/**
 * One route, opened.
 *
 * Where it is, is the route's own name where that is not already the title: the
 * service stores no locality, and asking a geocoder for one would send the
 * library's coordinates outside the Tailnet to answer a question the operator's
 * own naming already answers.
 */
function RouteCard({
  route,
  shape,
  readAt,
  change,
  onOpen,
  unitSystem,
}: {
  route: Route;
  shape: RouteShape | undefined;
  readAt: string | null;
  change: StageChange;
  onOpen: () => void;
  unitSystem: UnitSystem;
}) {
  /*
   * The card brings itself into view when it appears.
   *
   * A route can now be picked by pointing at it on the map, and the column it
   * expands in is not necessarily scrolled anywhere near it: without this, a
   * click on the ground would answer somewhere the reader cannot see. `nearest`
   * so a card picked out of the column, which is already on screen, does not
   * make the column jump under the hand that picked it.
   */
  const card = useRef<HTMLLIElement>(null);
  useEffect(() => {
    card.current?.scrollIntoView({ block: "nearest" });
  }, []);

  const where = route.routeName !== route.title ? route.routeName : null;
  const second = [where, readAt ? `read ${readAt}` : null].filter(Boolean).join(" · ");
  const mix = shape ? gradientMix(shape.coordinates) : [];
  // A run's place along the route is its identity — two runs of the same band
  // are the same band at different kilometres — so the key is where the run
  // starts rather than which band it is.
  let offset = 0;
  const runs = mix.map((entry) => {
    const start = offset;
    offset += entry.share;

    return { ...entry, start };
  });

  return (
    <li className="rounded-lg border border-[var(--rule)] bg-[var(--base)] p-3" ref={card}>
      <h2 className="text-base font-semibold">{route.title}</h2>
      <StageChangeBadge change={change} />
      <span className="ml-1 text-xs font-semibold tracking-[0.06em] text-[var(--ink-2)] uppercase">
        {providerLabel(route.provider)}
      </span>
      {second === "" ? null : <p className="mt-1 text-sm text-[var(--ink-2)]">{second}</p>}
      <dl className="mt-3 grid grid-cols-2 gap-x-4 gap-y-2 text-sm sm:grid-cols-4">
        <div>
          <dt>Distance</dt>
          <dd>{formatDistance(route.distanceMetres, unitSystem)}</dd>
        </div>
        <div>
          <dt>Climbing</dt>
          <dd>{formatAscent(route.ascentMetres, unitSystem)}</dd>
        </div>
        <div>
          <dt>Max</dt>
          <dd>{formatGradient(route.maxGradientPercent)}</dd>
        </div>
        <div>
          <dt>Moving time</dt>
          <dd>
            {formatMovingTime(route.movingSeconds)}
            {route.movingSeconds !== undefined && route.validation ? (
              <span className="ml-1 text-xs text-[var(--ink-2)]">
                {formatMovingTimeUncertainty(route.validation)}
              </span>
            ) : null}
          </dd>
        </div>
      </dl>
      {mix.length > 0 ? (
        // Decorative: every band in it is stated in the key on the route's own
        // page, and a reader who cannot see the colours loses nothing here that
        // the three figures above have not already said.
        <div
          className="mt-3 flex h-1.5 overflow-hidden rounded-sm"
          data-testid="gradient-mix"
          aria-hidden="true"
        >
          {runs.map((entry) => (
            <span
              key={`${entry.start.toFixed(6)}-${entry.band}`}
              data-band={entry.band}
              style={{ width: `${(entry.share * 100).toFixed(3)}%` }}
            />
          ))}
        </div>
      ) : null}
      {/*
       * The one primary control in this view. It opens the route in place —
       * there is no route page to go to — so it says what it will show rather
       * than where it will take anybody.
       */}
      <Button className="mt-3" variant="primary" onClick={onOpen}>
        Open route
      </Button>
    </li>
  );
}

export function SearchPanel({
  shown,
  total,
  query,
  onQueryChange,
  filters,
  onFiltersChange,
  filtersExpanded,
  onFiltersExpandedChange,
  selectedKey,
  onSelect,
  onOpen,
  shapes,
  readAt,
  changeOf,
  unitSystem,
}: SearchPanelProps) {
  const filtersActive = hasActiveFilters(filters);
  const hasQuery = query.trim() !== "";
  const [searchExpanded, setSearchExpanded] = useState(
    hasQuery || selectedKey !== null || filtersActive,
  );
  const [focusSearch, setFocusSearch] = useState(false);
  const field = useRef<HTMLInputElement>(null);

  // A route picked from the map still needs its card to appear in this panel.
  // Typing only becomes possible after the reader has explicitly opened search.
  useEffect(() => {
    if (hasQuery || selectedKey !== null) {
      setSearchExpanded(true);
    }
  }, [hasQuery, selectedKey]);

  useEffect(() => {
    if (focusSearch && searchExpanded) {
      field.current?.focus();
      setFocusSearch(false);
    }
  }, [focusSearch, searchExpanded]);

  // A filter narrows the library exactly as a typed word does, so it opens the
  // same results column and counts against the same total. Selecting from the
  // map is different: it names one route, not a reason to list the library.
  const hasResults = hasQuery || filtersActive;
  const selectedRoute = selectedKey
    ? (shown.find((route) => routeKey(route) === selectedKey) ?? null)
    : null;

  const compactWorkspace = !searchExpanded && !hasResults && selectedRoute === null;
  const workspaceWidth = compactWorkspace ? "w-fit" : "w-[32.5rem] max-w-full";

  return (
    <div
      className={`flex flex-col gap-3 ${workspaceWidth}`}
      data-compact-workspace={compactWorkspace ? "" : undefined}
    >
      <div className="flex items-center gap-2">
        {searchExpanded ? (
          <InputGroup className="bg-[var(--panel)]">
            <InputGroupAddon>
              <IconSearch size={16} stroke={1.6} aria-hidden="true" />
            </InputGroupAddon>
            <InputGroupInput
              ref={field}
              type="search"
              value={query}
              onChange={(event) => onQueryChange(event.target.value)}
              placeholder={`Search ${total} ${total === 1 ? "route" : "routes"}`}
              aria-label="Search the route library"
            />
          </InputGroup>
        ) : (
          <button
            className="inline-flex size-8 items-center justify-center rounded-lg border border-[var(--rule)] bg-[var(--panel)] text-[var(--ink-2)] hover:text-[var(--ink)] focus-visible:outline-2 focus-visible:outline-offset-2 focus-visible:outline-[var(--accent)]"
            type="button"
            aria-label="Search the route library"
            onClick={() => {
              setSearchExpanded(true);
              setFocusSearch(true);
            }}
          >
            <IconSearch size={16} stroke={1.6} aria-hidden="true" />
          </button>
        )}
        <FilterPanel
          filters={filters}
          onFiltersChange={(next) => {
            onFiltersChange(next);
            setSearchExpanded(true);
          }}
          expanded={filtersExpanded}
          onExpandedChange={onFiltersExpandedChange}
        />
      </div>
      {hasResults || selectedRoute ? (
        <div className="rounded-lg border border-[var(--rule)] bg-[var(--panel)] p-2 shadow-[var(--shadow)]">
          {hasResults && shown.length === 0 ? (
            <p className="p-2 text-sm text-[var(--ink-2)]">
              {/*
               * Whichever of the two actually narrowed the library to nothing —
               * or, typed and filtered at once, both. Blaming a filter for what a
               * misremembered name caused, or the reverse, points the reader
               * at the wrong control to relax.
               */}
              {hasQuery && filtersActive
                ? "Nothing here matches this search and these filters."
                : filtersActive
                  ? "Nothing here matches these filters."
                  : "Nothing here is called that."}
            </p>
          ) : null}
          {hasResults && shown.length > 0 ? (
            <ul className="grid gap-1">
              {shown.map((route) => {
                const key = routeKey(route);
                const shape = shapes.get(key);

                return key === selectedKey ? (
                  <RouteCard
                    key={key}
                    route={route}
                    shape={shape}
                    readAt={readAt}
                    change={changeOf(route)}
                    onOpen={() => onOpen(key)}
                    unitSystem={unitSystem}
                  />
                ) : (
                  <ResultRow
                    key={key}
                    route={route}
                    shape={shape}
                    change={changeOf(route)}
                    onSelect={() => onSelect(key)}
                    unitSystem={unitSystem}
                  />
                );
              })}
            </ul>
          ) : null}
          {!hasResults && selectedRoute ? (
            <ul className="grid gap-1">
              <RouteCard
                route={selectedRoute}
                shape={shapes.get(routeKey(selectedRoute))}
                readAt={readAt}
                change={changeOf(selectedRoute)}
                onOpen={() => onOpen(routeKey(selectedRoute))}
                unitSystem={unitSystem}
              />
            </ul>
          ) : null}
        </div>
      ) : null}
    </div>
  );
}
