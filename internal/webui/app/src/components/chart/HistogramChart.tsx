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
import { useElementWidth } from "../../lib/useElementWidth";
import { topRoundedBar } from "./TimeFrame";

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

/** The plot's height, and the narrowest it is drawn before it is measured, in pixels. */
const HEIGHT = 128;
const MIN_WIDTH = 240;
/** The ground left either side of a bar, in pixels. */
const GAP = 1;

export interface BarBox {
  x: number;
  y: number;
  width: number;
  height: number;
}

/**
 * Where each bar stands in a plot `width` pixels wide and HEIGHT tall: across the
 * span of the axis it covers, as tall as its value per unit against the densest.
 */
export function histogramBoxes(bars: readonly { value: number; span?: number }[], width: number) {
  const spans = bars.map((bar) => bar.span ?? 1);
  const axis = Math.max(
    spans.reduce((sum, span) => sum + span, 0),
    1,
  );
  const densities = bars.map((bar, index) => bar.value / (spans[index] ?? 1));
  const tallest = Math.max(0, ...densities);
  let start = 0;

  return bars.map((_, index): BarBox => {
    const span = spans[index] ?? 1;
    const height = tallest > 0 ? ((densities[index] ?? 0) / tallest) * HEIGHT : 0;
    const box = {
      x: (start / axis) * width + GAP,
      y: HEIGHT - height,
      width: Math.max((span / axis) * width - 2 * GAP, 1),
      height,
    };
    start += span;
    return box;
  });
}

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
  const { ref, width: measured } = useElementWidth<HTMLDivElement>();
  const width = Math.max(measured, MIN_WIDTH);
  const boxes = histogramBoxes(bars, width);
  const axis = bars.reduce((sum, bar) => sum + (bar.span ?? 1), 0);
  const percent = (edge: number) => (edge / Math.max(axis, 1)) * 100;
  const columnOf = (index: number) => {
    const box = boxes[index];
    return box ? { x: box.x - GAP, width: box.width + 2 * GAP } : { x: 0, width: 0 };
  };

  const opacity = (index: number, group: G) => {
    if (pointed !== null) {
      return index === pointed ? 1 : 0.2;
    }

    return activeGroup !== null && activeGroup !== group ? 0.2 : 1;
  };

  return (
    <div className="flex flex-col gap-1">
      <div ref={ref} className="relative">
        <svg
          width={width}
          height={HEIGHT}
          className="block"
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
              {...columnOf(index)}
              y={0}
              height={HEIGHT}
              fill="transparent"
              className="cursor-pointer"
              onMouseEnter={() => {
                setPointed(index);
                onActiveGroup?.(bar.group);
              }}
            />
          ))}
          {bars.map((bar, index) => {
            const box = boxes[index];
            return box ? (
              <path
                // biome-ignore lint/suspicious/noArrayIndexKey: a bar's place is its identity
                key={index}
                data-bar={index}
                d={topRoundedBar(box.x, box.y, box.width, box.height)}
                fill={bar.colour}
                opacity={opacity(index, bar.group)}
                pointerEvents="none"
              />
            ) : null;
          })}
          {markers.map((marker) => {
            const x = (marker.edge / Math.max(axis, 1)) * width;
            return (
              <line
                key={marker.edge}
                x1={x}
                x2={x}
                y1={0}
                y2={HEIGHT}
                stroke="var(--ink-2)"
                strokeDasharray="3 3"
                strokeWidth={1}
                pointerEvents="none"
              />
            );
          })}
        </svg>
        {pointed !== null && readout && boxes[pointed] ? (
          <div
            className="-translate-x-1/2 -translate-y-full pointer-events-none absolute flex flex-col whitespace-nowrap rounded-lg bg-[var(--primary)] px-2.5 py-1.5 text-[var(--primary-foreground)] text-xs tabular-nums shadow-lg"
            style={{
              // Clamped so the readout over an edge bar stays inside the chart.
              left: `${Math.min(88, Math.max(12, ((boxes[pointed].x + boxes[pointed].width / 2) / width) * 100))}%`,
              top: boxes[pointed].y - 6,
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
