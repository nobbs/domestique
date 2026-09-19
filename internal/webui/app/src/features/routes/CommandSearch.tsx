/**
 * The entry page's one way into the library: a quiet trigger, or ⌘K from
 * anywhere on the page, opens a command panel over the map — search, the
 * measure filters, and a keyboard-driven list of what they leave. Enter or a
 * click opens the route at once; there is no intermediate card.
 *
 * State lives on `AtlasPage` — this only reads it and reports keystrokes back,
 * the same split `SearchPanel` kept before it.
 */

import { Dialog as DialogPrimitive } from "@base-ui/react/dialog";
import {
  IconAdjustmentsHorizontal,
  IconArrowDown,
  IconArrowUp,
  IconCornerDownLeft,
  IconSearch,
} from "@tabler/icons-react";
import { useEffect, useMemo, useRef, useState } from "react";
import type { Position, Route, RouteSurface } from "../../api/types";
import { routeKey } from "../../api/types";
import { RangeSlider } from "../../components/RangeSlider";
import { RouteGlyph } from "../../components/RouteGlyph";
import { librarySources, offersSourceChoice, SourceChips } from "../../components/SourceChips";
import { Dialog, DialogOverlay, DialogPortal } from "../../components/ui/dialog";
import { domainOf } from "../../lib/domain";
import { EMPTY_FILTERS, hasActiveFilters, type LibraryFilters } from "../../lib/filters";
import { formatAscent, formatDistance, formatGradient, formatMovingTime } from "../../lib/format";
import { bandLabel, bandVariable, surfaceLabel, surfaceVariable } from "../../lib/mix";
import { gradientBand, gradientShares } from "../../lib/profile";
import type { RouteChange } from "../../lib/seenRoutes";
import { summariseSurface } from "../../lib/surface";
import { ThinBar } from "../catalogue/ThinBar";
import { RouteChangeBadge } from "./RouteChangeBadge";

/** The geometry a row needs, when it has arrived. Rows render without it. */
export interface RouteShape {
  coordinates: Position[];
  surface?: RouteSurface;
}

const MEASURES = [
  {
    key: "distanceMetres",
    label: "Distance",
    steps: [1_000, 2_000, 5_000, 10_000],
    format: (metres: number) => `${metres / 1000} km`,
  },
  {
    key: "ascentMetres",
    label: "Ascent",
    steps: [10, 20, 50, 100, 200],
    format: (metres: number) => `${metres} m`,
  },
  {
    key: "movingSeconds",
    label: "Duration",
    steps: [300, 600, 900, 1_800],
    format: (seconds: number) => (seconds === 0 ? "0 min" : formatMovingTime(seconds)),
  },
] as const;

/** The two bars a row is drawn with, from the same geometry the map already fetched. */
function mixOf(shape: RouteShape | undefined) {
  const coordinates = shape?.coordinates ?? [];
  const summary = shape?.surface ? summariseSurface(coordinates, shape.surface.ranges) : null;

  return {
    coordinates,
    surface:
      summary?.shares.map((entry) => ({
        key: entry.kind,
        label: surfaceLabel(entry.kind),
        colour: surfaceVariable(entry.kind),
        share: entry.share,
      })) ?? [],
    gradient: gradientShares(coordinates).map((entry) => ({
      key: `${entry.band}`,
      label: bandLabel(entry.band),
      colour: bandVariable(entry.band),
      share: entry.share,
    })),
  };
}

function resultsText(shown: Route[], library: Route[]): string {
  return shown.length === library.length
    ? `${library.length} routes`
    : `${shown.length} of ${library.length}`;
}

export interface CommandSearchProps {
  open: boolean;
  onOpenChange: (open: boolean) => void;
  /** Hides the trigger button; the panel itself still answers ⌘K. */
  routeOpen: boolean;
  library: Route[];
  shown: Route[];
  query: string;
  onQueryChange: (query: string) => void;
  filters: LibraryFilters;
  onFiltersChange: (filters: LibraryFilters) => void;
  /** The route the list opens on. Only read on the open transition. */
  activeKey: string | null;
  /** Opens a route and closes the panel. */
  onOpen: (key: string) => void;
  shapes: Map<string, RouteShape>;
  changeOf: (route: Route) => RouteChange;
}

export function CommandSearch({
  open,
  onOpenChange,
  routeOpen,
  library,
  shown,
  query,
  onQueryChange,
  filters,
  onFiltersChange,
  activeKey,
  onOpen,
  shapes,
  changeOf,
}: CommandSearchProps) {
  const [active, setActive] = useState(0);
  const [filtersOpen, setFiltersOpen] = useState(false);
  const field = useRef<HTMLInputElement>(null);
  const list = useRef<HTMLUListElement>(null);
  const measures = useMemo(
    () =>
      MEASURES.map((measure) => ({
        ...measure,
        values: library.map((route) => route[measure.key] ?? 0),
      })),
    [library],
  );

  // ⌘K / Ctrl+K toggles the panel from anywhere on the page, route open or not.
  useEffect(() => {
    const onKey = (event: KeyboardEvent) => {
      if ((event.metaKey || event.ctrlKey) && event.key.toLowerCase() === "k") {
        event.preventDefault();
        onOpenChange(!open);
      }
    };
    window.addEventListener("keydown", onKey);

    return () => window.removeEventListener("keydown", onKey);
  }, [open, onOpenChange]);

  // Reopening lands the active row on the route the panel was pointed at and
  // refocuses the field. Only on the open transition: `shown` and `activeKey`
  // move as the reader types and as the map is pointed at, and re-running this
  // on every one of those would fight the arrow keys and the reader's own scroll.
  // biome-ignore lint/correctness/useExhaustiveDependencies: intentional, see above
  useEffect(() => {
    if (!open) {
      return;
    }
    const at = activeKey ? shown.findIndex((route) => routeKey(route) === activeKey) : -1;
    setActive(at >= 0 ? at : 0);
    requestAnimationFrame(() => field.current?.focus());
  }, [open]);

  // Typing starts the list over at its top result.
  // biome-ignore lint/correctness/useExhaustiveDependencies: query itself is unused, only its changing matters
  useEffect(() => {
    setActive(0);
  }, [query]);

  const clampedActive = shown.length === 0 ? 0 : Math.min(active, shown.length - 1);
  useEffect(() => {
    list.current
      ?.querySelector(`[data-index="${clampedActive}"]`)
      ?.scrollIntoView({ block: "nearest" });
  }, [clampedActive]);

  const pick = (target: Route | undefined) => {
    if (!target) {
      return;
    }
    onOpen(routeKey(target));
    onOpenChange(false);
  };
  const onKeyDown = (event: React.KeyboardEvent) => {
    // The list answers keys typed into the query; the panel's buttons and
    // sliders keep their own.
    if (event.target !== field.current) {
      return;
    }
    if (event.key === "ArrowDown") {
      event.preventDefault();
      setActive((index) => Math.min(index + 1, shown.length - 1));
    } else if (event.key === "ArrowUp") {
      event.preventDefault();
      setActive((index) => Math.max(index - 1, 0));
    } else if (event.key === "Enter") {
      event.preventDefault();
      pick(shown[clampedActive]);
    }
  };

  const filtersActive = hasActiveFilters(filters);
  const sources = librarySources(library);
  const hasQuery = query.trim() !== "";

  return (
    <>
      {routeOpen ? null : (
        // The same pill mechanism `RoutePanel` uses: `data-compact-workspace`
        // strips the workspace rail's own chrome, so this button is the pill.
        <div data-compact-workspace="" className="w-fit max-w-full">
          <button
            type="button"
            onClick={() => onOpenChange(true)}
            aria-label="Search the route library"
            className="flex h-10 w-64 items-center gap-2 rounded-[11px] bg-[var(--panel)] px-3 text-[var(--ink-2)] text-sm shadow-[var(--shadow)] hover:text-[var(--ink)]"
          >
            <IconSearch size={16} stroke={1.8} aria-hidden="true" />
            <span className="flex-1 truncate text-left">
              {query || `Search ${library.length} routes`}
            </span>
            {filtersActive ? (
              <span
                data-testid="filters-active-dot"
                className="size-2 rounded-full bg-[var(--accent)]"
              />
            ) : null}
            <kbd className="rounded-[7px] bg-[var(--muted)] px-2 py-0.5 font-sans text-xs">⌘K</kbd>
          </button>
        </div>
      )}
      <Dialog open={open} onOpenChange={onOpenChange}>
        <DialogPortal>
          <DialogOverlay />
          <DialogPrimitive.Popup
            aria-label="Search the route library"
            className="fixed top-[10vh] left-1/2 z-50 flex h-fit max-h-[70vh] w-[40rem] max-w-[calc(100vw-2rem)] -translate-x-1/2 flex-col overflow-hidden rounded-xl bg-[var(--panel)] shadow-[var(--shadow)] outline-none"
            onKeyDown={onKeyDown}
          >
            <label className="flex items-center gap-3 border-[var(--rule)] border-b px-5 py-4">
              <IconSearch
                size={20}
                stroke={1.8}
                className="text-[var(--ink-2)]"
                aria-hidden="true"
              />
              <input
                ref={field}
                type="search"
                value={query}
                onChange={(event) => onQueryChange(event.target.value)}
                placeholder="Route name or place"
                aria-label="Search the route library"
                aria-activedescendant={
                  shown[clampedActive]
                    ? `search-option-${routeKey(shown[clampedActive])}`
                    : undefined
                }
                className="min-w-0 flex-1 bg-transparent text-lg outline-none placeholder:text-[var(--ink-2)] [&::-webkit-search-cancel-button]:appearance-none"
              />
              <button
                type="button"
                aria-expanded={filtersOpen}
                onClick={() => setFiltersOpen((current) => !current)}
                className={`flex h-8 items-center gap-1.5 rounded-[9px] px-2.5 text-xs ${
                  filtersOpen
                    ? "bg-[var(--muted)] text-[var(--ink)]"
                    : filtersActive
                      ? "bg-[var(--ink)] text-[var(--panel)]"
                      : "text-[var(--ink-2)] hover:bg-[var(--muted)]"
                }`}
              >
                <IconAdjustmentsHorizontal size={14} stroke={1.8} aria-hidden="true" />
                Filters
              </button>
            </label>
            {filtersOpen ? (
              <div className="flex flex-col gap-3 border-[var(--rule)] border-b px-5 py-3">
                {offersSourceChoice(sources, filters.providers) ? (
                  <div className="flex items-center gap-4">
                    <span className="shrink-0 font-semibold text-sm">Source</span>
                    <SourceChips
                      sources={sources}
                      chosen={filters.providers}
                      onChange={(providers) => onFiltersChange({ ...filters, providers })}
                    />
                  </div>
                ) : null}
                <div className="grid grid-cols-3 gap-4">
                  {measures.map((measure) => {
                    const domain = domainOf(measure.values, [...measure.steps]);

                    return (
                      <RangeSlider
                        key={measure.key}
                        legend={measure.label}
                        min={0}
                        max={domain.max}
                        step={domain.step}
                        range={filters[measure.key]}
                        onChange={(next) => onFiltersChange({ ...filters, [measure.key]: next })}
                        format={measure.format}
                        values={measure.values}
                      />
                    );
                  })}
                </div>
                {filtersActive ? (
                  <button
                    type="button"
                    className="self-end text-[var(--ink-2)] text-xs hover:text-[var(--ink)]"
                    onClick={() => onFiltersChange(EMPTY_FILTERS)}
                  >
                    Clear filters
                  </button>
                ) : null}
              </div>
            ) : null}
            <ul ref={list} role="listbox" className="flex min-h-0 flex-col overflow-y-auto p-2">
              {shown.length === 0 ? (
                <li className="px-3 py-6 text-center text-[var(--ink-2)] text-sm">
                  {hasQuery && filtersActive
                    ? "Nothing here matches this search and these filters."
                    : filtersActive
                      ? "Nothing here matches these filters."
                      : "Nothing here is called that."}
                </li>
              ) : (
                shown.map((route, index) => {
                  const key = routeKey(route);
                  const mix = mixOf(shapes.get(key));
                  const isActive = index === clampedActive;

                  return (
                    <li
                      key={key}
                      id={`search-option-${key}`}
                      role="option"
                      aria-selected={isActive}
                      data-index={index}
                      onMouseMove={() => setActive(index)}
                      onClick={() => pick(route)}
                      className={`flex cursor-pointer items-center gap-3 rounded-[9px] px-3 py-2 ${
                        isActive ? "bg-[var(--muted)]" : ""
                      }`}
                    >
                      <span className="block size-9 shrink-0">
                        <RouteGlyph
                          coordinates={mix.coordinates}
                          title={route.title}
                          band={gradientBand(route.maxGradientPercent)}
                        />
                      </span>
                      <span className="flex min-w-0 flex-1 flex-col gap-1">
                        <span className="flex items-center gap-1.5">
                          <span className="truncate font-medium text-sm">{route.title}</span>
                          <RouteChangeBadge change={changeOf(route)} />
                        </span>
                        <span className="flex max-w-60 flex-col gap-0.5">
                          <ThinBar segments={mix.surface} label="Surface" />
                          <ThinBar segments={mix.gradient} label="Gradient" />
                        </span>
                      </span>
                      <span className="grid grid-cols-[auto_auto] gap-x-4 text-right text-xs tabular-nums">
                        <span className="font-semibold text-sm">
                          {formatDistance(route.distanceMetres)}
                        </span>
                        <span className="text-sm">{formatAscent(route.ascentMetres)}</span>
                        <span className="text-[var(--ink-2)]">
                          {formatMovingTime(route.movingSeconds)}
                        </span>
                        <span className="text-[var(--ink-2)]">
                          max {formatGradient(route.maxGradientPercent)}
                        </span>
                      </span>
                      {isActive ? (
                        <IconCornerDownLeft
                          size={14}
                          className="text-[var(--ink-2)]"
                          aria-hidden="true"
                        />
                      ) : (
                        <span className="w-3.5" />
                      )}
                    </li>
                  );
                })
              )}
            </ul>
            <div className="flex items-center gap-4 border-[var(--rule)] border-t px-5 py-2.5 text-[var(--ink-2)] text-xs">
              <span>{resultsText(shown, library)}</span>
              <span className="ml-auto flex items-center gap-1">
                <IconArrowUp size={12} />
                <IconArrowDown size={12} /> move
              </span>
              <span className="flex items-center gap-1">
                <IconCornerDownLeft size={12} /> open route
              </span>
              <span>esc close</span>
            </div>
          </DialogPrimitive.Popup>
        </DialogPortal>
      </Dialog>
    </>
  );
}
