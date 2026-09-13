/**
 * Bars side by side, each as wide as the span of the axis it covers and
 * belonging to a group it shares a colour with, and labelled marks on the axis.
 * A bar's height is its value per unit of span, so a bar cut narrower than its
 * neighbours is not drawn as if it held less.
 *
 * Pointing at a bar lifts that bar alone and names its group, so a chart or
 * table beside it can show the whole group; a group named from elsewhere lifts
 * its bars. The pointed bar's own reading floats above it.
 */

import { type ReactNode, useState } from "react";

export interface HistogramBar<G extends string | number> {
  value: number;
  /** Units of the axis the bar covers; 1 when absent. */
  span?: number;
  colour: string;
  group: G;
}

export interface HistogramMarker {
  /** Where on the axis the mark stands, in span units from the first bar's left edge. */
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

/** Height and one span unit's width in viewBox units; the chart is stretched to its box. */
const HEIGHT = 100;
const UNIT = 10;
/** The ground left either side of a bar, in viewBox units. */
const GAP = 1;

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
  const spans = bars.map((bar) => bar.span ?? 1);
  const starts = spans.map((_, index) =>
    spans.slice(0, index).reduce((sum, span) => sum + span, 0),
  );
  const axis = spans.reduce((sum, span) => sum + span, 0);
  const densities = bars.map((bar, index) => bar.value / (spans[index] ?? 1));
  const tallest = Math.max(0, ...densities);
  const heightOf = (index: number) =>
    tallest > 0 ? ((densities[index] ?? 0) / tallest) * HEIGHT : 0;
  const percent = (edge: number) => (edge / Math.max(axis, 1)) * 100;

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
          viewBox={`0 0 ${axis * UNIT} ${HEIGHT}`}
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
              x={(starts[index] ?? 0) * UNIT}
              y={0}
              width={(spans[index] ?? 1) * UNIT}
              height={HEIGHT}
              fill="transparent"
              onMouseEnter={() => {
                setPointed(index);
                onActiveGroup?.(bar.group);
              }}
            />
          ))}
          {bars.map((bar, index) => {
            const height = heightOf(index);
            return (
              <rect
                // biome-ignore lint/suspicious/noArrayIndexKey: a bar's place is its identity
                key={index}
                data-bar={index}
                x={(starts[index] ?? 0) * UNIT + GAP}
                y={HEIGHT - height}
                width={(spans[index] ?? 1) * UNIT - 2 * GAP}
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
              x1={marker.edge * UNIT}
              x2={marker.edge * UNIT}
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
        {pointed !== null && readout ? (
          <div
            className="-translate-x-1/2 -translate-y-full pointer-events-none absolute flex flex-col whitespace-nowrap rounded-md bg-[var(--panel)] px-2 py-1 text-xs tabular-nums shadow-md ring-1 ring-black/10"
            style={{
              // Clamped so the readout over an edge bar stays inside the chart.
              left: `${Math.min(88, Math.max(12, percent((starts[pointed] ?? 0) + (spans[pointed] ?? 1) / 2)))}%`,
              top: `calc(${100 - (heightOf(pointed) / HEIGHT) * 100}% - 6px)`,
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
