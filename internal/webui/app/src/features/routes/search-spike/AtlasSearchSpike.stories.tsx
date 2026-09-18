/**
 * Four ways to search and filter the atlas, over a stand-in map drawn from the
 * synthetic library's own shapes. Storybook only.
 */

import type { Meta, StoryObj } from "@storybook/react-vite";
import {
  IconAdjustmentsHorizontal,
  IconArrowDown,
  IconArrowRight,
  IconArrowUp,
  IconChevronDown,
  IconCornerDownLeft,
  IconMapPin,
  IconSearch,
  IconX,
} from "@tabler/icons-react";
import { type ReactNode, useEffect, useMemo, useRef, useState } from "react";
import { MemoryRouter } from "react-router";
import { type Route, routeKey } from "../../../api/types";
import { RangeSlider } from "../../../components/RangeSlider";
import { RouteGlyph } from "../../../components/RouteGlyph";
import { Segmented } from "../../../components/Segmented";
import { Popover, PopoverContent, PopoverTrigger } from "../../../components/ui/popover";
import { domainOf } from "../../../lib/domain";
import {
  EMPTY_FILTERS,
  hasActiveFilters,
  type LibraryFilters,
  matchesFilters,
} from "../../../lib/filters";
import {
  formatAscent,
  formatDistance,
  formatGradient,
  formatMovingTime,
} from "../../../lib/format";
import { matchingRoutes } from "../../../lib/library";
import { bandLabel, bandVariable, surfaceLabel, surfaceVariable } from "../../../lib/mix";
import { gradientBand, gradientShares } from "../../../lib/profile";
import { summariseSurface } from "../../../lib/surface";
import { ThinBar } from "../../catalogue/ThinBar";
import { LIBRARY } from "./data";

const ROUTES = LIBRARY.map((entry) => entry.route);
const GEOMETRY = new Map(LIBRARY.map((entry) => [routeKey(entry.route), entry.geometry]));
const WASH = "bg-[color-mix(in_oklab,var(--ink-2)_7%,transparent)]";
const CARD = "rounded-2xl bg-[var(--panel)] shadow-[var(--shadow)]";

function mixes(route: Route) {
  const geometry = GEOMETRY.get(routeKey(route));
  const coordinates = geometry?.coordinates ?? [];
  const summary = geometry?.surface ? summariseSurface(coordinates, geometry.surface.ranges) : null;
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

function useSearch() {
  const [query, setQuery] = useState("");
  const [filters, setFilters] = useState<LibraryFilters>(EMPTY_FILTERS);
  const [picked, setPicked] = useState<string | null>(null);
  const shown = useMemo(
    () => matchingRoutes(ROUTES, query).filter((route) => matchesFilters(route, filters)),
    [query, filters],
  );
  return { query, setQuery, filters, setFilters, picked, setPicked, shown };
}

/** A stand-in map: every route's line on a paper ground, the picked one drawn over the rest. */
function FakeMap({ picked, shown }: { picked: string | null; shown: Route[] }) {
  const all = LIBRARY.flatMap((entry) => entry.geometry.coordinates);
  const lons = all.map((c) => c[0]);
  const lats = all.map((c) => c[1]);
  const [x0, x1, y0, y1] = [
    Math.min(...lons),
    Math.max(...lons),
    Math.min(...lats),
    Math.max(...lats),
  ];
  const visible = new Set(shown.map(routeKey));
  const path = (key: string) =>
    (GEOMETRY.get(key)?.coordinates ?? [])
      .map(
        (c, i) =>
          `${i ? "L" : "M"}${(((c[0] - x0) / (x1 - x0)) * 1000).toFixed(1)},${((1 - (c[1] - y0) / (y1 - y0)) * 700).toFixed(1)}`,
      )
      .join("");
  return (
    <svg
      viewBox="-150 -80 1300 860"
      preserveAspectRatio="xMidYMid slice"
      className="absolute inset-0 size-full bg-[#ebe7dc]"
      aria-hidden="true"
    >
      {LIBRARY.map(({ route }) => {
        const key = routeKey(route);
        return (
          <path
            key={key}
            d={path(key)}
            fill="none"
            stroke={key === picked ? "var(--accent)" : "#1a1a1a"}
            strokeWidth={key === picked ? 4 : 1.4}
            opacity={key === picked ? 1 : visible.has(key) ? 0.7 : 0.12}
          />
        );
      })}
    </svg>
  );
}

function Stage({
  children,
  picked,
  shown,
}: {
  children: ReactNode;
  picked: string | null;
  shown: Route[];
}) {
  return (
    <div className="relative h-[900px] overflow-hidden">
      <FakeMap picked={picked} shown={shown} />
      {children}
    </div>
  );
}

function SoftSearch({
  query,
  onQuery,
  className = "w-full",
}: {
  query: string;
  onQuery: (q: string) => void;
  className?: string;
}) {
  const lit = query !== "";
  return (
    <label
      data-lit={lit || undefined}
      className={`flex h-10 items-center gap-2 rounded-[11px] bg-[var(--muted)] px-3 focus-within:bg-[var(--panel)] focus-within:shadow-[0_0_0_1px_var(--rule),var(--shadow)] data-lit:bg-[var(--panel)] data-lit:shadow-[0_0_0_1px_var(--rule),var(--shadow)] ${className}`}
    >
      <IconSearch size={16} stroke={1.8} className="text-[var(--ink-2)]" aria-hidden="true" />
      <input
        type="search"
        value={query}
        onChange={(event) => onQuery(event.target.value)}
        placeholder={`Search ${ROUTES.length} routes`}
        aria-label="Search the route library"
        className="min-w-0 flex-1 bg-transparent text-sm outline-none placeholder:text-[var(--ink-2)] [&::-webkit-search-cancel-button]:appearance-none"
      />
      {lit ? (
        <button
          type="button"
          aria-label="Clear search"
          onClick={() => onQuery("")}
          className="text-[var(--ink-2)] hover:text-[var(--ink)]"
        >
          <IconX size={14} />
        </button>
      ) : null}
    </label>
  );
}

function FilterToggle({
  open,
  onOpen,
  filters,
}: {
  open: boolean;
  onOpen: (open: boolean) => void;
  filters: LibraryFilters;
}) {
  const active = hasActiveFilters(filters);
  return (
    <button
      type="button"
      aria-expanded={open}
      aria-label="Filters"
      onClick={() => onOpen(!open)}
      className={`grid size-10 shrink-0 place-items-center rounded-[11px] ${open ? "bg-[var(--panel)] shadow-[0_0_0_1px_var(--rule),var(--shadow)]" : active ? "bg-[var(--ink)] text-[var(--panel)]" : "bg-[var(--muted)] text-[var(--ink-2)] hover:text-[var(--ink)]"}`}
    >
      <IconAdjustmentsHorizontal size={16} stroke={1.8} />
    </button>
  );
}

const MEASURES = [
  {
    key: "distanceMetres",
    label: "Distance",
    values: ROUTES.map((r) => r.distanceMetres),
    steps: [1_000, 2_000, 5_000, 10_000],
    format: (m: number) => `${m / 1000} km`,
  },
  {
    key: "ascentMetres",
    label: "Ascent",
    values: ROUTES.map((r) => r.ascentMetres),
    steps: [10, 20, 50, 100, 200],
    format: (m: number) => `${m} m`,
  },
  {
    key: "movingSeconds",
    label: "Duration",
    values: ROUTES.map((r) => r.movingSeconds ?? 0),
    steps: [300, 600, 900, 1_800],
    format: (s: number) => (s === 0 ? "0 min" : formatMovingTime(s)),
  },
] as const;

function Sliders({
  filters,
  onFilters,
}: {
  filters: LibraryFilters;
  onFilters: (next: LibraryFilters) => void;
}) {
  return (
    <div className={`flex flex-col overflow-hidden rounded-[11px] ${WASH}`}>
      {MEASURES.map((measure) => {
        const domain = domainOf([...measure.values], [...measure.steps]);
        return (
          <div
            key={measure.key}
            className="border-[var(--panel)] border-b-2 px-3.5 py-3 last:border-b-0"
          >
            <RangeSlider
              legend={measure.label}
              min={0}
              max={domain.max}
              step={domain.step}
              range={filters[measure.key]}
              onChange={(next) => onFilters({ ...filters, [measure.key]: next })}
              format={measure.format}
              values={[...measure.values]}
            />
          </div>
        );
      })}
    </div>
  );
}

function Row({ route, picked, onPick }: { route: Route; picked: boolean; onPick: () => void }) {
  const mix = mixes(route);
  return (
    <li className="border-[var(--panel)] border-b-2 last:border-b-0">
      <button
        type="button"
        onClick={onPick}
        aria-pressed={picked}
        className={`relative flex w-full items-center gap-3 px-3 py-2 text-left before:absolute before:inset-1 before:rounded-[7px] hover:before:bg-[color-mix(in_oklab,var(--ink-2)_8%,transparent)] ${picked ? "before:bg-[color-mix(in_oklab,var(--accent)_14%,transparent)]" : ""}`}
      >
        <span className="relative block size-9 shrink-0">
          <RouteGlyph
            coordinates={mix.coordinates}
            title={route.title}
            band={gradientBand(route.maxGradientPercent)}
          />
        </span>
        <span className="relative flex min-w-0 flex-1 flex-col gap-1">
          <span className="truncate font-semibold text-sm">{route.title}</span>
          <span className="flex flex-col gap-0.5">
            <ThinBar segments={mix.surface} label="Surface" />
            <ThinBar segments={mix.gradient} label="Gradient" />
          </span>
        </span>
        <span className="relative flex flex-col items-end text-xs tabular-nums">
          <span className="font-semibold text-sm">{formatDistance(route.distanceMetres)}</span>
          <span className="text-[var(--ink-2)]">{formatAscent(route.ascentMetres)}</span>
        </span>
      </button>
    </li>
  );
}

function PickedCard({ route, onClose }: { route: Route; onClose: () => void }) {
  const mix = mixes(route);
  return (
    <section className={`flex flex-col gap-3 p-4 ${CARD}`}>
      <div className="flex items-start gap-3">
        <span className="block size-12 shrink-0">
          <RouteGlyph
            coordinates={mix.coordinates}
            title={route.title}
            band={gradientBand(route.maxGradientPercent)}
          />
        </span>
        <span className="min-w-0 flex-1 font-semibold">{route.title}</span>
        <button
          type="button"
          aria-label="Close"
          onClick={onClose}
          className="text-[var(--ink-2)] hover:text-[var(--ink)]"
        >
          <IconX size={16} />
        </button>
      </div>
      <div className="grid grid-cols-4 gap-2 text-sm tabular-nums">
        {[
          ["Distance", formatDistance(route.distanceMetres)],
          ["Ascent", formatAscent(route.ascentMetres)],
          ["Time", formatMovingTime(route.movingSeconds)],
          ["Steepest", formatGradient(route.maxGradientPercent)],
        ].map(([label, value]) => (
          <span key={label} className="flex flex-col">
            <span className="font-semibold">{value}</span>
            <span className="text-[var(--ink-2)] text-xs">{label}</span>
          </span>
        ))}
      </div>
      <button
        type="button"
        className="flex h-9 items-center justify-center gap-2 rounded-[9px] bg-[var(--primary)] font-semibold text-[var(--primary-foreground)] text-sm"
      >
        Open route <IconArrowRight size={16} />
      </button>
    </section>
  );
}

const count = (shown: Route[]) =>
  shown.length === ROUTES.length
    ? `${ROUTES.length} routes`
    : `${shown.length} of ${ROUTES.length}`;

/** V1 · Floating card: one card top left holding search, the filter toggle, the sliders when open, and a scrolling list that never outgrows the map. */
function V1() {
  const s = useSearch();
  const [open, setOpen] = useState(false);
  const route = ROUTES.find((r) => routeKey(r) === s.picked);
  return (
    <Stage picked={s.picked} shown={s.shown}>
      <div className="absolute top-4 bottom-4 left-4 flex w-[26rem] flex-col gap-3">
        <section className={`flex max-h-full min-h-0 flex-col gap-3 p-3 ${CARD}`}>
          <div className="flex items-center gap-2">
            <SoftSearch query={s.query} onQuery={s.setQuery} />
            <FilterToggle open={open} onOpen={setOpen} filters={s.filters} />
          </div>
          {open ? <Sliders filters={s.filters} onFilters={s.setFilters} /> : null}
          <div className="flex items-baseline justify-between px-1 text-xs">
            <span className="text-[var(--ink-2)]">{count(s.shown)}</span>
            {hasActiveFilters(s.filters) ? (
              <button
                type="button"
                className="text-[var(--ink-2)] hover:text-[var(--ink)]"
                onClick={() => s.setFilters(EMPTY_FILTERS)}
              >
                Clear filters
              </button>
            ) : null}
          </div>
          <ul className={`flex min-h-0 flex-col overflow-y-auto rounded-[11px] ${WASH}`}>
            {s.shown.map((r) => (
              <Row
                key={routeKey(r)}
                route={r}
                picked={routeKey(r) === s.picked}
                onPick={() => s.setPicked(routeKey(r))}
              />
            ))}
          </ul>
        </section>
      </div>
      {route ? (
        <div className="absolute right-20 bottom-6 w-[26rem]">
          <PickedCard route={route} onClose={() => s.setPicked(null)} />
        </div>
      ) : null}
    </Stage>
  );
}

/** V2 · Docked sidebar: a full-height column on the left owns search, filters and the list; the map fills the rest. */
function V2() {
  const s = useSearch();
  const [open, setOpen] = useState(true);
  const route = ROUTES.find((r) => routeKey(r) === s.picked);
  return (
    <div className="flex h-[900px]">
      <aside className="flex w-[24rem] shrink-0 flex-col gap-3 border-[var(--rule)] border-r bg-[var(--panel)] p-4">
        <div className="flex items-center gap-2">
          <SoftSearch query={s.query} onQuery={s.setQuery} />
          <FilterToggle open={open} onOpen={setOpen} filters={s.filters} />
        </div>
        {open ? <Sliders filters={s.filters} onFilters={s.setFilters} /> : null}
        <span className="px-1 text-[var(--ink-2)] text-xs">{count(s.shown)}</span>
        <ul className={`flex min-h-0 flex-1 flex-col overflow-y-auto rounded-[11px] ${WASH}`}>
          {s.shown.map((r) => (
            <Row
              key={routeKey(r)}
              route={r}
              picked={routeKey(r) === s.picked}
              onPick={() => s.setPicked(routeKey(r))}
            />
          ))}
        </ul>
      </aside>
      <div className="relative flex-1 overflow-hidden">
        <FakeMap picked={s.picked} shown={s.shown} />
        {route ? (
          <div className="absolute top-4 left-4 w-[24rem]">
            <PickedCard route={route} onClose={() => s.setPicked(null)} />
          </div>
        ) : null}
      </div>
    </div>
  );
}

/** V3 · Search bar across the top with filter chips under it; results drop down as a sheet only while searching or filtering. */
function V3() {
  const s = useSearch();
  const [focused, setFocused] = useState(false);
  const route = ROUTES.find((r) => routeKey(r) === s.picked);
  const listing = focused || s.query !== "" || hasActiveFilters(s.filters);
  return (
    <Stage picked={s.picked} shown={s.shown}>
      <div className="absolute top-4 left-1/2 flex w-[36rem] -translate-x-1/2 flex-col gap-2">
        <div className={`flex flex-col gap-2 p-2 ${CARD}`}>
          <div onFocus={() => setFocused(true)}>
            <SoftSearch query={s.query} onQuery={s.setQuery} />
          </div>
          <div className="flex flex-wrap items-center gap-1.5 px-1">
            {MEASURES.map((measure) => {
              const range = s.filters[measure.key];
              const set = range.min !== null || range.max !== null;
              const domain = domainOf([...measure.values], [...measure.steps]);
              return (
                <Popover key={measure.key}>
                  <PopoverTrigger
                    className={`inline-flex h-8 items-center gap-1.5 rounded-[9px] px-3 text-xs ${set ? "bg-[var(--ink)] text-[var(--panel)]" : "bg-[var(--muted)] text-[var(--ink-2)] hover:text-[var(--ink)]"}`}
                  >
                    {measure.label}
                    <IconChevronDown size={12} aria-hidden="true" />
                  </PopoverTrigger>
                  <PopoverContent className="w-80">
                    <RangeSlider
                      legend={measure.label}
                      min={0}
                      max={domain.max}
                      step={domain.step}
                      range={range}
                      onChange={(next) => s.setFilters({ ...s.filters, [measure.key]: next })}
                      format={measure.format}
                      values={[...measure.values]}
                    />
                  </PopoverContent>
                </Popover>
              );
            })}
            <span className="ml-auto text-[var(--ink-2)] text-xs">{count(s.shown)}</span>
            {listing ? (
              <button
                type="button"
                onClick={() => {
                  setFocused(false);
                  s.setQuery("");
                  s.setFilters(EMPTY_FILTERS);
                }}
                className="text-[var(--ink-2)] text-xs hover:text-[var(--ink)]"
              >
                Done
              </button>
            ) : null}
          </div>
        </div>
        {listing ? (
          <ul className={`flex max-h-[32rem] flex-col overflow-y-auto p-1.5 ${CARD}`}>
            {s.shown.map((r) => (
              <Row
                key={routeKey(r)}
                route={r}
                picked={routeKey(r) === s.picked}
                onPick={() => {
                  s.setPicked(routeKey(r));
                  setFocused(false);
                }}
              />
            ))}
          </ul>
        ) : null}
      </div>
      {route && !listing ? (
        <div className="absolute bottom-6 left-1/2 w-[30rem] -translate-x-1/2">
          <PickedCard route={route} onClose={() => s.setPicked(null)} />
        </div>
      ) : null}
    </Stage>
  );
}

/** V4 · Search top left, results as a card strip along the bottom of the map: scroll sideways, pick one to raise it on the map. */
function V4() {
  const s = useSearch();
  const [open, setOpen] = useState(false);
  const [sort, setSort] = useState<"distance" | "ascent" | "name">("distance");
  const sorted = [...s.shown].sort((a, b) =>
    sort === "name"
      ? a.title.localeCompare(b.title)
      : sort === "distance"
        ? a.distanceMetres - b.distanceMetres
        : a.ascentMetres - b.ascentMetres,
  );
  return (
    <Stage picked={s.picked} shown={s.shown}>
      <div className="absolute top-4 left-4 flex w-[24rem] flex-col gap-2">
        <div className={`flex flex-col gap-2 p-2 ${CARD}`}>
          <div className="flex items-center gap-2">
            <SoftSearch query={s.query} onQuery={s.setQuery} />
            <FilterToggle open={open} onOpen={setOpen} filters={s.filters} />
          </div>
          {open ? <Sliders filters={s.filters} onFilters={s.setFilters} /> : null}
        </div>
      </div>
      <div className="absolute right-4 bottom-4 left-4 flex flex-col gap-2">
        <div className="flex items-center gap-2">
          <span className={`rounded-[9px] px-2.5 py-1 text-xs ${CARD}`}>{count(s.shown)}</span>
          <Segmented
            label="Sort"
            size="sm"
            items={[
              { key: "distance", label: "Distance" },
              { key: "ascent", label: "Ascent" },
              { key: "name", label: "Name" },
            ]}
            value={sort}
            onChange={setSort}
          />
        </div>
        <ul className="flex snap-x gap-3 overflow-x-auto pb-1">
          {sorted.map((route) => {
            const mix = mixes(route);
            const key = routeKey(route);
            return (
              <li key={key} className="snap-start">
                <button
                  type="button"
                  onClick={() => s.setPicked(key)}
                  aria-pressed={key === s.picked}
                  className={`flex w-60 flex-col gap-2 p-3 text-left ${CARD} ${key === s.picked ? "ring-2 ring-[var(--accent)]" : ""}`}
                >
                  <span className="flex items-center gap-2">
                    <span className="block size-10 shrink-0">
                      <RouteGlyph
                        coordinates={mix.coordinates}
                        title={route.title}
                        band={gradientBand(route.maxGradientPercent)}
                      />
                    </span>
                    <span className="line-clamp-2 font-semibold text-sm">{route.title}</span>
                  </span>
                  <span className="flex justify-between text-xs tabular-nums">
                    <span className="font-semibold">{formatDistance(route.distanceMetres)}</span>
                    <span>{formatAscent(route.ascentMetres)}</span>
                    <span className="text-[var(--ink-2)]">
                      {formatMovingTime(route.movingSeconds)}
                    </span>
                  </span>
                  <span className="flex flex-col gap-0.5">
                    <ThinBar segments={mix.surface} label="Surface" />
                    <ThinBar segments={mix.gradient} label="Gradient" />
                  </span>
                  {key === s.picked ? (
                    <span className="inline-flex items-center gap-1 font-semibold text-[var(--accent)] text-xs">
                      <IconMapPin size={12} /> Open route
                    </span>
                  ) : null}
                </button>
              </li>
            );
          })}
        </ul>
      </div>
    </Stage>
  );
}

/** A stand-in for the route's own panel, which opening a route swaps in on the atlas. */
function OpenedRoute({ route, onClose }: { route: Route; onClose: () => void }) {
  const mix = mixes(route);
  return (
    <section className={`flex h-full flex-col gap-4 p-5 ${CARD}`}>
      <div className="flex items-start gap-3">
        <span className="block size-12 shrink-0">
          <RouteGlyph
            coordinates={mix.coordinates}
            title={route.title}
            band={gradientBand(route.maxGradientPercent)}
          />
        </span>
        <span className="flex min-w-0 flex-1 flex-col">
          <span className="font-semibold text-lg leading-tight">{route.title}</span>
          <span className="text-[var(--ink-2)] text-xs">Route open · ⌘K to search again</span>
        </span>
        <button
          type="button"
          aria-label="Close route"
          onClick={onClose}
          className="text-[var(--ink-2)] hover:text-[var(--ink)]"
        >
          <IconX size={18} />
        </button>
      </div>
      <div className="grid grid-cols-4 gap-2 text-sm tabular-nums">
        {[
          ["Distance", formatDistance(route.distanceMetres)],
          ["Ascent", formatAscent(route.ascentMetres)],
          ["Time", formatMovingTime(route.movingSeconds)],
          ["Steepest", formatGradient(route.maxGradientPercent)],
        ].map(([label, value]) => (
          <span key={label} className="flex flex-col">
            <span className="font-semibold">{value}</span>
            <span className="text-[var(--ink-2)] text-xs">{label}</span>
          </span>
        ))}
      </div>
      <div className="flex flex-col gap-1">
        <ThinBar segments={mix.surface} label="Surface" />
        <ThinBar segments={mix.gradient} label="Gradient" />
      </div>
      <div
        className={`grid flex-1 place-items-center rounded-xl text-[var(--ink-2)] text-xs ${WASH}`}
      >
        elevation, weather and the rest of the route panel
      </div>
    </section>
  );
}

/** V5 · Command panel: a quiet button, or ⌘K anywhere, opens a panel over the map; Enter opens the route at once, and ⌘K comes back to search with it still open behind. */
function V5() {
  const s = useSearch();
  const [open, setOpen] = useState(false);
  const [active, setActive] = useState(0);
  const [filtersOpen, setFiltersOpen] = useState(false);
  const field = useRef<HTMLInputElement>(null);
  const list = useRef<HTMLUListElement>(null);
  const route = ROUTES.find((r) => routeKey(r) === s.picked);

  useEffect(() => {
    const onKey = (event: KeyboardEvent) => {
      if ((event.metaKey || event.ctrlKey) && event.key.toLowerCase() === "k") {
        event.preventDefault();
        setOpen((current) => !current);
      }
    };
    window.addEventListener("keydown", onKey);
    return () => window.removeEventListener("keydown", onKey);
  }, []);
  useEffect(() => {
    if (open) requestAnimationFrame(() => field.current?.focus());
  }, [open]);
  useEffect(() => setActive(0), []);
  useEffect(() => {
    list.current?.querySelector(`[data-index="${active}"]`)?.scrollIntoView({ block: "nearest" });
  }, [active]);

  const pick = (target: Route | undefined) => {
    if (!target) return;
    s.setPicked(routeKey(target));
    setOpen(false);
  };
  const onKeyDown = (event: React.KeyboardEvent) => {
    if (event.key === "ArrowDown") {
      event.preventDefault();
      setActive((index) => Math.min(index + 1, s.shown.length - 1));
    } else if (event.key === "ArrowUp") {
      event.preventDefault();
      setActive((index) => Math.max(index - 1, 0));
    } else if (event.key === "Enter") {
      event.preventDefault();
      pick(s.shown[active]);
    } else if (event.key === "Escape") {
      setOpen(false);
    }
  };

  return (
    <Stage picked={s.picked} shown={s.shown}>
      {route ? null : (
        <button
          type="button"
          onClick={() => setOpen(true)}
          className={`absolute top-4 left-4 flex h-10 w-64 items-center gap-2 px-3 text-[var(--ink-2)] text-sm hover:text-[var(--ink)] ${CARD} rounded-[11px]`}
        >
          <IconSearch size={16} stroke={1.8} aria-hidden="true" />
          <span className="flex-1 text-left">
            {s.query ? s.query : `Search ${ROUTES.length} routes`}
          </span>
          {hasActiveFilters(s.filters) ? (
            <span className="size-2 rounded-full bg-[var(--accent)]" />
          ) : null}
          <kbd className="rounded-[7px] bg-[var(--muted)] px-2 py-0.5 font-sans text-xs">⌘K</kbd>
        </button>
      )}
      {route ? (
        <div className="absolute top-4 bottom-4 left-4 w-[26rem]">
          <OpenedRoute route={route} onClose={() => s.setPicked(null)} />
        </div>
      ) : null}
      {open ? (
        <div
          role="presentation"
          className="absolute inset-0 z-10 flex justify-center bg-[color-mix(in_oklab,var(--ink)_22%,transparent)] pt-[10vh]"
          onMouseDown={() => setOpen(false)}
        >
          <div
            role="dialog"
            aria-label="Search the route library"
            className={`flex h-fit max-h-[70vh] w-[40rem] flex-col overflow-hidden ${CARD}`}
            onMouseDown={(event) => event.stopPropagation()}
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
                value={s.query}
                onChange={(event) => s.setQuery(event.target.value)}
                placeholder="Route name or place"
                aria-label="Search the route library"
                aria-activedescendant={
                  s.shown[active] ? `palette-${routeKey(s.shown[active])}` : undefined
                }
                className="min-w-0 flex-1 bg-transparent text-lg outline-none placeholder:text-[var(--ink-2)] [&::-webkit-search-cancel-button]:appearance-none"
              />
              <button
                type="button"
                aria-expanded={filtersOpen}
                onClick={() => setFiltersOpen((current) => !current)}
                className={`flex h-8 items-center gap-1.5 rounded-[9px] px-2.5 text-xs ${filtersOpen ? "bg-[var(--muted)] text-[var(--ink)]" : hasActiveFilters(s.filters) ? "bg-[var(--ink)] text-[var(--panel)]" : "text-[var(--ink-2)] hover:bg-[var(--muted)]"}`}
              >
                <IconAdjustmentsHorizontal size={14} stroke={1.8} aria-hidden="true" />
                Filters
              </button>
            </label>
            {filtersOpen ? (
              <div className="grid grid-cols-3 gap-4 border-[var(--rule)] border-b px-5 py-3">
                {MEASURES.map((measure) => {
                  const domain = domainOf([...measure.values], [...measure.steps]);
                  return (
                    <RangeSlider
                      key={measure.key}
                      legend={measure.label}
                      min={0}
                      max={domain.max}
                      step={domain.step}
                      range={s.filters[measure.key]}
                      onChange={(next) => s.setFilters({ ...s.filters, [measure.key]: next })}
                      format={measure.format}
                      values={[...measure.values]}
                    />
                  );
                })}
              </div>
            ) : null}
            <ul ref={list} role="listbox" className="flex min-h-0 flex-col overflow-y-auto p-2">
              {s.shown.length === 0 ? (
                <li className="px-3 py-6 text-center text-[var(--ink-2)] text-sm">
                  Nothing here is called that.
                </li>
              ) : (
                s.shown.map((r, index) => {
                  const mix = mixes(r);
                  return (
                    <li
                      key={routeKey(r)}
                      id={`palette-${routeKey(r)}`}
                      role="option"
                      aria-selected={index === active}
                      data-index={index}
                      onMouseMove={() => setActive(index)}
                      onClick={() => pick(r)}
                      className={`flex cursor-pointer items-center gap-3 rounded-[9px] px-3 py-2 ${index === active ? "bg-[var(--muted)]" : ""}`}
                    >
                      <span className="block size-9 shrink-0">
                        <RouteGlyph
                          coordinates={mix.coordinates}
                          title={r.title}
                          band={gradientBand(r.maxGradientPercent)}
                        />
                      </span>
                      <span className="flex min-w-0 flex-1 flex-col gap-1">
                        <span className="truncate font-medium text-sm">{r.title}</span>
                        <span className="flex max-w-60 flex-col gap-0.5">
                          <ThinBar segments={mix.surface} label="Surface" />
                          <ThinBar segments={mix.gradient} label="Gradient" />
                        </span>
                      </span>
                      <span className="grid grid-cols-[auto_auto] gap-x-4 text-right text-xs tabular-nums">
                        <span className="font-semibold text-sm">
                          {formatDistance(r.distanceMetres)}
                        </span>
                        <span className="text-sm">{formatAscent(r.ascentMetres)}</span>
                        <span className="text-[var(--ink-2)]">
                          {formatMovingTime(r.movingSeconds)}
                        </span>
                        <span className="text-[var(--ink-2)]">
                          max {formatGradient(r.maxGradientPercent)}
                        </span>
                      </span>
                      {index === active ? (
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
              <span>{count(s.shown)}</span>
              <span className="ml-auto flex items-center gap-1">
                <IconArrowUp size={12} />
                <IconArrowDown size={12} /> move
              </span>
              <span className="flex items-center gap-1">
                <IconCornerDownLeft size={12} /> open route
              </span>
              <span>esc close</span>
            </div>
          </div>
        </div>
      ) : null}
    </Stage>
  );
}

const meta = {
  title: "Spikes/Atlas Search",
  parameters: { layout: "fullscreen" },
  decorators: [
    (Story) => (
      <MemoryRouter>
        <Story />
      </MemoryRouter>
    ),
  ],
} satisfies Meta;

export default meta;
type Story = StoryObj<typeof meta>;

export const V1_FloatingCard: Story = { render: () => <V1 /> };
export const V2_DockedSidebar: Story = { render: () => <V2 /> };
export const V3_TopBarAndSheet: Story = { render: () => <V3 /> };
export const V4_BottomCardStrip: Story = { render: () => <V4 /> };
export const V5_CommandPanel: Story = { render: () => <V5 /> };
