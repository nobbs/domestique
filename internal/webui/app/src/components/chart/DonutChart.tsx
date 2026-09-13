/**
 * A ring cut into shares of one whole, with something to say at its centre.
 *
 * Decoration beside a table that carries the same numbers, so it is hidden
 * from assistive technology; what it adds is the pointer. Pointing at a
 * segment names its key, and a key named from elsewhere lifts that segment
 * and sets the rest back.
 */

import type { ReactNode } from "react";

export interface DonutSegment<K extends string | number> {
  key: K;
  value: number;
  colour: string;
}

export interface DonutChartProps<K extends string | number> {
  segments: readonly DonutSegment<K>[];
  active?: K | null;
  onActive?: (key: K | null) => void;
  children?: ReactNode;
}

/** A circumference of 100 makes every dash length a percentage. */
const RADIUS = 50 / Math.PI;
/** The sliver of ground left between two segments, in percent of the ring. */
const GAP = 0.6;

export function DonutChart<K extends string | number>({
  segments,
  active = null,
  onActive,
  children,
}: DonutChartProps<K>) {
  const total = segments.reduce((sum, segment) => sum + Math.max(segment.value, 0), 0);
  let offset = 0;

  return (
    <div className="relative size-40 shrink-0">
      <svg viewBox="0 0 42 42" className="-rotate-90 size-full" aria-hidden="true">
        {total > 0
          ? segments.map((segment) => {
              const length = (Math.max(segment.value, 0) / total) * 100;
              const start = offset;
              offset += length;
              if (length === 0) {
                return null;
              }

              return (
                <circle
                  key={segment.key}
                  data-key={segment.key}
                  cx="21"
                  cy="21"
                  r={RADIUS}
                  fill="none"
                  stroke={segment.colour}
                  strokeWidth={active === segment.key ? 6.5 : 5}
                  opacity={active !== null && active !== segment.key ? 0.2 : 1}
                  strokeDasharray={`${Math.max(length - GAP, 0.3)} ${100 - length + GAP}`}
                  strokeDashoffset={-start}
                  className="transition-[opacity,stroke-width] duration-150"
                  onMouseEnter={() => onActive?.(segment.key)}
                  // Per segment, not only on the ring: the hole and the gaps are not a segment either.
                  onMouseLeave={() => onActive?.(null)}
                />
              );
            })
          : null}
      </svg>
      <div className="pointer-events-none absolute inset-0 flex flex-col items-center justify-center text-center">
        {children}
      </div>
    </div>
  );
}
