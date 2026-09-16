/**
 * Four takes on the ride page's hero after the sample's billing dashboard:
 * a KPI strip with delta chips, a row of tinted figure pills, outlined tiles
 * with a mark in the corner, and one hero figure over a share bar and dot rows.
 *
 * Storybook only. The map is a stand-in box: what is being compared is how the
 * five figures are weighted and where the map sits against them.
 */

import {
  IconArrowBarUp,
  IconBike,
  IconBolt,
  IconChevronLeft,
  IconFlame,
  IconRuler2,
  IconStopwatch,
} from "@tabler/icons-react";
import type { ReactNode } from "react";
import { formatAscent, formatDistance, formatDuration } from "../../../lib/format";
import {
  ASCENT_METRES,
  ELAPSED_SECONDS,
  METRICS,
  MOVING_SECONDS,
  TITLE,
  TOTAL_METRES,
} from "./data";

const WEATHER_LINE = "11–22°, wind 17 km/h, 8.5 mm of rain";
const ROUTE = "Synthetic Kaiserstuhl Loop";
const CALORIES = "4959";

type Tone = "good" | "info" | "hold" | "alert" | "quiet";

function tint(tone: Tone, share = 12): string {
  const colour =
    tone === "quiet" ? "var(--ink-2)" : tone === "info" ? "var(--accent)" : `var(--${tone})`;
  return `color-mix(in oklab, ${colour} ${share}%, transparent)`;
}

function ink(tone: Tone): string {
  return tone === "quiet" ? "var(--ink)" : tone === "info" ? "var(--accent)" : `var(--${tone})`;
}

function Chip({ tone, children }: { tone: Tone; children: ReactNode }) {
  return (
    <span
      className="inline-flex items-center whitespace-nowrap rounded-full px-1.5 py-0.5 font-medium text-[11px] tabular-nums"
      style={{ color: ink(tone), background: tint(tone) }}
    >
      {children}
    </span>
  );
}

function Mark({ children, size = 8 }: { children: ReactNode; size?: 7 | 8 | 9 }) {
  return (
    <span
      className={`grid shrink-0 place-items-center rounded-md bg-[var(--ink)] text-[var(--panel)] ${size === 7 ? "size-7" : size === 9 ? "size-9" : "size-8"}`}
    >
      {children}
    </span>
  );
}

function Title() {
  return (
    <div className="flex flex-col gap-1.5">
      <span className="inline-flex w-fit items-center gap-0.5 text-[var(--ink-2)] text-xs hover:text-[var(--ink)]">
        <IconChevronLeft size={14} stroke={2} aria-hidden="true" />
        Activities
      </span>
      <h1 className="font-semibold text-2xl leading-tight tracking-tight">{ROUTE}</h1>
      <p className="text-[var(--ink-2)] text-sm">
        {TITLE} · {WEATHER_LINE}
      </p>
    </div>
  );
}

function MapBox({ className = "" }: { className?: string }) {
  return (
    <div
      className={`grid place-items-center rounded-2xl bg-[var(--ground)] text-[var(--ink-2)] text-xs shadow-[var(--shadow)] ${className}`}
    >
      map
    </div>
  );
}

const FIGURES = {
  distance: formatDistance(TOTAL_METRES),
  moving: formatDuration(MOVING_SECONDS),
  elapsed: formatDuration(ELAPSED_SECONDS),
  climbed: formatAscent(ASCENT_METRES),
  tss: String(METRICS.powerTss),
  intensity: METRICS.intensityFactor?.toFixed(2) ?? "",
};

/* ------------------------------------------------------------------------ */

function StripCell({
  icon,
  label,
  value,
  unit,
  chip,
}: {
  icon: ReactNode;
  label: string;
  value: string;
  unit?: string;
  chip?: ReactNode;
}) {
  return (
    <div className="relative flex min-w-0 flex-1 items-center gap-3 px-4 py-3 not-first:before:absolute not-first:before:top-1/2 not-first:before:left-0 not-first:before:h-8 not-first:before:w-px not-first:before:-translate-y-1/2 not-first:before:bg-[var(--rule)] not-first:before:content-['']">
      <Mark size={9}>{icon}</Mark>
      <div className="min-w-0 flex-1">
        <div className="flex items-center gap-2 text-[var(--ink-2)] text-xs">
          {label}
          {chip}
        </div>
        <div className="whitespace-nowrap font-semibold text-xl leading-tight tabular-nums">
          {value}
          {unit ? (
            <span className="ml-1 font-normal text-[var(--ink-2)] text-sm">{unit}</span>
          ) : null}
        </div>
      </div>
    </div>
  );
}

/** A · Strip: the dashboard's top row, full width over the map, a delta chip on every cell. */
export function StripHero() {
  return (
    <div className="grid gap-4">
      <div className="overflow-hidden rounded-2xl bg-[var(--panel)] shadow-[var(--shadow)]">
        {/* The ride itself as the strip's header: what was ridden, when, in what. */}
        <div className="flex items-center gap-3 px-4 pt-4 pb-1">
          <Mark size={9}>
            <IconBike size={18} stroke={1.8} aria-hidden="true" />
          </Mark>
          <div className="min-w-0">
            <div className="truncate font-semibold text-base leading-tight">{ROUTE}</div>
            <div className="truncate text-[var(--ink-2)] text-xs">
              {TITLE} · {WEATHER_LINE}
            </div>
          </div>
        </div>
        <div className="flex">
          <StripCell
            icon={<IconRuler2 size={18} stroke={1.8} aria-hidden="true" />}
            label="Distance"
            value={FIGURES.distance}
            chip={<Chip tone="good">+4.2 km</Chip>}
          />
          <StripCell
            icon={<IconStopwatch size={18} stroke={1.8} aria-hidden="true" />}
            label="Moving"
            value={FIGURES.moving}
            chip={<Chip tone="quiet">+17 min stopped</Chip>}
          />
          <StripCell
            icon={<IconArrowBarUp size={18} stroke={1.8} aria-hidden="true" />}
            label="Climbed"
            value={FIGURES.climbed}
            chip={<Chip tone="hold">55 m/km</Chip>}
          />
          <StripCell
            icon={<IconBolt size={18} stroke={1.8} aria-hidden="true" />}
            label="Training stress"
            value={FIGURES.tss}
            unit="TSS"
            chip={<Chip tone="alert">{FIGURES.intensity} IF</Chip>}
          />
        </div>
      </div>
      <MapBox className="h-72" />
    </div>
  );
}

/* ------------------------------------------------------------------------ */

function Pill({
  tone,
  value,
  label,
  note,
  verdict,
}: {
  tone: Tone;
  value: string;
  label: string;
  note: string;
  verdict?: { tone: Tone; text: string };
}) {
  return (
    <div className="grid min-w-0 flex-1 gap-1.5">
      <div
        className="rounded-full px-4 py-2 font-semibold text-lg tabular-nums"
        style={{ color: ink(tone), background: tint(tone, 14) }}
      >
        {value}
      </div>
      <div className="flex items-center gap-1.5 text-[var(--ink-2)] text-xs">
        <span className="size-1.5 rounded-full" style={{ background: ink(tone) }} />
        {label}
      </div>
      {verdict ? (
        <div>
          <Chip tone={verdict.tone}>{verdict.text}</Chip>
        </div>
      ) : null}
      <div className="text-[var(--ink-2)] text-xs">{note}</div>
    </div>
  );
}

/** B · Pills: the aging card — each figure a tinted pill, its label and reading beneath. */
export function PillsHero() {
  return (
    <div className="grid gap-4 lg:grid-cols-[minmax(0,3fr)_minmax(0,2fr)]">
      <div className="grid content-start gap-4 rounded-2xl bg-[var(--panel)] p-5 shadow-[var(--shadow)]">
        <div className="flex items-center gap-3">
          <Mark size={9}>
            <IconBike size={18} stroke={1.8} aria-hidden="true" />
          </Mark>
          <div className="min-w-0">
            <h1 className="font-semibold text-base leading-tight">{TITLE}</h1>
            <p className="text-[var(--ink-2)] text-xs">
              {ROUTE} · {WEATHER_LINE}
            </p>
          </div>
        </div>
        <div className="flex gap-3">
          <Pill
            tone="info"
            value={FIGURES.distance}
            label="Distance"
            note={`${FIGURES.moving} moving`}
            verdict={{ tone: "info", text: "Whole route" }}
          />
          <Pill
            tone="quiet"
            value={FIGURES.moving}
            label="Moving"
            note={`${FIGURES.elapsed} elapsed`}
            verdict={{ tone: "quiet", text: "17 min stopped" }}
          />
          <Pill
            tone="hold"
            value={FIGURES.climbed}
            label="Climbed"
            note="55 m per km"
            verdict={{ tone: "hold", text: "Mountainous" }}
          />
          <Pill
            tone="alert"
            value={`${FIGURES.tss} TSS`}
            label="Training stress"
            note={`${FIGURES.intensity} of threshold`}
            verdict={{ tone: "alert", text: "Very hard" }}
          />
        </div>
      </div>
      <MapBox className="min-h-56" />
    </div>
  );
}

/* ------------------------------------------------------------------------ */

function Tile({
  icon,
  label,
  value,
  unit,
  note,
}: {
  icon: ReactNode;
  label: string;
  value: string;
  unit?: string;
  note?: string;
}) {
  return (
    <div className="grid gap-1 rounded-xl border border-[var(--rule)] p-3">
      <span className="text-[var(--ink-2)]">{icon}</span>
      <div className="text-[var(--ink-2)] text-xs">{label}</div>
      <div className="font-semibold text-xl leading-tight tabular-nums">
        {value}
        {unit ? <span className="ml-1 font-normal text-[var(--ink-2)] text-sm">{unit}</span> : null}
      </div>
      {note ? <div className="text-[var(--ink-2)] text-xs">{note}</div> : null}
    </div>
  );
}

/** C · Tiles: the "who owes it" card — outlined tiles with a quiet mark in the corner, no fill. */
export function TilesHero() {
  return (
    <div className="grid gap-4 lg:grid-cols-[minmax(0,2fr)_minmax(0,3fr)]">
      <div className="grid content-start gap-4">
        <Title />
        <div className="grid gap-3 rounded-2xl bg-[var(--panel)] p-4 shadow-[var(--shadow)]">
          <h2 className="font-semibold text-base">Ride</h2>
          <div className="grid grid-cols-2 gap-2">
            <Tile
              icon={<IconRuler2 size={18} stroke={1.6} aria-hidden="true" />}
              label="Distance"
              value={FIGURES.distance}
            />
            <Tile
              icon={<IconStopwatch size={18} stroke={1.6} aria-hidden="true" />}
              label="Moving"
              value={FIGURES.moving}
              note={`${FIGURES.elapsed} elapsed`}
            />
            <Tile
              icon={<IconArrowBarUp size={18} stroke={1.6} aria-hidden="true" />}
              label="Climbed"
              value={FIGURES.climbed}
            />
            <Tile
              icon={<IconFlame size={18} stroke={1.6} aria-hidden="true" />}
              label="Calories (est.)"
              value={CALORIES}
              unit="kcal"
            />
            <Tile
              icon={<IconBolt size={18} stroke={1.6} aria-hidden="true" />}
              label="Training stress"
              value={FIGURES.tss}
              unit="TSS"
              note={`${FIGURES.intensity} of threshold · 99% sensor coverage`}
            />
          </div>
        </div>
      </div>
      <MapBox className="min-h-80" />
    </div>
  );
}

/* ------------------------------------------------------------------------ */

function DotRow({
  tone,
  label,
  value,
  share,
}: {
  tone: Tone;
  label: string;
  value: string;
  share?: string;
}) {
  return (
    <div className="flex items-center gap-2 py-1 text-sm">
      <span className="size-1.5 shrink-0 rounded-full" style={{ background: ink(tone) }} />
      <span className="flex-1">{label}</span>
      <span className="font-medium tabular-nums">{value}</span>
      {share ? (
        <span className="w-14 whitespace-nowrap text-right text-[var(--ink-2)] text-xs tabular-nums">
          {share}
        </span>
      ) : null}
    </div>
  );
}

/** D · Hero: the "collected this month" card — one big figure, a share bar, dot rows for the rest. */
export function HeroFigure() {
  const moving = MOVING_SECONDS / ELAPSED_SECONDS;
  return (
    <div className="grid gap-4 lg:grid-cols-[minmax(0,2fr)_minmax(0,3fr)]">
      <div className="grid content-start gap-4">
        <Title />
        <div className="grid gap-3 rounded-2xl bg-[var(--panel)] p-5 shadow-[var(--shadow)]">
          <div className="flex items-baseline gap-2">
            <span className="font-semibold text-4xl leading-none tabular-nums tracking-tight">
              {FIGURES.distance}
            </span>
            <Chip tone="alert">{FIGURES.tss} TSS</Chip>
          </div>
          <div className="h-2 overflow-hidden rounded-full bg-[var(--muted)]">
            <div
              className="h-full rounded-full bg-[var(--good)]"
              style={{ width: `${moving * 100}%` }}
            />
          </div>
          <p className="text-[var(--ink-2)] text-xs">
            {FIGURES.moving} moving of {FIGURES.elapsed} · {Math.round(moving * 100)}%
          </p>
          <div>
            <DotRow tone="good" label="Climbed" value={FIGURES.climbed} share="55 m/km" />
            <DotRow tone="hold" label="Calories (est.)" value={`${CALORIES} kcal`} />
            <DotRow
              tone="alert"
              label="Intensity"
              value={`${FIGURES.intensity} of threshold`}
              share="99%"
            />
          </div>
          <p className="border-[var(--rule)] border-t pt-3 text-[var(--ink-2)] text-xs">
            Seventeen minutes stopped, most of it at the top of the second climb.
          </p>
        </div>
      </div>
      <MapBox className="min-h-80" />
    </div>
  );
}
