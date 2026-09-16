/**
 * Four takes on the zone ring: more room for the figure at its centre, and
 * rounded segment ends the way the sample's gauges are drawn.
 *
 * Storybook only: nothing here is imported by the application.
 */

import type { Meta, StoryObj } from "@storybook/react-vite";
import { useState } from "react";
import { StoryProviders } from "../../storybook/fixtures";

const ZONES = [
  { name: "Recovery", seconds: 3104, colour: "var(--grade-0)" },
  { name: "Endurance", seconds: 764, colour: "var(--grade-1)" },
  { name: "Tempo", seconds: 469, colour: "var(--grade-2)" },
  { name: "Threshold", seconds: 862, colour: "var(--grade-3)" },
  { name: "VO₂ max", seconds: 11_220, colour: "var(--grade-4)" },
];
const TOTAL = ZONES.reduce((sum, zone) => sum + zone.seconds, 0);

function duration(seconds: number): string {
  const h = Math.floor(seconds / 3600);
  const m = Math.round((seconds % 3600) / 60);
  return h > 0 ? `${h} h ${m} min` : `${m} min`;
}

const RADIUS = 50 / Math.PI;

/** A point on the circle at `r` from the centre, `a` radians clockwise from twelve. */
function at(r: number, a: number): string {
  return `${(21 + r * Math.sin(a)).toFixed(3)} ${(21 - r * Math.cos(a)).toFixed(3)}`;
}

/**
 * An annular sector from `a0` to `a1` with every corner rounded by `rc`.
 *
 * The rounding eats an angle of `rc / r` at each end on each edge, so the
 * corner radius is clamped to what the inner edge can spare.
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

interface RingProps {
  /** The box's side, in rem. */
  size: number;
  /** In viewBox units of 42. */
  stroke: number;
  /** Percent of the ring left clear between segments. */
  gap: number;
  /** Corner radius in viewBox units; 0 is square, half the stroke is a full round. */
  corner: number;
  /** Degrees of the circle the ring covers; 360 is a closed ring. */
  arc?: number;
  figure: "lg" | "2xl" | "3xl";
}

function Ring({ size, stroke, gap, corner, arc = 360, figure }: RingProps) {
  const [active, setActive] = useState<number | null>(null);
  const span = arc / 360;
  const text = figure === "lg" ? "text-lg" : figure === "2xl" ? "text-2xl" : "text-3xl";
  // A slot holds at least its two rounded corners and the gap; the long
  // zones give up the difference.
  const least = gap + ((2 * corner) / (2 * Math.PI * (RADIUS - stroke / 2))) * 100 + 0.2;
  const raw = ZONES.map((zone) => (zone.seconds / TOTAL) * 100 * span);
  const floored = raw.map((length) => Math.max(length, least));
  const scale = (100 * span) / floored.reduce((sum, length) => sum + length, 0);
  const slots = floored.map((length) => length * scale);
  // The lifted segment is the widest thing drawn; the frame keeps that clear.
  const lift = stroke * 0.3;
  const pad = lift + 0.5;
  // An open arc starts at the bottom left, so the opening sits under the figure.
  const first = arc === 360 ? 0 : Math.PI + ((360 - arc) / 2) * (Math.PI / 180);
  const half = ((360 - arc) / 2) * (Math.PI / 180);
  const bottom =
    arc === 360 ? 21 + RADIUS + stroke / 2 : 21 + (RADIUS + stroke / 2) * Math.cos(half);
  const top = -pad;
  const height = bottom + pad - top;
  const width = 42 + 2 * pad;
  const box = size * (height / width);
  const rIn = RADIUS - stroke / 2;
  const rOut = RADIUS + stroke / 2;
  const toAngle = (percent: number) => first + (percent / 100) * 2 * Math.PI;
  let offset = 0;

  return (
    <div
      className="relative shrink-0"
      style={{ width: `${size}rem`, height: `${box}rem`, marginBottom: arc === 360 ? 0 : "3.5rem" }}
    >
      <svg viewBox={`${-pad} ${top} ${width} ${height}`} className="size-full" aria-hidden="true">
        {arc < 360 ? (
          <path
            d={sector(rIn, rOut, toAngle(gap / 2), toAngle(span * 100 - gap / 2), corner)}
            fill="var(--muted)"
          />
        ) : null}
        {ZONES.map((zone, index) => {
          const length = slots[index] ?? 0;
          const start = offset;
          offset += length;
          const a0 = toAngle(start + gap / 2);
          const a1 = toAngle(start + length - gap / 2);
          const on = active === index;
          return (
            <path
              key={zone.name}
              d={sector(on ? rIn - lift : rIn, on ? rOut + lift : rOut, a0, a1, corner)}
              fill={zone.colour}
              opacity={active !== null && !on ? 0.2 : 1}
              className="transition-opacity duration-150"
              onMouseEnter={() => setActive(index)}
              onMouseLeave={() => setActive(null)}
            />
          );
        })}
      </svg>
      {/* Inside a closed ring; under an open one, whose middle is not its centre. */}
      <div
        className={
          arc === 360
            ? "pointer-events-none absolute inset-0 flex flex-col items-center justify-center text-center"
            : "pointer-events-none absolute inset-x-0 top-full flex flex-col items-center pt-1 text-center"
        }
      >
        <span className={`font-semibold ${text} leading-tight tabular-nums`}>
          {duration(active === null ? TOTAL : (ZONES[active]?.seconds ?? 0))}
        </span>
        <span className="text-[var(--ink-2)] text-xs">
          {active === null ? "in zones" : ZONES[active]?.name}
        </span>
      </div>
    </div>
  );
}

const meta = {
  title: "Spikes/Zone Ring",
  parameters: { layout: "fullscreen" },
} satisfies Meta;

export default meta;
type Story = StoryObj<typeof meta>;

const VARIANTS: Array<{ name: string; note: string; props: RingProps }> = [
  {
    name: "Today",
    note: "As shipped: 10rem, a 5-wide stroke, square ends.",
    props: { size: 10, stroke: 5, gap: 0.6, corner: 0, figure: "lg" },
  },
  {
    name: "A · Room",
    note: "13rem and a thinner ring; the figure steps up two sizes.",
    props: { size: 13, stroke: 3.5, gap: 0.8, corner: 0, figure: "2xl" },
  },
  {
    name: "B · Rounded",
    note: "Softened corners on every segment, a quarter of the stroke.",
    props: { size: 13, stroke: 4, gap: 1, corner: 1, figure: "2xl" },
  },
  {
    name: "C · Wide",
    note: "A fat ring with softened corners; the smallest zones become pills.",
    props: { size: 13, stroke: 7, gap: 1.2, corner: 1.6, figure: "2xl" },
  },
  {
    name: "D · Gauge",
    note: "Three quarters of a circle, open at the foot, the way the sample draws its score.",
    props: { size: 11, stroke: 5, gap: 1, corner: 1.2, arc: 270, figure: "2xl" },
  },
];

export const SideBySide: Story = {
  render: () => (
    <StoryProviders>
      <div className="flex flex-wrap items-start gap-8 bg-[var(--base)] p-6">
        {VARIANTS.map(({ name, note, props }) => (
          <div key={name} className="grid w-64 gap-3">
            <div>
              <h2 className="font-semibold text-sm">{name}</h2>
              <p className="text-[var(--ink-2)] text-xs">{note}</p>
            </div>
            <div className="grid place-items-center rounded-2xl bg-[var(--panel)] p-4 shadow-[var(--shadow)]">
              <Ring {...props} />
            </div>
          </div>
        ))}
      </div>
    </StoryProviders>
  ),
};
