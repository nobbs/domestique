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
import { InputGroup, InputGroupAddon, InputGroupInput } from "../../components/ui/input-group";
import type { LibraryFilters } from "../../lib/filters";
import { hasActiveFilters } from "../../lib/filters";
import type { RouteChange } from "../../lib/seenRoutes";
import { FilterPanel } from "./FilterPanel";
import { ResultRow } from "./ResultRow";
import { RouteCard } from "./RouteCard";

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
  pickedKey: string | null;
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
  changeOf: (route: Route) => RouteChange;
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
  pickedKey,
  onSelect,
  onOpen,
  shapes,
  readAt,
  changeOf,
}: SearchPanelProps) {
  const filtersActive = hasActiveFilters(filters);
  const hasQuery = query.trim() !== "";
  const [searchExpanded, setSearchExpanded] = useState(
    hasQuery || pickedKey !== null || filtersActive,
  );
  const [focusSearch, setFocusSearch] = useState(false);
  const field = useRef<HTMLInputElement>(null);

  // A route picked from the map still needs its card to appear in this panel.
  // Typing only becomes possible after the reader has explicitly opened search.
  useEffect(() => {
    if (hasQuery || pickedKey !== null) {
      setSearchExpanded(true);
    }
  }, [hasQuery, pickedKey]);

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
  const pickedRoute = pickedKey
    ? (shown.find((route) => routeKey(route) === pickedKey) ?? null)
    : null;

  const compactWorkspace = !searchExpanded && !hasResults && pickedRoute === null;
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
          <Button
            variant="panel"
            icon={<IconSearch stroke={1.6} />}
            aria-label="Search the route library"
            onClick={() => {
              setSearchExpanded(true);
              setFocusSearch(true);
            }}
          />
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
      {hasResults || pickedRoute ? (
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

                return key === pickedKey ? (
                  <RouteCard
                    key={key}
                    route={route}
                    shape={shape}
                    readAt={readAt}
                    change={changeOf(route)}
                    onOpen={() => onOpen(key)}
                  />
                ) : (
                  <ResultRow
                    key={key}
                    route={route}
                    shape={shape}
                    change={changeOf(route)}
                    onSelect={() => onSelect(key)}
                  />
                );
              })}
            </ul>
          ) : null}
          {!hasResults && pickedRoute ? (
            <ul className="grid gap-1">
              <RouteCard
                route={pickedRoute}
                shape={shapes.get(routeKey(pickedRoute))}
                readAt={readAt}
                change={changeOf(pickedRoute)}
                onOpen={() => onOpen(routeKey(pickedRoute))}
              />
            </ul>
          ) : null}
        </div>
      ) : null}
    </div>
  );
}
