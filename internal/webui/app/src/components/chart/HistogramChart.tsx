/**
 * Equal-width bars side by side, each belonging to a group it shares a colour
 * with, and labelled marks at chosen bar edges.
 *
 * Pointing at a bar lifts that bar alone and names its group, so a chart or
 * table beside it can show the whole group; a group named from elsewhere lifts
 * its bars. The pointed bar's own reading floats above it.
 */

import { type ReactNode, useState } from "react";

export interface HistogramBar<G extends string | number> {
  value: number;
  colour: string;
  group: G;
}

export interface HistogramMarker {
  /** The bar edge the mark stands on: 0 is the left of the first bar. */
  edge: number;
  label: string;
}

export interface HistogramChartProps<G extends string | number> {
  label: string;
  bars: readonly HistogramBar<G>[];
  markers?: readonly HistogramMarker[];
  /** Names the axis at its right-hand end. */
  unit?: string;
  activeGroup?: G | null;
  onActiveGroup?: (group: G | null) => void;
  /** The reading shown above a pointed bar. */
  readout?: (index: number) => ReactNode;
}

/** Height and bar width in viewBox units; the chart is stretched to its box. */
const HEIGHT = 100;
const BAR = 10;

export function HistogramChart<G extends string | number>({
  label,
  bars,
  markers = [],
  unit,
  activeGroup = null,
  onActiveGroup,
  readout,
}: HistogramChartProps<G>) {
  const [pointed, setPointed] = useState<number | null>(null);
  const tallest = Math.max(0, ...bars.map((bar) => bar.value));
  const width = bars.length * BAR;
  const percent = (edge: number) => (edge / Math.max(bars.length, 1)) * 100;
  const pointedBar = pointed === null ? undefined : bars[pointed];

  const opacity = (index: number, group: G) => {
    if (pointed !== null) {
      return index === pointed ? 1 : 0.2;
    }

    return activeGroup !== null && activeGroup !== group ? 0.2 : 1;
  };

  return (
    <div className="flex flex-col gap-1">
      <div className="relative">
        <svg
          viewBox={`0 0 ${width} ${HEIGHT}`}
          preserveAspectRatio="none"
          className="block h-32 w-full"
          role="img"
          aria-label={label}
          onMouseLeave={() => {
            setPointed(null);
            onActiveGroup?.(null);
          }}
        >
          {/* The whole column is the target, so a short bar is as easy to point at as a tall one. */}
          {bars.map((bar, index) => (
            <rect
              // biome-ignore lint/suspicious/noArrayIndexKey: a bar's place is its identity
              key={index}
              x={index * BAR}
              y={0}
              width={BAR}
              height={HEIGHT}
              fill="transparent"
              onMouseEnter={() => {
                setPointed(index);
                onActiveGroup?.(bar.group);
              }}
            />
          ))}
          {bars.map((bar, index) => {
            const height = tallest > 0 ? (bar.value / tallest) * HEIGHT : 0;
            return (
              <rect
                // biome-ignore lint/suspicious/noArrayIndexKey: a bar's place is its identity
                key={index}
                data-bar={index}
                x={index * BAR + 1}
                y={HEIGHT - height}
                width={BAR - 2}
                height={height}
                fill={bar.colour}
                opacity={opacity(index, bar.group)}
                pointerEvents="none"
              />
            );
          })}
          {markers.map((marker) => (
            <line
              key={marker.edge}
              x1={marker.edge * BAR}
              x2={marker.edge * BAR}
              y1={0}
              y2={HEIGHT}
              stroke="var(--ink-2)"
              strokeDasharray="3 3"
              strokeWidth={1}
              vectorEffect="non-scaling-stroke"
              pointerEvents="none"
            />
          ))}
        </svg>
        {pointed !== null && pointedBar && readout ? (
          <div
            className="-translate-x-1/2 -translate-y-full pointer-events-none absolute flex flex-col whitespace-nowrap rounded-md bg-[var(--panel)] px-2 py-1 text-xs tabular-nums shadow-md ring-1 ring-black/10"
            style={{
              // Clamped so the readout over an edge bar stays inside the chart.
              left: `${Math.min(88, Math.max(12, percent(pointed + 0.5)))}%`,
              top: `calc(${tallest > 0 ? (1 - pointedBar.value / tallest) * 100 : 100}% - 6px)`,
            }}
          >
            {readout(pointed)}
          </div>
        ) : null}
      </div>
      {markers.length > 0 || unit ? (
        <div
          aria-hidden="true"
          className="relative h-4 text-[10px] text-[var(--ink-2)] tabular-nums"
        >
          {markers.map((marker) => (
            <span
              key={marker.edge}
              className="-translate-x-1/2 absolute"
              style={{ left: `${percent(marker.edge)}%` }}
            >
              {marker.label}
            </span>
          ))}
          {unit ? <span className="absolute right-0">{unit}</span> : null}
        </div>
      ) : null}
    </div>
  );
}
