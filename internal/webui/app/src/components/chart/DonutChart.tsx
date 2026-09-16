/**
 * A gauge cut into shares of one whole: three quarters of a ring, open at
 * the foot, with something to say beneath it.
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

/** A circumference of 100 makes every share a percentage of the full circle. */
const RADIUS = 50 / Math.PI;
/** How much of the circle the gauge covers, in percent; the rest is the opening at the foot. */
const ARC = 75;
const STROKE = 5;
/** How far a lifted segment grows, outward and inward. */
const LIFT = 1.5;
/** The corner radius on every segment: a quarter of the stroke, softened rather than round. */
const CORNER = 1.2;
/** The sliver of ground left between two segments, in percent of the circle. */
const GAP = 1;
const R_IN = RADIUS - STROKE / 2;
const R_OUT = RADIUS + STROKE / 2;
/** The frame keeps a lifted segment clear on every side. */
const PAD = LIFT + 0.5;
/** Where the arc begins, in radians clockwise from twelve: the bottom left, so the opening sits under the figure. */
const FIRST = Math.PI + ((100 - ARC) / 100) * Math.PI;
/** The lowest point the arc reaches; the frame stops there rather than at the circle's foot. */
const BOTTOM = 21 + R_OUT * Math.cos(((100 - ARC) / 100) * Math.PI);
const TOP = -PAD;
const HEIGHT = BOTTOM + PAD - TOP;
const WIDTH = 42 + 2 * PAD;

/** A point on the circle at `r` from the centre, `a` radians clockwise from twelve. */
function at(r: number, a: number): string {
  return `${(21 + r * Math.sin(a)).toFixed(3)} ${(21 - r * Math.cos(a)).toFixed(3)}`;
}

/**
 * An annular sector from `a0` to `a1` with every corner rounded by `rc`. The
 * rounding eats an angle of `rc / r` at each end on each edge, so the radius
 * is clamped to what the inner edge can spare.
 */
function sector(rIn: number, rOut: number, a0: number, a1: number, rc: number): string {
  const span = a1 - a0;
  const r = Math.min(rc, (rOut - rIn) / 2, (span * rIn) / 2);
  const dOut = r / rOut;
  const dIn = r / rIn;
  const big = (angle: number) => (angle > Math.PI ? 1 : 0);
  if (r <= 0.001) {
    return `M ${at(rOut, a0)} A ${rOut} ${rOut} 0 ${big(span)} 1 ${at(rOut, a1)} L ${at(rIn, a1)} A ${rIn} ${rIn} 0 ${big(span)} 0 ${at(rIn, a0)} Z`;
  }
  return [
    `M ${at(rOut, a0 + dOut)}`,
    `A ${rOut} ${rOut} 0 ${big(span - 2 * dOut)} 1 ${at(rOut, a1 - dOut)}`,
    `A ${r} ${r} 0 0 1 ${at(rOut - r, a1)}`,
    `L ${at(rIn + r, a1)}`,
    `A ${r} ${r} 0 0 1 ${at(rIn, a1 - dIn)}`,
    `A ${rIn} ${rIn} 0 ${big(span - 2 * dIn)} 0 ${at(rIn, a0 + dIn)}`,
    `A ${r} ${r} 0 0 1 ${at(rIn + r, a0)}`,
    `L ${at(rOut - r, a0)}`,
    `A ${r} ${r} 0 0 1 ${at(rOut, a0 + dOut)}`,
    "Z",
  ].join(" ");
}

function angle(percent: number): number {
  return FIRST + (percent / 100) * 2 * Math.PI;
}

/**
 * Each share's slot along the arc, in percent of the circle. A slot holds at
 * least its two corners and the gap, so a sliver still shows as a pill; the
 * long shares give up the difference.
 */
function slots(values: readonly number[]): number[] {
  const total = values.reduce((sum, value) => sum + value, 0);
  if (total <= 0) {
    return values.map(() => 0);
  }
  const least = GAP + ((2 * CORNER) / (2 * Math.PI * R_IN)) * 100 + 0.2;
  const floored = values.map((value) => (value > 0 ? Math.max((value / total) * ARC, least) : 0));
  const scale = ARC / floored.reduce((sum, length) => sum + length, 0);

  return floored.map((length) => length * scale);
}

export function DonutChart<K extends string | number>({
  segments,
  active = null,
  onActive,
  children,
}: DonutChartProps<K>) {
  const lengths = slots(segments.map((segment) => Math.max(segment.value, 0)));
  let offset = 0;

  return (
    <div className="flex w-56 shrink-0 flex-col items-center">
      <svg
        viewBox={`${-PAD} ${TOP} ${WIDTH} ${HEIGHT}`}
        className="w-full"
        style={{ aspectRatio: `${WIDTH} / ${HEIGHT}` }}
        aria-hidden="true"
      >
        <path
          d={sector(R_IN, R_OUT, angle(GAP / 2), angle(ARC - GAP / 2), CORNER)}
          fill="var(--muted)"
        />
        {segments.map((segment, index) => {
          const length = lengths[index] ?? 0;
          const start = offset;
          offset += length;
          if (length === 0) {
            return null;
          }
          const on = active === segment.key;

          return (
            <path
              key={segment.key}
              data-key={segment.key}
              d={sector(
                on ? R_IN - LIFT : R_IN,
                on ? R_OUT + LIFT : R_OUT,
                angle(start + GAP / 2),
                angle(start + length - GAP / 2),
                CORNER,
              )}
              fill={segment.colour}
              opacity={active !== null && !on ? 0.2 : 1}
              className="transition-opacity duration-150"
              onMouseEnter={() => onActive?.(segment.key)}
              // Per segment, not only on the ring: the hole and the gaps are not a segment either.
              onMouseLeave={() => onActive?.(null)}
            />
          );
        })}
      </svg>
      <div className="pointer-events-none mt-1 flex flex-col items-center text-center">
        {children}
      </div>
    </div>
  );
}
