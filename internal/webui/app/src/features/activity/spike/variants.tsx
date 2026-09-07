/**
 * Four ways the ride page could weight and divide what it holds.
 *
 * Today the page is six panels of equal weight stacked in a column, and a
 * forty-nine-row table at the foot. Each variant below takes one position on
 * what a rider opens the page to see, and lets everything else follow from
 * it, so comparing them is a choice between ideas rather than between margins.
 */

import { IconArrowUp, IconChevronDown, IconChevronUp } from "@tabler/icons-react";
import { type ReactNode, useCallback, useMemo, useRef, useState } from "react";
import type { BoundingBox, RideWeatherStep } from "../../../api/types";
import { CartographyProvider } from "../../../components/map/CartographyContext";
import { MapViewport } from "../../../components/map/MapViewport";
import { MapWidget } from "../../../components/map/MapWidget";
import {
  formatAscent,
  formatClock,
  formatDistance,
  formatDuration,
  formatKilometres,
  formatWindSpeed,
} from "../../../lib/format";
import { PADDING, plotAxis } from "../../../lib/plotAxis";
import { buildActivityProfile, gradientBand } from "../../../lib/profile";
import type { AlignedSeries } from "../../../lib/rideSeries";
import { useElementWidth } from "../../../lib/useElementWidth";
import { temperatureColour, weatherIcon } from "../../../lib/weather";
import { flowBearingDegrees } from "../../../lib/windField";
import { ElevationProfile } from "../../routes/ElevationProfile";
import { windWeight } from "../../routes/forecastCells";
import { RouteOverlay } from "../../routes/RouteOverlay";
import { RideSplits } from "../RideSplits";
import {
  ASCENT_METRES,
  AVERAGE_KMH,
  COORDINATES,
  ELAPSED_SECONDS,
  METRICS,
  MOVING_SECONDS,
  SPLITS,
  STEP_METRES,
  TITLE,
  TOTAL_METRES,
  WEATHER,
} from "./data";

/* ------------------------------------------------------------ shared parts */

const STYLES = {
  light: "https://tiles.openfreemap.org/styles/bright",
  dark: "https://tiles.openfreemap.org/styles/dark",
};
const ZONE_NAMES = ["Recovery", "Endurance", "Tempo", "Threshold", "VO₂ max"];
const KM = SPLITS.length;

function useDark(): boolean {
  return document.documentElement.dataset.theme === "dark";
}

function useRide() {
  return useMemo(() => {
    const profile = buildActivityProfile(COORDINATES);
    const longitudes = COORDINATES.map((position) => position[0]);
    const latitudes = COORDINATES.map((position) => position[1]);
    const bounds: BoundingBox = [
      Math.min(...longitudes),
      Math.min(...latitudes),
      Math.max(...longitudes),
      Math.max(...latitudes),
    ];
    const perSample = (read: (km: number) => number | undefined) =>
      profile?.samples.map(
        (sample) => read(Math.min(Math.floor(sample.distanceMetres / 1000), KM - 1)) ?? null,
      ) ?? [];
    const series: AlignedSeries[] = [
      {
        key: "heartRate",
        label: "Heart rate",
        unit: "bpm",
        colour: "var(--series-heart-rate)",
        values: perSample((km) => SPLITS[km]?.heartRateBpm),
      },
      {
        key: "power",
        label: "Power",
        unit: "W",
        colour: "var(--series-power)",
        values: perSample((km) => SPLITS[km]?.powerWatts),
      },
    ];

    return { profile, bounds, series };
  }, []);
}

interface Cursor {
  active: number | null;
  onActive: (metres: number | null) => void;
}

function kmAt(active: number | null): number | null {
  return active === null ? null : Math.min(Math.floor(active / 1000), KM - 1);
}

function TrackMap({ active, onActive, className }: Cursor & { className: string }) {
  const dark = useDark();
  const { profile, bounds } = useRide();

  return (
    <div className={className}>
      <CartographyProvider dark={dark}>
        <MapWidget styleUrl={dark ? STYLES.dark : STYLES.light} ariaLabel="Recorded track">
          <MapViewport bounds={bounds} maxZoom={15} padding={24} />
          <RouteOverlay
            coordinates={COORDINATES}
            profile={profile}
            activeMetres={active}
            onActiveChange={onActive}
          />
        </MapWidget>
      </CartographyProvider>
    </div>
  );
}

function Panel({
  title,
  action,
  className = "",
  children,
}: {
  title?: string;
  action?: ReactNode;
  className?: string;
  children: ReactNode;
}) {
  return (
    <section
      className={`flex flex-col gap-3 rounded-xl bg-[var(--panel)] p-4 ring-1 ring-black/5 ${className}`}
    >
      {title ? (
        <div className="flex items-baseline justify-between gap-3">
          <h2 className="font-medium text-sm">{title}</h2>
          {action}
        </div>
      ) : null}
      {children}
    </section>
  );
}

function Label({ children }: { children: ReactNode }) {
  return (
    <span className="font-semibold text-[10px] text-[var(--ink-2)] uppercase tracking-[0.08em]">
      {children}
    </span>
  );
}

function Stat({
  label,
  value,
  unit,
  note,
  size = "md",
}: {
  label: string;
  value: string;
  unit?: string;
  note?: string;
  size?: "sm" | "md" | "lg";
}) {
  const valueClass = { sm: "text-lg", md: "text-2xl", lg: "text-4xl" }[size];

  return (
    <div className="flex flex-col gap-0.5">
      <Label>{label}</Label>
      <span className={`font-semibold ${valueClass} tabular-nums tracking-tight`}>
        {value}
        {unit ? <span className="ml-1 font-normal text-[var(--ink-2)] text-sm">{unit}</span> : null}
      </span>
      {note ? <span className="text-[var(--ink-2)] text-xs">{note}</span> : null}
    </div>
  );
}

function zoneColour(zone: number): string {
  return `color-mix(in oklab, var(--accent) ${20 + zone * 20}%, var(--panel))`;
}

/** One bar, five segments: the ride's time as a whole rather than five races to the right edge. */
function ZoneStack({ legend = true }: { legend?: boolean }) {
  const seconds = METRICS.zoneSeconds ?? [];
  const total = seconds.reduce((sum, value) => sum + value, 0);

  return (
    <div className="flex flex-col gap-2">
      <div className="flex h-3 overflow-hidden rounded-full bg-black/5" aria-hidden="true">
        {seconds.map((value, zone) => (
          <span
            key={ZONE_NAMES[zone]}
            className="h-full"
            style={{ width: `${(value / total) * 100}%`, backgroundColor: zoneColour(zone) }}
          />
        ))}
      </div>
      {legend ? (
        <ul className="flex flex-wrap gap-x-4 gap-y-1 text-xs">
          {seconds.map((value, zone) => (
            <li key={ZONE_NAMES[zone]} className="flex items-center gap-1.5">
              <span
                aria-hidden="true"
                className="h-2 w-2 rounded-full"
                style={{ backgroundColor: zoneColour(zone) }}
              />
              <span className="text-[var(--ink-2)]">{ZONE_NAMES[zone]}</span>
              <span className="tabular-nums">{formatDuration(value)}</span>
            </li>
          ))}
        </ul>
      ) : null}
    </div>
  );
}

function StepTile({ step, compact = false }: { step: RideWeatherStep; compact?: boolean }) {
  const Glyph = weatherIcon(step.weatherCode);

  return (
    <li className={`flex items-center gap-2 ${compact ? "" : "flex-col gap-1"}`}>
      <span className="text-[var(--ink-2)] text-xs tabular-nums">
        {formatClock(new Date(step.time))}
      </span>
      <Glyph aria-hidden="true" size={18} stroke={1.6} />
      <span
        className="font-medium text-sm tabular-nums"
        style={{ color: temperatureColour(step.temperatureCelsius) }}
      >
        {Math.round(step.temperatureCelsius)}°
      </span>
      <span className="flex items-center gap-0.5 text-[var(--ink-2)] text-xs">
        <IconArrowUp
          aria-hidden="true"
          size={12}
          stroke={1.8}
          style={{ transform: `rotate(${flowBearingDegrees(step.windDirectionDegrees)}deg)` }}
        />
        <span className="tabular-nums">{formatWindSpeed(step.windSpeedKmh)}</span>
      </span>
      {step.precipitationMillimetres > 0 ? (
        <span className="text-[var(--rain-2)] text-xs tabular-nums">
          {step.precipitationMillimetres} mm
        </span>
      ) : null}
    </li>
  );
}

function Steps({ compact = false }: { compact?: boolean }) {
  return (
    <ul className={compact ? "flex flex-col gap-1.5" : "flex justify-between gap-2"}>
      {WEATHER.map((step) => (
        <StepTile key={step.time} step={step} compact={compact} />
      ))}
    </ul>
  );
}

function conditionsSentence(): string {
  const temps = WEATHER.map((step) => step.temperatureCelsius);
  const rain = WEATHER.reduce((sum, step) => sum + step.precipitationMillimetres, 0);

  return `${Math.round(Math.min(...temps))}–${Math.round(Math.max(...temps))}°, wind ${formatWindSpeed(
    WEATHER.reduce((sum, step) => sum + step.windSpeedKmh, 0) / WEATHER.length,
  )}${rain > 0 ? `, ${rain} mm of rain` : ""}`;
}

/**
 * The ride's weather as tiles under the terrain, each as wide as the ground
 * the ride covered in that hour: the route page's forecast strip, read back.
 * Where each hour began is taken from the splits' own clock.
 */
function RideStrip() {
  const { ref, width } = useElementWidth<HTMLDivElement>();
  const { x } = plotAxis(width, 0, TOTAL_METRES);
  const cells = WEATHER.map((step, index) => ({
    step,
    startMetres: STEP_METRES[index] ?? TOTAL_METRES,
    endMetres: STEP_METRES[index + 1] ?? TOTAL_METRES,
  })).filter((cell) => cell.startMetres < TOTAL_METRES);

  return (
    <div ref={ref} style={{ paddingLeft: PADDING.left, paddingRight: PADDING.right }}>
      <div className="relative h-[68px] overflow-hidden rounded-md border border-[var(--rule)]">
        {cells.map(({ step, startMetres, endMetres }) => {
          const Glyph = weatherIcon(step.weatherCode);
          const left = x(startMetres);
          const cellWidth = Math.max(x(endMetres) - left, 0);
          const wet = Math.min(step.precipitationMillimetres / 5, 1) * 0.5;
          // What a tile gives up as it narrows, in the route strip's order.
          const place =
            cellWidth >= 84
              ? `${formatKilometres(startMetres)} · ${formatClock(new Date(step.time))}`
              : cellWidth >= 34
                ? formatClock(new Date(step.time))
                : null;
          const wind = cellWidth >= 34;

          return (
            <div
              key={step.time}
              className="absolute top-0 flex h-full flex-col items-center justify-center gap-0.5 overflow-hidden border-[var(--rule)] not-last:border-r"
              style={{
                left,
                width: cellWidth,
                backgroundColor: `color-mix(in srgb, var(--rain-1) ${wet * 100}%, transparent)`,
              }}
            >
              {place === null ? null : (
                <span className="text-[10px] text-[var(--ink-2)] tabular-nums whitespace-nowrap">
                  {place}
                </span>
              )}
              <span className="flex items-center gap-1.5">
                <Glyph size={15} stroke={1.7} aria-hidden="true" className="text-[var(--ink-2)]" />
                <span
                  className="rounded px-1 font-semibold text-[11px] tabular-nums"
                  style={{
                    backgroundColor: `color-mix(in srgb, ${temperatureColour(step.temperatureCelsius)} 60%, transparent)`,
                  }}
                >
                  {Math.round(step.temperatureCelsius)}°
                </span>
              </span>
              <span
                className="flex items-center gap-0.5 text-[10px] text-[var(--ink-2)] tabular-nums"
                style={{ opacity: 0.4 + windWeight(step.windSpeedKmh) * 0.6 }}
              >
                {wind ? (
                  <IconArrowUp
                    size={12}
                    stroke={2.2}
                    aria-hidden="true"
                    style={{
                      transform: `rotate(${flowBearingDegrees(step.windDirectionDegrees)}deg)`,
                    }}
                  />
                ) : null}
                {wind ? Math.round(step.windSpeedKmh) : null}
                {wind && step.precipitationMillimetres > 0
                  ? ` · ${step.precipitationMillimetres} mm`
                  : ""}
              </span>
            </div>
          );
        })}
      </div>
    </div>
  );
}

/** Pointer position across a plot as metres along the ride. */
function useCursorHandlers(onActive: (metres: number | null) => void) {
  const plot = useRef<HTMLDivElement>(null);
  const onPointerMove = useCallback(
    (event: React.PointerEvent) => {
      const rect = plot.current?.getBoundingClientRect();
      if (!rect || rect.width === 0) {
        return;
      }
      const fraction = Math.min(Math.max((event.clientX - rect.left) / rect.width, 0), 1);
      onActive(fraction * TOTAL_METRES);
    },
    [onActive],
  );
  const onPointerLeave = useCallback(() => onActive(null), [onActive]);

  return { plot, onPointerMove, onPointerLeave };
}

const LANE = { width: 1000, height: 100 } as const;
const BAR = LANE.width / KM;

function speedKmh(km: number): number {
  const split = SPLITS[km];

  return split && split.movingSeconds > 0 ? (split.distanceMetres / split.movingSeconds) * 3.6 : 0;
}

/** Per-kilometre bars, each coloured by how steep that kilometre was. */
function SplitBars({ active, height = 96 }: { active: number | null; height?: number }) {
  const fastest = Math.max(...SPLITS.map((_, km) => speedKmh(km)));
  const activeKm = kmAt(active);

  return (
    <svg
      viewBox={`0 0 ${LANE.width} ${LANE.height}`}
      preserveAspectRatio="none"
      className="block w-full"
      style={{ height }}
      aria-hidden="true"
    >
      {SPLITS.map((split, km) => {
        const bar = (speedKmh(km) / fastest) * LANE.height;

        return (
          <rect
            // The kilometre is the bar's identity.
            // biome-ignore lint/suspicious/noArrayIndexKey: the index is the split
            key={km}
            x={km * BAR + 1}
            y={LANE.height - bar}
            width={BAR - 2}
            height={bar}
            rx={1}
            fill={`var(--grade-${gradientBand(split.ascentMetres / 10)})`}
            opacity={activeKm === null || activeKm === km ? 1 : 0.45}
          />
        );
      })}
    </svg>
  );
}

/** A line over per-kilometre values, drawn in the series' own colour. */
function LineLane({
  read,
  colour,
  active,
  height = 56,
}: {
  read: (km: number) => number | undefined;
  colour: string;
  active: number | null;
  height?: number;
}) {
  const values = SPLITS.map((_, km) => read(km) ?? 0);
  const low = Math.min(...values);
  const high = Math.max(...values);
  const y = (value: number) => LANE.height - ((value - low) / Math.max(high - low, 1)) * 90 - 5;
  const points = values.map((value, km) => `${km * BAR + BAR / 2},${y(value)}`).join(" ");
  const activeKm = kmAt(active);

  return (
    <svg
      viewBox={`0 0 ${LANE.width} ${LANE.height}`}
      preserveAspectRatio="none"
      className="block w-full"
      style={{ height }}
      aria-hidden="true"
    >
      <polyline
        points={points}
        fill="none"
        stroke={colour}
        strokeWidth={1.75}
        vectorEffect="non-scaling-stroke"
        strokeLinejoin="round"
      />
      {activeKm !== null ? (
        <circle
          cx={activeKm * BAR + BAR / 2}
          cy={y(values[activeKm] ?? 0)}
          r={4}
          fill={colour}
          vectorEffect="non-scaling-stroke"
        />
      ) : null}
    </svg>
  );
}

/** The terrain as one area, banded by the steepness of each kilometre. */
function TerrainLane({ height = 110 }: { height?: number }) {
  const { profile } = useRide();
  if (!profile) {
    return null;
  }
  const span = Math.max(profile.maxElevationMetres - profile.minElevationMetres, 10);
  const x = (metres: number) => (metres / TOTAL_METRES) * LANE.width;
  const y = (elevation: number) =>
    LANE.height - ((elevation - profile.minElevationMetres) / span) * 92 - 4;
  const line = profile.samples.map(
    (sample) => `${x(sample.distanceMetres)},${y(sample.elevationMetres)}`,
  );
  const area = `M0,${LANE.height} L${line.join(" L")} L${LANE.width},${LANE.height} Z`;

  return (
    <svg
      viewBox={`0 0 ${LANE.width} ${LANE.height}`}
      preserveAspectRatio="none"
      className="block w-full"
      style={{ height }}
      aria-hidden="true"
    >
      <defs>
        <clipPath id="terrain-clip">
          <path d={area} />
        </clipPath>
      </defs>
      <g clipPath="url(#terrain-clip)">
        {SPLITS.map((split, km) => (
          <rect
            // biome-ignore lint/suspicious/noArrayIndexKey: the index is the split
            key={km}
            x={km * BAR}
            y={0}
            width={BAR + 0.5}
            height={LANE.height}
            fill={`var(--grade-${gradientBand(split.ascentMetres / 10)})`}
            opacity={0.55}
          />
        ))}
      </g>
      <polyline
        points={line.join(" ")}
        fill="none"
        stroke="var(--ink)"
        strokeWidth={1.25}
        vectorEffect="non-scaling-stroke"
        opacity={0.7}
      />
    </svg>
  );
}

function Crumb() {
  return (
    <a className="text-[var(--ink-2)] text-xs underline" href="#activities">
      Activities
    </a>
  );
}

function Page({ children, wide = false }: { children: ReactNode; wide?: boolean }) {
  return (
    <div className={`mx-auto flex w-full flex-col gap-4 ${wide ? "max-w-7xl" : "max-w-5xl"}`}>
      {children}
    </div>
  );
}

/* ------------------------------------------------------------- A: Headline */

/**
 * The bet: four figures decide the ride, and a rider should read them before
 * anything else. They are set large beside the map; the effort and the
 * weather become two quiet panels, and the table folds into one chart.
 */
export function HeadlinePage() {
  const [active, setActive] = useState<number | null>(null);
  const [table, setTable] = useState(false);
  const { profile, series } = useRide();

  return (
    <Page>
      <Crumb />
      <div className="grid gap-6 lg:grid-cols-[minmax(0,2fr)_minmax(0,3fr)]">
        <div className="flex flex-col justify-between gap-6 py-1">
          <div className="flex flex-col gap-1">
            <h1 className="font-semibold text-4xl tracking-tight">{TITLE}</h1>
            <p className="text-[var(--ink-2)] text-sm">{conditionsSentence()}</p>
          </div>
          <dl className="grid grid-cols-2 gap-x-6 gap-y-5">
            <Stat label="Distance" value={formatDistance(TOTAL_METRES)} size="lg" />
            <Stat
              label="Moving"
              value={formatDuration(MOVING_SECONDS)}
              note={`${formatDuration(ELAPSED_SECONDS)} elapsed`}
              size="lg"
            />
            <Stat label="Climbed" value={formatAscent(ASCENT_METRES)} size="lg" />
            <Stat
              label="Training stress"
              value={String(METRICS.powerTss)}
              unit="TSS"
              note={`${METRICS.intensityFactor} of threshold`}
              size="lg"
            />
          </dl>
        </div>
        <TrackMap
          active={active}
          onActive={setActive}
          className="h-80 overflow-hidden rounded-2xl ring-1 ring-black/5"
        />
      </div>
      <Panel>
        <ElevationProfile
          profile={profile}
          title={TITLE}
          series={series}
          activeMetres={active}
          onActiveChange={setActive}
          caption={false}
        />
        <RideStrip />
        <SeriesLegend active={active} />
      </Panel>
      <Panel title="Effort">
        <div className="grid gap-6 md:grid-cols-[minmax(0,1fr)_minmax(0,1fr)]">
          <ZoneStack />
          <div className="grid grid-cols-3 gap-x-4 gap-y-3">
            <Stat label="Avg heart rate" value="143" unit="bpm" size="sm" />
            <Stat label="Max heart rate" value="172" unit="bpm" size="sm" />
            <Stat label="Cadence" value="76" unit="rpm" size="sm" />
            <Stat label="Normalized" value="232" unit="W" size="sm" />
            <Stat label="hrTSS" value="73" size="sm" />
            <Stat label="TRIMP" value="130" size="sm" />
          </div>
        </div>
      </Panel>
      <Panel
        title="By the kilometre"
        action={
          <button
            type="button"
            onClick={() => setTable(!table)}
            className="flex items-center gap-1 text-[var(--ink-2)] text-xs hover:text-[var(--ink)]"
          >
            {table ? "Hide the table" : "Show the table"}
            {table ? <IconChevronUp size={14} /> : <IconChevronDown size={14} />}
          </button>
        }
      >
        <Scrub active={active} onActive={setActive}>
          <SplitBars active={active} />
        </Scrub>
        <SplitReadout active={active} />
      </Panel>
      {table ? <RideSplits splits={SPLITS} /> : null}
    </Page>
  );
}

function Scrub({ children, onActive }: Cursor & { children: ReactNode }) {
  const { plot, onPointerMove, onPointerLeave } = useCursorHandlers(onActive);

  return (
    // The mark on the map and the line on the profile are the same cursor.
    <div ref={plot} onPointerMove={onPointerMove} onPointerLeave={onPointerLeave}>
      {children}
    </div>
  );
}

function SplitReadout({ active }: { active: number | null }) {
  const km = kmAt(active);
  const split = km === null ? null : SPLITS[km];

  return (
    <p className="h-4 text-[var(--ink-2)] text-xs tabular-nums">
      {split && km !== null
        ? `${formatKilometres((km + 1) * 1000)} · ${speedKmh(km).toFixed(1)} km/h · ${formatAscent(
            split.ascentMetres,
          )} · ${Math.round(split.heartRateBpm ?? 0)} bpm · ${Math.round(split.powerWatts ?? 0)} W`
        : "Speed by the kilometre, coloured by how steep it was."}
    </p>
  );
}

function SeriesLegend({ active }: { active: number | null }) {
  const { profile, series } = useRide();
  const index =
    profile && active !== null
      ? Math.round((active / TOTAL_METRES) * (profile.samples.length - 1))
      : null;

  return (
    <div className="flex flex-wrap gap-4 text-xs">
      {series.map((one) => (
        <span key={one.key} className="flex items-center gap-1.5">
          <span
            aria-hidden="true"
            className="h-2 w-2 rounded-full"
            style={{ backgroundColor: one.colour }}
          />
          <span className="text-[var(--ink-2)]">{one.label}</span>
          <span className="tabular-nums">
            {index === null ? "" : `${Math.round(one.values[index] ?? 0)} ${one.unit}`}
          </span>
        </span>
      ))}
    </div>
  );
}

/* ---------------------------------------------------------------- B: Bento */

/**
 * The bet: the page is a dashboard, and a dashboard is tiles. Every figure
 * gets a tile of its own with one label and one number, so nothing has to be
 * read out of a sentence, and the map is simply the largest tile.
 */
function Tile({
  label,
  className = "",
  children,
}: {
  label?: string;
  className?: string;
  children: ReactNode;
}) {
  return (
    <div
      className={`flex flex-col gap-2 rounded-2xl bg-[var(--panel)] p-4 ring-1 ring-black/5 ${className}`}
    >
      {label ? <Label>{label}</Label> : null}
      {children}
    </div>
  );
}

function Big({ value, unit }: { value: string; unit?: string }) {
  return (
    <span className="whitespace-nowrap font-semibold text-2xl tabular-nums tracking-tight">
      {value}
      {unit ? <span className="ml-1 font-normal text-[var(--ink-2)] text-base">{unit}</span> : null}
    </span>
  );
}

export function BentoPage() {
  const [active, setActive] = useState<number | null>(null);
  const { profile, series } = useRide();

  return (
    <Page>
      <div className="flex flex-col gap-1">
        <Crumb />
        <h1 className="font-semibold text-2xl tracking-tight">{TITLE}</h1>
      </div>
      <div className="grid auto-rows-[minmax(7rem,auto)] grid-cols-6 gap-3">
        <TrackMap
          active={active}
          onActive={setActive}
          className="col-span-4 row-span-2 overflow-hidden rounded-2xl ring-1 ring-black/5"
        />
        <Tile label="Distance">
          <Big value={formatDistance(TOTAL_METRES)} />
        </Tile>
        <Tile label="Moving">
          <Big value={formatDuration(MOVING_SECONDS)} />
          <span className="text-[var(--ink-2)] text-xs">
            {formatDuration(ELAPSED_SECONDS)} elapsed
          </span>
        </Tile>
        <Tile label="Climbed">
          <Big value={formatAscent(ASCENT_METRES)} />
        </Tile>
        <Tile label="Speed">
          <Big value={AVERAGE_KMH} unit="km/h" />
        </Tile>
        <Tile label="Heart rate" className="col-span-2">
          <div className="flex items-baseline gap-4">
            <Big value="143" unit="bpm" />
            <span className="text-[var(--ink-2)] text-sm tabular-nums">172 max</span>
          </div>
          <LineLane
            read={(km) => SPLITS[km]?.heartRateBpm}
            colour="var(--series-heart-rate)"
            active={active}
            height={40}
          />
        </Tile>
        <Tile label="Power" className="col-span-2">
          <div className="flex items-baseline gap-4">
            <Big value="232" unit="W" />
            <span className="text-[var(--ink-2)] text-sm tabular-nums">232 NP · 0.88 IF</span>
          </div>
          <LineLane
            read={(km) => SPLITS[km]?.powerWatts}
            colour="var(--series-power)"
            active={active}
            height={40}
          />
        </Tile>
        <Tile label="Training load" className="col-span-2">
          <div className="flex items-baseline gap-4">
            <Big value="118" unit="TSS" />
            <span className="text-[var(--ink-2)] text-sm tabular-nums">73 hrTSS · 130 TRIMP</span>
          </div>
          <ZoneStack legend={false} />
        </Tile>
        <Tile label="Time in zone" className="col-span-3">
          <ul className="grid grid-cols-5 gap-2">
            {(METRICS.zoneSeconds ?? []).map((seconds, zone) => (
              <li key={ZONE_NAMES[zone]} className="flex flex-col gap-1">
                <span
                  className="h-1.5 rounded-full"
                  style={{ backgroundColor: zoneColour(zone) }}
                />
                <span className="text-[var(--ink-2)] text-xs">{ZONE_NAMES[zone]}</span>
                <span className="text-sm tabular-nums">{formatDuration(seconds)}</span>
              </li>
            ))}
          </ul>
        </Tile>
        <Tile label="Conditions" className="col-span-3">
          <Steps />
        </Tile>
        <Tile className="col-span-6">
          <ElevationProfile
            profile={profile}
            title={TITLE}
            series={series}
            activeMetres={active}
            onActiveChange={setActive}
            caption={false}
          />
          <SeriesLegend active={active} />
        </Tile>
        <Tile label="Speed by the kilometre" className="col-span-6">
          <Scrub active={active} onActive={setActive}>
            <SplitBars active={active} height={72} />
          </Scrub>
          <SplitReadout active={active} />
        </Tile>
      </div>
    </Page>
  );
}

/* ---------------------------------------------------------------- C: Lanes */

/**
 * The bet: a ride is one axis, and everything the page knows is a function
 * of where along it you are. Terrain, speed, heart rate, power and weather are
 * lanes on the same distance scale, under one cursor, with a readout beside
 * each. The table goes: the lanes are the splits.
 */
function Lane({
  label,
  readout,
  children,
  plotRef,
}: {
  label: string;
  readout: string;
  children: ReactNode;
  plotRef?: React.Ref<HTMLDivElement>;
}) {
  return (
    <>
      <div className="flex items-center">
        <Label>{label}</Label>
      </div>
      <div ref={plotRef} className="border-[var(--rule)] border-b">
        {children}
      </div>
      <div className="flex items-center pl-3 text-sm tabular-nums">{readout}</div>
    </>
  );
}

export function LanesPage() {
  const [active, setActive] = useState<number | null>(null);
  const { plot, onPointerMove, onPointerLeave } = useCursorHandlers(setActive);
  const km = kmAt(active);
  const split = km === null ? null : SPLITS[km];
  const step =
    active === null ? null : WEATHER[STEP_METRES.filter((at) => at <= active).length - 1];
  const at = (value: string) => (split ? value : "");

  return (
    <Page>
      <div className="flex flex-wrap items-end justify-between gap-4">
        <div className="flex flex-col gap-1">
          <Crumb />
          <h1 className="font-semibold text-2xl tracking-tight">{TITLE}</h1>
        </div>
        <dl className="flex gap-6">
          <Stat label="Distance" value={formatDistance(TOTAL_METRES)} size="sm" />
          <Stat label="Moving" value={formatDuration(MOVING_SECONDS)} size="sm" />
          <Stat label="Climbed" value={formatAscent(ASCENT_METRES)} size="sm" />
          <Stat label="TSS" value="118" size="sm" />
          <Stat label="Avg HR" value="143" unit="bpm" size="sm" />
          <Stat label="Avg power" value="232" unit="W" size="sm" />
        </dl>
      </div>
      <TrackMap
        active={active}
        onActive={setActive}
        className="h-64 overflow-hidden rounded-xl ring-1 ring-black/5"
      />
      <div
        className="grid grid-cols-[6rem_minmax(0,1fr)_7rem] gap-x-2 rounded-xl bg-[var(--panel)] p-4 ring-1 ring-black/5"
        onPointerMove={onPointerMove}
        onPointerLeave={onPointerLeave}
      >
        <div />
        <div className="relative h-8">
          {WEATHER.map((one, index) => {
            const Glyph = weatherIcon(one.weatherCode);
            const at = (STEP_METRES[index] ?? 0) / TOTAL_METRES;
            if (at >= 1) {
              return null;
            }
            return (
              <span
                key={one.time}
                className="absolute top-0 flex items-center gap-1 text-xs"
                style={{
                  left: `${at * 100}%`,
                  transform: at > 0.8 ? "translateX(-100%)" : undefined,
                }}
              >
                <Glyph size={16} stroke={1.6} aria-hidden="true" />
                <span style={{ color: temperatureColour(one.temperatureCelsius) }}>
                  {Math.round(one.temperatureCelsius)}°
                </span>
                <IconArrowUp
                  size={11}
                  stroke={1.8}
                  aria-hidden="true"
                  className="text-[var(--ink-2)]"
                  style={{
                    transform: `rotate(${flowBearingDegrees(one.windDirectionDegrees)}deg)`,
                  }}
                />
              </span>
            );
          })}
        </div>
        <div className="pl-3 text-[var(--ink-2)] text-xs">
          {step ? formatClock(new Date(step.time)) : ""}
        </div>
        <Lane
          label="Terrain"
          readout={at(`${formatAscent(split?.ascentMetres ?? 0)} up`)}
          plotRef={plot}
        >
          <div className="relative">
            <TerrainLane />
            {active !== null ? (
              <span
                aria-hidden="true"
                className="absolute inset-y-0 w-px bg-[var(--ink)]"
                style={{ left: `${(active / TOTAL_METRES) * 100}%` }}
              />
            ) : null}
          </div>
        </Lane>
        <Lane label="Speed" readout={at(`${km === null ? "" : speedKmh(km).toFixed(1)} km/h`)}>
          <SplitBars active={active} height={64} />
        </Lane>
        <Lane label="Heart rate" readout={at(`${Math.round(split?.heartRateBpm ?? 0)} bpm`)}>
          <LineLane
            read={(one) => SPLITS[one]?.heartRateBpm}
            colour="var(--series-heart-rate)"
            active={active}
          />
        </Lane>
        <Lane label="Power" readout={at(`${Math.round(split?.powerWatts ?? 0)} W`)}>
          <LineLane
            read={(one) => SPLITS[one]?.powerWatts}
            colour="var(--series-power)"
            active={active}
          />
        </Lane>
        <div />
        <div className="flex justify-between pt-1 text-[10px] text-[var(--ink-2)] tabular-nums">
          {[0, 10, 20, 30, 40, 49].map((tick) => (
            <span key={tick}>{tick} km</span>
          ))}
        </div>
        <div className="pl-3 text-[var(--ink-2)] text-xs tabular-nums">
          {km === null ? "" : formatKilometres((km + 1) * 1000)}
        </div>
      </div>
      <div className="grid gap-4 md:grid-cols-[minmax(0,1fr)_minmax(0,2fr)]">
        <Panel title="Time in zone">
          <ZoneStack />
        </Panel>
        <Panel title="Load">
          <div className="grid grid-cols-4 gap-x-4 gap-y-3">
            <Stat label="TSS" value="118" note="power" size="sm" />
            <Stat label="hrTSS" value="73" note="heart rate" size="sm" />
            <Stat label="TRIMP" value="130" note="Banister" size="sm" />
            <Stat label="Intensity" value="0.88" note="of threshold" size="sm" />
            <Stat label="Normalized" value="232" unit="W" size="sm" />
            <Stat label="Max HR" value="172" unit="bpm" size="sm" />
            <Stat label="Cadence" value="76" unit="rpm" size="sm" />
            <Stat label="Elapsed" value={formatDuration(ELAPSED_SECONDS)} size="sm" />
          </div>
        </Panel>
      </div>
    </Page>
  );
}

/* ---------------------------------------------------------------- D: Atlas */

/**
 * The bet: a ride is a place first, and the page should look like the atlas
 * the rest of the application already is. The map is the stage; a rail card
 * floats over it with the figures, the profile docks along its foot, and only
 * the weather and the table sit below.
 */
export function AtlasPage() {
  const [active, setActive] = useState<number | null>(null);
  const [open, setOpen] = useState(true);
  const { profile, series } = useRide();

  return (
    <Page wide>
      <div className="relative h-[38rem] overflow-hidden rounded-2xl ring-1 ring-black/5">
        <TrackMap active={active} onActive={setActive} className="absolute inset-0" />
        <aside className="absolute top-3 left-3 flex w-[21rem] max-h-[calc(100%-1.5rem)] flex-col gap-4 overflow-y-auto rounded-xl bg-[var(--panel)] p-4 shadow-[var(--shadow)] ring-1 ring-black/5">
          <div className="flex flex-col gap-0.5">
            <Crumb />
            <h1 className="font-semibold text-xl tracking-tight">{TITLE}</h1>
            <p className="text-[var(--ink-2)] text-xs">{conditionsSentence()}</p>
          </div>
          <dl className="grid grid-cols-2 gap-x-4 gap-y-3">
            <Stat label="Distance" value={formatDistance(TOTAL_METRES)} />
            <Stat label="Climbed" value={formatAscent(ASCENT_METRES)} />
            <Stat
              label="Moving"
              value={formatDuration(MOVING_SECONDS)}
              note={`${formatDuration(ELAPSED_SECONDS)} elapsed`}
            />
            <Stat label="Speed" value={AVERAGE_KMH} unit="km/h" />
          </dl>
          <button
            type="button"
            onClick={() => setOpen(!open)}
            className="flex items-center justify-between text-left"
          >
            <Label>Effort</Label>
            {open ? (
              <IconChevronUp size={14} className="text-[var(--ink-2)]" />
            ) : (
              <IconChevronDown size={14} className="text-[var(--ink-2)]" />
            )}
          </button>
          {open ? (
            <>
              <ZoneStack />
              <dl className="grid grid-cols-3 gap-x-3 gap-y-3">
                <Stat label="TSS" value="118" size="sm" />
                <Stat label="hrTSS" value="73" size="sm" />
                <Stat label="TRIMP" value="130" size="sm" />
                <Stat label="Avg HR" value="143" size="sm" />
                <Stat label="Max HR" value="172" size="sm" />
                <Stat label="Cadence" value="76" size="sm" />
                <Stat label="Power" value="232" size="sm" />
                <Stat label="Normalized" value="232" size="sm" />
                <Stat label="Intensity" value="0.88" size="sm" />
              </dl>
            </>
          ) : null}
        </aside>
        <div className="absolute right-3 bottom-3 left-[23rem] rounded-xl bg-[var(--panel)] p-3 shadow-[var(--shadow)] ring-1 ring-black/5">
          <ElevationProfile
            profile={profile}
            title={TITLE}
            series={series}
            activeMetres={active}
            onActiveChange={setActive}
            caption={false}
          />
          <SeriesLegend active={active} />
        </div>
      </div>
      <div className="grid gap-4 md:grid-cols-[minmax(0,1fr)_minmax(0,2fr)]">
        <Panel title="Conditions">
          <Steps compact />
        </Panel>
        <RideSplits splits={SPLITS} />
      </div>
    </Page>
  );
}
