/**
 * Stacked panels over one axis of calendar days, with real ticks on both axes
 * and one crosshair whose readout covers every panel at the day it rests on.
 *
 * Days are `YYYY-MM-DD` strings, already in whatever zone the caller counts
 * days in; the frame only lays them out and never converts them. Past a
 * `todayIndex`, the days can take a fixed share of the width, so a few
 * projected weeks stay legible beside a year that was ridden.
 */

import { type KeyboardEvent, type PointerEvent, type ReactNode, useState } from "react";
import { useElementWidth } from "../../lib/useElementWidth";

/** The room reserved down the left for the value ticks. */
export const FRAME_LEFT = 40;
const RIGHT = 12;
const AXIS = 22;
const GAP = 20;
const MIN_WIDTH = 240;

export interface FramePanel {
  height: number;
  domain: [number, number];
  /** Values to rule and label; round ones across the domain when absent. */
  ticks?: number[];
  format?: (value: number) => string;
  title?: string;
  /** `active` is the pointed day, for a bar chart to lift its bar and set the rest back. */
  draw: (
    x: (index: number) => number,
    y: (value: number) => number,
    active: number | null,
  ) => ReactNode;
}

export interface TimeFrameProps {
  dates: readonly string[];
  panels: readonly FramePanel[];
  label: string;
  readout: (index: number) => ReactNode;
  /** Days the crosshair may rest on; every day when absent. */
  snap?: readonly number[];
  /** The last day ridden; the days after it are projected. */
  todayIndex?: number;
  /** Width share the days after `todayIndex` take, whatever their count; linear when absent. */
  futureShare?: number;
  /** A bar chart: the pointed bar is lifted by its own draw, so no crosshair is ruled over it. */
  bars?: boolean;
  /** Heads the readout; the pointed day's date when absent. */
  heading?: (index: number) => ReactNode;
}

/** Round ticks covering [low, high], about `count` of them. */
export function niceTicks(low: number, high: number, count = 4): number[] {
  const raw = (high - low || 1) / count;
  const power = 10 ** Math.floor(Math.log10(raw));
  const step = ([1, 2, 5, 10].find((multiple) => multiple * power >= raw) ?? 10) * power;
  const ticks: number[] = [];
  for (let tick = Math.ceil(low / step) * step; tick <= high + step * 1e-9; tick += step) {
    ticks.push(Math.round(tick * 1e6) / 1e6);
  }

  return ticks;
}

const dayMonth = new Intl.DateTimeFormat("en-GB", {
  day: "numeric",
  month: "short",
  timeZone: "UTC",
});
const monthOnly = new Intl.DateTimeFormat("en-GB", { month: "short", timeZone: "UTC" });

/** A calendar day as a reader says it, "14 Sept". */
export function formatCalendarDay(date: string): string {
  return dayMonth.format(new Date(`${date}T00:00:00Z`));
}

/** Month starts on a long frame, Mondays on a short one. */
function dateTicks(dates: readonly string[]): Array<{ index: number; label: string }> {
  const long = dates.length > 120;
  const every = dates.length > 70 ? 2 : 1;
  let mondays = 0;

  return dates.flatMap((date, index) => {
    const at = new Date(`${date}T00:00:00Z`);
    if (long) {
      return at.getUTCDate() === 1 ? [{ index, label: monthOnly.format(at) }] : [];
    }
    if (at.getUTCDay() !== 1) {
      return [];
    }
    mondays += 1;

    return mondays % every === 0 ? [{ index, label: dayMonth.format(at) }] : [];
  });
}

/** Room a date label needs; a tick nearer its predecessor than this goes unlabelled. */
const LABEL_SPACING = 44;

function spaced<T extends { index: number }>(
  ticks: readonly T[],
  x: (index: number) => number,
): T[] {
  let previous = -Infinity;

  return ticks.filter((tick) => {
    if (x(tick.index) - previous < LABEL_SPACING) {
      return false;
    }
    previous = x(tick.index);

    return true;
  });
}

/** Where each day sits across the plot, 0 to 1, and back again. */
export function frameScale(count: number, todayIndex?: number, futureShare?: number) {
  const last = Math.max(count - 1, 1);
  const split =
    futureShare !== undefined && todayIndex !== undefined && todayIndex > 0 && todayIndex < last
      ? { index: todayIndex, at: 1 - futureShare }
      : { index: last, at: 1 };

  return {
    at: (index: number) =>
      index <= split.index
        ? (index / split.index) * split.at
        : split.at + ((index - split.index) / (last - split.index)) * (1 - split.at),
    index: (fraction: number) =>
      fraction <= split.at
        ? (fraction / split.at) * split.index
        : split.index + ((fraction - split.at) / (1 - split.at)) * (last - split.index),
  };
}

/** The box every chart's readout floats in: the page's primary ink, its text the primary's foreground. */
const READOUT_BOX =
  "pointer-events-none absolute top-0 z-10 min-w-40 rounded-lg bg-[var(--primary)] px-3 py-2 text-[var(--primary-foreground)] text-xs shadow-lg";

/** An SVG path for a bar whose top corners are rounded by `radius` and whose foot stays square. */
export function topRoundedBar(x: number, y: number, width: number, height: number, radius = 4) {
  const r = Math.max(0, Math.min(radius, width / 2, height));
  return `M${x},${y + height}V${y + r}A${r},${r} 0 0 1 ${x + r},${y}H${x + width - r}A${r},${r} 0 0 1 ${x + width},${y + r}V${y + height}Z`;
}

/** A bar's opacity while `active` is pointed: set back unless it is the pointed one. */
export function barOpacity(active: number | null, index: number): number {
  return active === null || active === index ? 1 : 0.2;
}

export function TimeFrame({
  dates,
  panels,
  label,
  readout,
  snap,
  todayIndex,
  futureShare,
  bars = false,
  heading,
}: TimeFrameProps) {
  const { ref, width: measured } = useElementWidth<HTMLDivElement>();
  const width = Math.max(measured, MIN_WIDTH);
  const [active, setActive] = useState<number | null>(null);
  const plotWidth = width - FRAME_LEFT - RIGHT;
  const scale = frameScale(dates.length, todayIndex, futureShare);
  const x = (index: number) => FRAME_LEFT + scale.at(index) * plotWidth;
  const height = panels.reduce((sum, panel) => sum + panel.height + GAP, 0) + AXIS;
  const lastIndex = dates.length - 1;
  // Callers pass days in whatever order their rows came, repeats included; the keys walk them in date order.
  const stops = snap ? [...new Set(snap)].sort((one, other) => one - other) : [];

  const nearest = (index: number) =>
    stops.length > 0
      ? stops.reduce((best, candidate) =>
          Math.abs(candidate - index) < Math.abs(best - index) ? candidate : best,
        )
      : Math.min(Math.max(Math.round(index), 0), lastIndex);

  // A bar spans its day to the next stop, so the one pointed at is the last that starts at or before the pointer.
  const under = (index: number) =>
    stops.length > 0
      ? (stops.findLast((stop) => stop <= index) ?? stops[0] ?? 0)
      : Math.min(Math.max(Math.floor(index), 0), lastIndex);

  const onPointer = (event: PointerEvent<SVGRectElement>) => {
    const box = event.currentTarget.getBoundingClientRect();
    const index = scale.index((event.clientX - box.left) / (box.width || 1));
    setActive(bars ? under(index) : nearest(index));
  };
  const onKey = (event: KeyboardEvent<HTMLDivElement>) => {
    const step = event.key === "ArrowLeft" ? -1 : event.key === "ArrowRight" ? 1 : 0;
    if (step === 0) {
      return;
    }
    event.preventDefault();
    const from = active ?? todayIndex ?? lastIndex;
    if (stops.length > 0) {
      const position = stops.indexOf(nearest(from));
      setActive(stops[Math.min(Math.max(position + step, 0), stops.length - 1)] ?? from);
    } else {
      setActive(Math.min(Math.max(from + step, 0), lastIndex));
    }
  };

  let top = GAP;
  const offsets = panels.map((panel) => {
    const offset = top;
    top += panel.height + GAP;

    return offset;
  });
  const floor = height - AXIS;

  return (
    // Focusable so the arrow keys walk the same crosshair the pointer moves.
    <div
      ref={ref}
      className="relative outline-none focus-visible:ring-2 focus-visible:ring-[var(--accent)]"
      tabIndex={0}
      onKeyDown={onKey}
      onBlur={() => setActive(null)}
    >
      {/* Named by aria-label, not <title>: a browser shows a <title> as its own tooltip over the readout. */}
      <svg width={width} height={height} role="img" aria-label={label} className="block">
        {panels.map((panel, panelIndex) => {
          const offset = offsets[panelIndex] ?? 0;
          const [low, high] = panel.domain;
          const y = (value: number) =>
            offset + panel.height - ((value - low) / (high - low || 1)) * panel.height;
          const format = panel.format ?? String;

          return (
            <g key={panel.title ?? panelIndex}>
              {(panel.ticks ?? niceTicks(low, high)).map((tick) => (
                <g key={tick}>
                  <line
                    x1={FRAME_LEFT}
                    x2={width - RIGHT}
                    y1={y(tick)}
                    y2={y(tick)}
                    stroke="var(--rule)"
                    strokeWidth={tick === 0 ? 1 : 0.5}
                    strokeDasharray={tick === 0 ? undefined : "2 3"}
                  />
                  <text
                    x={FRAME_LEFT - 6}
                    y={y(tick)}
                    dy="0.32em"
                    textAnchor="end"
                    className="fill-[var(--ink-2)] text-[10px] tabular-nums"
                  >
                    {format(tick)}
                  </text>
                </g>
              ))}
              {panel.title ? (
                <text
                  x={FRAME_LEFT + 4}
                  y={offset - 7}
                  className="fill-[var(--ink-2)] font-medium text-[10px]"
                >
                  {panel.title}
                </text>
              ) : null}
              {panel.draw(x, y, active)}
            </g>
          );
        })}
        {spaced(dateTicks(dates), x).map((tick) => (
          <text
            key={tick.index}
            x={x(tick.index)}
            y={height - 6}
            textAnchor="middle"
            className="fill-[var(--ink-2)] text-[10px]"
          >
            {tick.label}
          </text>
        ))}
        {todayIndex !== undefined && todayIndex < lastIndex ? (
          <g>
            <line
              x1={x(todayIndex)}
              x2={x(todayIndex)}
              y1={0}
              y2={floor}
              stroke="var(--ink-2)"
              strokeDasharray="1 3"
            />
            <text
              x={x(todayIndex) + 6}
              y={GAP - 7}
              className="fill-[var(--ink-2)] font-medium text-[10px]"
            >
              Projected
            </text>
          </g>
        ) : null}
        {active !== null && !bars ? (
          <line
            x1={x(active)}
            x2={x(active)}
            y1={0}
            y2={floor}
            stroke="var(--ink)"
            strokeWidth={1}
            opacity={0.5}
          />
        ) : null}
        <rect
          x={FRAME_LEFT}
          y={0}
          width={plotWidth}
          height={floor}
          fill="transparent"
          className={bars ? "cursor-pointer" : undefined}
          onPointerMove={onPointer}
          onPointerLeave={() => setActive(null)}
        />
      </svg>
      {active !== null ? (
        <div
          role="status"
          className={READOUT_BOX}
          style={
            x(active) > width / 2 ? { right: width - x(active) + 10 } : { left: x(active) + 10 }
          }
        >
          <div className="mb-1 font-medium">
            {heading ? heading(active) : formatCalendarDay(dates[active] ?? "")}
          </div>
          {readout(active)}
        </div>
      ) : null}
    </div>
  );
}

/** One readout row: the value leads, a short stroke keys the series. */
export function ReadoutRow({
  colour,
  label,
  value,
  dashed = false,
}: {
  colour?: string;
  label: string;
  value: string;
  dashed?: boolean;
}) {
  return (
    <div className="flex items-center gap-2">
      <svg width={12} height={4} aria-hidden="true">
        {colour ? (
          <line
            x1={0}
            x2={12}
            y1={2}
            y2={2}
            // Ink on the ink box would vanish; the box's own text colour is the same series inverted.
            stroke={colour === "var(--ink)" ? "currentColor" : colour}
            strokeWidth={2}
            strokeDasharray={dashed ? "3 2" : undefined}
          />
        ) : null}
      </svg>
      <span className="font-semibold tabular-nums">{value}</span>
      <span className="opacity-70">{label}</span>
    </div>
  );
}

export interface LegendItem {
  label: string;
  colour: string;
  dashed?: boolean;
  /** A filled square, for bars and dots, rather than a stroke for a line. */
  swatch?: boolean;
}

export function ChartLegend({ items }: { items: readonly LegendItem[] }) {
  return (
    <ul className="flex flex-wrap gap-x-4 gap-y-1 text-[var(--ink-2)] text-xs">
      {items.map((item) => (
        <li key={item.label} className="flex items-center gap-1.5">
          <svg width={14} height={10} aria-hidden="true">
            {item.swatch ? (
              <rect width={10} height={10} rx={2} fill={item.colour} />
            ) : (
              <line
                x1={0}
                x2={14}
                y1={5}
                y2={5}
                stroke={item.colour}
                strokeWidth={2}
                strokeDasharray={item.dashed ? "3 2" : undefined}
              />
            )}
          </svg>
          {item.label}
        </li>
      ))}
    </ul>
  );
}
