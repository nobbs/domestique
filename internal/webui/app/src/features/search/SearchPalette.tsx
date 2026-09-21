/**
 * The search palette: the one way to find a route, from any page.
 *
 * ⌘K or the menu bar's Search button opens it. One field takes names and the
 * query language in `lib/query.ts` (`dist:40-80 by distance`), with completions
 * for its tokens; on a wide screen the highlighted row is mapped beside the list.
 * An admin on a deployment that plans also finds their drafts here, marked as
 * such, which open in the planner. The query outlasts closing the palette.
 *
 * Nothing is fetched for opening it beyond the listing: the preview asks for the
 * one highlighted route's line, and only ranking by distance to start asks for
 * the reader's position and every route's line.
 */

import { Dialog as DialogPrimitive } from "@base-ui/react/dialog";
import {
  IconArrowDown,
  IconArrowUp,
  IconCornerDownLeft,
  IconSearch,
  IconX,
} from "@tabler/icons-react";
import type { UseQueryResult } from "@tanstack/react-query";
import { useQueries, useQuery } from "@tanstack/react-query";
import { useCallback, useEffect, useMemo, useRef, useState } from "react";
import { useLocation, useNavigate } from "react-router";
import { getGetPlanQueryOptions, getListPlansQueryOptions } from "../../api/generated";
import { routeGeometryQuery, routesQuery, webUIConfigQuery } from "../../api/queries";
import type { Position, Route, RouteGeometry } from "../../api/types";
import { routeKey } from "../../api/types";
import { librarySources } from "../../components/SourceChips";
import { Badge } from "../../components/ui/badge";
import { Dialog, DialogOverlay, DialogPortal } from "../../components/ui/dialog";
import { basemapFor, useBasemapChoice, usePrefersDarkScheme } from "../../lib/basemap";
import { hasActiveFilters, matchesFilters } from "../../lib/filters";
import { formatAscent, formatDistance, formatMovingTime } from "../../lib/format";
import { useEffectiveAdmin } from "../../lib/identity";
import { matchesText, matchingRoutes, routePath } from "../../lib/library";
import { useMediaQuery } from "../../lib/mediaQuery";
import { haversineMetres, rangeBounds } from "../../lib/profile";
import { providerLabel } from "../../lib/provider";
import { joinQuery, parseQuery, splitQuery, suggest, withoutToken } from "../../lib/query";
import { sortRoutes } from "../../lib/ranking";
import { ownsShortcut, useSearchPalette } from "../../lib/searchPalette";
import { useStartupLocation } from "../../lib/startupLocation";
import { resolvesDark, type ThemeChoice } from "../../lib/theme";
import { LibraryMap } from "../routes/LibraryMap";

/** One row the palette can open: a published route, or one of the admin's drafts. */
interface Entry {
  key: string;
  title: string;
  to: string;
  route: Route | null;
  draftId: number | null;
  distanceMetres: number;
  ascentMetres: number;
  movingSeconds?: number;
  startMetres?: number;
}

/** A row's columns: the name, then its figures, and the distance to its start while ranked by it. */
const ROW_GRID = "grid grid-cols-[minmax(0,1fr)_4.5rem_4.5rem_4.5rem] gap-x-3";
const ROW_GRID_NEAREST = "grid grid-cols-[minmax(0,1fr)_4.5rem_4.5rem_4.5rem_4.5rem] gap-x-3";

/** Where a start lies, once its line has arrived; formatDistance reads zero as missing. */
function formatStart(metres: number): string {
  return metres === 0 ? "0 m" : formatDistance(metres);
}

export function SearchPalette({ themeChoice }: { themeChoice: ThemeChoice }) {
  const { pathname } = useLocation();
  const navigate = useNavigate();
  const { open, setOpen } = useSearchPalette();
  const shortcut = !ownsShortcut(pathname);
  const [query, setQuery] = useState("");
  // The query text is the one source of truth; every control edits its tokens.
  const parsed = useMemo(() => parseQuery(query), [query]);
  const { chips, draft } = useMemo(() => splitQuery(query), [query]);
  const { filters, order } = parsed;
  const [active, setActive] = useState(0);
  const field = useRef<HTMLInputElement>(null);
  const list = useRef<HTMLUListElement>(null);
  const wide = useMediaQuery("(min-width: 64rem)");

  const routes = useQuery({ ...routesQuery(), enabled: open });
  const library = useMemo(() => routes.data ?? [], [routes.data]);
  const config = useQuery(webUIConfigQuery());
  const planner = useEffectiveAdmin() && config.data?.planning === true;
  const plans = useQuery({ ...getListPlansQueryOptions(), enabled: open && planner });

  const nearest = order.some((key) => key.column === "start");
  // Never stored or sent: it only measures how far each start is from here.
  const location = useStartupLocation(open && nearest);
  const combine = useCallback(
    (results: Array<UseQueryResult<RouteGeometry>>) => {
      const starts = new Map<string, Position>();
      library.forEach((route, index) => {
        const first = results[index]?.data?.coordinates[0];
        if (first) {
          starts.set(routeKey(route), first);
        }
      });
      return starts;
    },
    [library],
  );
  const starts = useQueries({
    queries: library.map((route) => ({
      ...routeGeometryQuery(route.provider, route.sourceRouteId, route.stageOrder),
      enabled: open && nearest,
    })),
    combine,
  });
  const startOf = useCallback(
    (route: Route) => {
      const first = starts.get(routeKey(route));
      return location && first ? haversineMetres(location, first) : undefined;
    },
    [location, starts],
  );

  const filtersActive = hasActiveFilters(filters);
  const shown = useMemo<Entry[]>(() => {
    // Filters measure what a draft's listing does not carry, so a filtered search holds no drafts.
    const drafts =
      planner && parsed.drafts !== "none" && (parsed.drafts === "only" || !filtersActive)
        ? (plans.data?.data.plans ?? [])
        : [];
    const ranked =
      parsed.drafts === "only"
        ? []
        : // Least significant key first: each stable sort keeps the ties the next one leaves.
          order.reduceRight(
            (list, key) => sortRoutes(list, key.column, key.direction, startOf),
            matchingRoutes(library, parsed.words).filter((route) => matchesFilters(route, filters)),
          );

    return [
      ...drafts
        .filter((plan) => !plan.published && matchesText(plan.name, parsed.words))
        .sort((a, b) => Date.parse(b.updatedAt) - Date.parse(a.updatedAt))
        .map((plan) => ({
          key: `draft/${plan.id}`,
          title: plan.name,
          to: `/plan/${plan.id}`,
          route: null,
          draftId: plan.id,
          distanceMetres: plan.distanceMetres,
          ascentMetres: plan.ascentMetres,
        })),
      ...ranked.map((route) => {
        const start = startOf(route);
        return {
          key: routeKey(route),
          title: route.title,
          to: routePath(route),
          route,
          draftId: null,
          distanceMetres: route.distanceMetres,
          ascentMetres: route.ascentMetres,
          ...(route.movingSeconds === undefined ? {} : { movingSeconds: route.movingSeconds }),
          ...(start === undefined ? {} : { startMetres: start }),
        };
      }),
    ];
  }, [planner, filtersActive, plans.data, library, parsed, filters, order, startOf]);

  useEffect(() => {
    if (!shortcut) {
      return;
    }
    const onKey = (event: KeyboardEvent) => {
      if ((event.metaKey || event.ctrlKey) && event.key.toLowerCase() === "k") {
        event.preventDefault();
        setOpen((current) => !current);
      }
    };
    window.addEventListener("keydown", onKey);

    return () => window.removeEventListener("keydown", onKey);
  }, [shortcut, setOpen]);

  useEffect(() => {
    if (open) {
      setActive(0);
    }
  }, [open]);

  const clampedActive = shown.length === 0 ? 0 : Math.min(active, shown.length - 1);
  useEffect(() => {
    list.current
      ?.querySelector(`[data-index="${clampedActive}"]`)
      ?.scrollIntoView({ block: "nearest" });
  }, [clampedActive]);

  const current = shown[clampedActive] ?? null;
  const preview = open && wide && current !== null;
  const routeLine = useQuery({
    ...routeGeometryQuery(
      current?.route?.provider ?? "",
      current?.route?.sourceRouteId ?? 0,
      current?.route?.stageOrder ?? 0,
    ),
    enabled: preview && current?.route !== null,
  });
  const draftLine = useQuery(
    getGetPlanQueryOptions(current?.draftId ?? 0, {
      query: { enabled: preview && current?.draftId !== null },
    }),
  );
  const previewCoordinates = useMemo<Position[]>(
    () =>
      (current?.route
        ? routeLine.data?.coordinates
        : (draftLine.data?.data.geometry.coordinates as Position[] | undefined)) ?? [],
    [current?.route, routeLine.data, draftLine.data],
  );
  // Memoised on the line itself, or every render would fly the camera again.
  const previewBounds = useMemo(
    () =>
      current?.route
        ? (routeLine.data?.bbox ?? null)
        : rangeBounds(previewCoordinates, {
            startIndex: 0,
            endIndex: previewCoordinates.length - 1,
          }),
    [current?.route, routeLine.data, previewCoordinates],
  );
  const fresh = useMemo(
    () =>
      current && previewCoordinates.length > 1
        ? { lines: [{ key: current.key, coordinates: previewCoordinates }], bounds: previewBounds }
        : null,
    [current, previewCoordinates, previewBounds],
  );
  // The last line shown stays up while the next one loads: unmounting the map in the
  // gap would start a new one from the whole world for every row the reader moves to.
  const [held, setHeld] = useState<typeof fresh>(null);
  useEffect(() => {
    if (fresh) {
      setHeld(fresh);
    }
  }, [fresh]);
  // With no row highlighted there is nothing to preview, and no next line coming.
  const shownPreview = current ? (fresh ?? held) : null;
  const prefersDark = usePrefersDarkScheme();
  const [basemapChoice] = useBasemapChoice();
  const basemap = config.data
    ? basemapFor(config.data, resolvesDark(themeChoice, prefersDark), basemapChoice)
    : null;

  const sources = useMemo(() => librarySources(library), [library]);
  const suggestions = useMemo(
    () =>
      suggest(query, {
        providers: sources.map(({ provider, count }) => ({
          provider,
          count,
          label: provider === "local" ? "Planner" : providerLabel(provider),
        })),
        distances: library.map((route) => route.distanceMetres),
        ascents: library.map((route) => route.ascentMetres),
        durations: library.map((route) => route.movingSeconds ?? 0),
        drafts: planner,
      }),
    [query, sources, library, planner],
  );
  const [suggested, setSuggested] = useState(0);
  const offered = suggestions[suggested] ?? suggestions[0];

  const pick = (index: number) => {
    const target = shown[index];
    if (!target) {
      return;
    }
    setOpen(false);
    navigate(target.to);
  };
  const onKeyDown = (event: React.KeyboardEvent) => {
    // By physical key: on a Mac, Option turns the letter into another character.
    // The list answers keys typed into the query; the chips and completions keep their own.
    if (event.target !== field.current) {
      return;
    }
    if (event.key === "Tab" && offered) {
      event.preventDefault();
      setQuery(offered.query);
      setSuggested(0);
      return;
    }
    // Right has nowhere to take the caret at the end of the query, so it picks the next
    // completion there; Left steps back only while a later one is picked.
    const atEnd = field.current.selectionStart === draft.length;
    // Backspace in an empty field takes the last chip back into the text, to edit it.
    const last = chips[chips.length - 1];
    if (event.key === "Backspace" && draft === "" && last) {
      event.preventDefault();
      setQuery(joinQuery(chips.slice(0, -1), last.text));
      return;
    }
    if (event.key === "ArrowRight" && atEnd && suggestions.length > 1) {
      event.preventDefault();
      setSuggested((suggested + 1) % suggestions.length);
      return;
    }
    if (event.key === "ArrowLeft" && atEnd && suggested > 0) {
      event.preventDefault();
      setSuggested(suggested - 1);
      return;
    }
    if (event.key === "ArrowDown") {
      event.preventDefault();
      setActive(Math.min(clampedActive + 1, shown.length - 1));
    } else if (event.key === "ArrowUp") {
      event.preventDefault();
      setActive(Math.max(clampedActive - 1, 0));
    } else if (event.key === "Enter") {
      event.preventDefault();
      pick(clampedActive);
    }
  };

  return (
    <Dialog open={open} onOpenChange={setOpen}>
      <DialogPortal>
        <DialogOverlay />
        <DialogPrimitive.Popup
          aria-label="Search"
          initialFocus={field}
          className="fixed top-[8vh] left-1/2 z-50 flex h-fit max-h-[80vh] w-[40rem] max-w-[calc(100vw-2rem)] -translate-x-1/2 flex-col overflow-hidden rounded-xl bg-[var(--panel)] shadow-[var(--shadow)] outline-none lg:w-[64rem]"
          onKeyDown={onKeyDown}
        >
          {/* Not a label: it holds the chips' own buttons, and a label would press the first. */}
          {/* biome-ignore lint/a11y/noStaticElementInteractions: a pointer convenience; the field itself takes focus from the keyboard */}
          {/* biome-ignore lint/a11y/useKeyWithClickEvents: as above */}
          <div
            onClick={(event) => {
              if (event.target === event.currentTarget) {
                field.current?.focus();
              }
            }}
            className="flex cursor-text flex-wrap items-center gap-x-2 gap-y-1.5 border-[var(--rule)] border-b px-5 py-4"
          >
            <IconSearch size={20} stroke={1.8} className="text-[var(--ink-2)]" aria-hidden="true" />
            {chips.map((token, index) => (
              <span
                // The occurrence too: `src:` repeats, and two orders can read the same.
                key={`${token.text}#${chips.slice(0, index).filter((chip) => chip.text === token.text).length}`}
                className="flex items-center gap-1 rounded-[7px] bg-[var(--muted)] py-0.5 pr-1 pl-2 font-mono text-[var(--ink)] text-sm"
              >
                {token.text}
                <button
                  type="button"
                  aria-label={`Remove ${token.text}`}
                  onClick={() => {
                    setQuery((text) => withoutToken(text, token));
                    field.current?.focus();
                  }}
                  className="text-[var(--ink-2)] hover:text-[var(--ink)]"
                >
                  <IconX size={12} aria-hidden="true" />
                </button>
              </span>
            ))}
            <input
              ref={field}
              type="search"
              value={draft}
              onChange={(event) => {
                setQuery(joinQuery(chips, event.target.value));
                setActive(0);
                setSuggested(0);
              }}
              placeholder={
                chips.length === 0 ? "Route name or place, or dist:40-80 up:<1000 by distance" : ""
              }
              aria-label="Search the route library"
              aria-activedescendant={current ? `search-option-${current.key}` : undefined}
              className="min-w-32 flex-1 bg-transparent text-lg outline-none placeholder:text-[var(--ink-2)] [&::-webkit-search-cancel-button]:appearance-none"
            />
          </div>
          <div
            role="group"
            aria-label="Completions"
            className="flex min-h-10 flex-wrap items-center gap-1.5 border-[var(--rule)] border-b px-5 py-2 text-xs"
          >
            {suggestions.length === 0 ? (
              <span className="text-[var(--ink-2)]">Space for filters</span>
            ) : null}
            {suggestions.map((entry) => (
              <button
                key={entry.label}
                type="button"
                aria-pressed={entry === offered}
                title={entry.example ?? entry.hint}
                onClick={() => {
                  setQuery(entry.query);
                  setSuggested(0);
                  field.current?.focus();
                }}
                className={`flex items-center gap-1.5 rounded-[7px] px-2 py-0.5 ${
                  entry === offered
                    ? "bg-[var(--ink)] text-[var(--panel)]"
                    : "bg-[var(--muted)] text-[var(--ink)] hover:bg-[var(--rule)]"
                }`}
              >
                <span className="font-mono">{entry.label}</span>
                <span className="opacity-70">{entry.hint}</span>
              </button>
            ))}
            {suggestions.length > 0 ? (
              <span
                className="ml-auto text-[var(--ink-2)]"
                title="Left and right choose; Tab completes"
              >
                ←→ tab
              </span>
            ) : null}
          </div>
          <div className="flex min-h-0 flex-1 lg:grid lg:grid-cols-[minmax(0,1fr)_24rem]">
            <div className="flex min-h-0 w-full flex-col">
              <ul
                ref={list}
                role="listbox"
                className="flex min-h-0 w-full flex-col overflow-y-auto p-2 lg:max-h-[56vh]"
              >
                {shown.length === 0 ? (
                  <li className="px-3 py-6 text-center text-[var(--ink-2)] text-sm">
                    {routes.isError
                      ? "Could not load the route library."
                      : routes.isPending
                        ? "Loading the route library…"
                        : filtersActive
                          ? "Nothing here matches this search."
                          : "Nothing here is called that."}
                  </li>
                ) : (
                  shown.map((entry, index) => {
                    const isActive = index === clampedActive;

                    return (
                      <li
                        key={entry.key}
                        id={`search-option-${entry.key}`}
                        role="option"
                        aria-selected={isActive}
                        data-index={index}
                        onMouseMove={() => setActive(index)}
                        onClick={() => pick(index)}
                        className={`${nearest ? ROW_GRID_NEAREST : ROW_GRID} cursor-pointer items-center rounded-[9px] px-3 py-2 text-[var(--ink-2)] text-xs tabular-nums ${
                          isActive ? "bg-[var(--muted)]" : ""
                        }`}
                      >
                        <span className="flex min-w-0 items-center gap-2 text-[var(--ink)]">
                          <span className="truncate font-medium text-sm">{entry.title}</span>
                          {entry.draftId === null ? null : <Badge variant="secondary">Draft</Badge>}
                        </span>
                        <span className="text-right font-semibold text-[var(--ink)]">
                          {formatDistance(entry.distanceMetres)}
                        </span>
                        <span className="text-right">{formatAscent(entry.ascentMetres)}</span>
                        <span className="text-right">
                          {entry.route ? formatMovingTime(entry.movingSeconds) : "–"}
                        </span>
                        {nearest ? (
                          <span className="text-right" title="Distance to start">
                            {entry.startMetres === undefined ? "–" : formatStart(entry.startMetres)}
                          </span>
                        ) : null}
                      </li>
                    );
                  })
                )}
              </ul>
            </div>
            {wide && basemap && shownPreview ? (
              <div className="m-2 ml-0 hidden min-h-80 overflow-hidden rounded-[11px] lg:block">
                <LibraryMap
                  styleUrl={basemap.styleUrl}
                  darkBasemap={basemap.dark}
                  lines={shownPreview.lines}
                  pickedKey={shownPreview.lines[0]?.key ?? null}
                  bounds={shownPreview.bounds}
                  controls={false}
                />
              </div>
            ) : null}
          </div>
          <div className="flex items-center gap-4 border-[var(--rule)] border-t px-5 py-2.5 text-[var(--ink-2)] text-xs">
            <span>
              {`${shown.filter((entry) => entry.route).length} of ${library.length} routes`}
              {nearest && location === null ? " · waiting for your position" : ""}
            </span>
            <span className="ml-auto flex items-center gap-1">
              <IconArrowUp size={12} />
              <IconArrowDown size={12} /> move
            </span>
            <span className="flex items-center gap-1">
              <IconCornerDownLeft size={12} /> open
            </span>
            <span>esc close</span>
          </div>
        </DialogPrimitive.Popup>
      </DialogPortal>
    </Dialog>
  );
}
