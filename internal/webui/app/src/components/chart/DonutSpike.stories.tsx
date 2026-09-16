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

interface RingProps {
  /** The box's side, in rem. */
  size: number;
  /** In viewBox units of 42. */
  stroke: number;
  /** Percent of the ring left clear between segments. */
  gap: number;
  round: boolean;
  /** Degrees of the circle the ring covers; 360 is a closed ring. */
  arc?: number;
  figure: "lg" | "2xl" | "3xl";
}

function Ring({ size, stroke, gap, round, arc = 360, figure }: RingProps) {
  const [active, setActive] = useState<number | null>(null);
  const span = arc / 360;
  // With round ends a dash is drawn `stroke` shorter than its slot: half a
  // stroke of cap grows back at each end. The slot itself is never smaller
  // than one dot plus the gap, and the long zones give up the difference.
  const reserve = round ? stroke + gap : gap;
  const raw = ZONES.map((zone) => (zone.seconds / TOTAL) * 100 * span);
  const floored = raw.map((length) => Math.max(length, round ? reserve + 0.01 : length));
  const scale = (100 * span) / floored.reduce((sum, length) => sum + length, 0);
  const slots = floored.map((length) => length * scale);
  let offset = 0;
  const text = figure === "lg" ? "text-lg" : figure === "2xl" ? "text-2xl" : "text-3xl";
  // The lifted stroke is the widest thing drawn, and a round cap reaches half
  // of it past the arc: the frame keeps that much clear on every side.
  const pad = (stroke * 1.3) / 2 + 0.5;
  // An open arc starts at the bottom left, so the gap sits under the figure.
  const rotate = arc === 360 ? -90 : 90 + (360 - arc) / 2;
  // The lowest point the arc reaches, so the frame stops there instead of at
  // the circle's foot; a closed ring reaches the whole way down.
  const half = ((360 - arc) / 2) * (Math.PI / 180);
  const bottom = arc === 360 ? 21 + RADIUS : 21 + RADIUS * Math.cos(half);
  const top = -pad;
  const height = bottom + pad - top;
  const width = 42 + 2 * pad;
  const box = size * (height / width);

  return (
    <div
      className="relative shrink-0"
      style={{ width: `${size}rem`, height: `${box}rem`, marginBottom: arc === 360 ? 0 : "3.5rem" }}
    >
      <svg viewBox={`${-pad} ${top} ${width} ${height}`} className="size-full" aria-hidden="true">
        <g transform={`rotate(${rotate} 21 21)`}>
          {arc < 360 ? (
            <circle
              cx="21"
              cy="21"
              r={RADIUS}
              fill="none"
              stroke="var(--muted)"
              strokeWidth={stroke}
              strokeLinecap={round ? "round" : "butt"}
              // Trimmed and shifted like the segments, so its caps end where theirs do.
              strokeDasharray={`${span * 100 - reserve} ${100 - span * 100 + reserve}`}
              strokeDashoffset={-(reserve / 2)}
            />
          ) : null}
          {ZONES.map((zone, index) => {
            const length = slots[index] ?? 0;
            const start = offset;
            offset += length;
            const drawn = Math.max(length - reserve, 0.01);
            return (
              <circle
                key={zone.name}
                cx="21"
                cy="21"
                r={RADIUS}
                fill="none"
                stroke={zone.colour}
                strokeWidth={active === index ? stroke * 1.3 : stroke}
                strokeLinecap={round ? "round" : "butt"}
                opacity={active !== null && active !== index ? 0.2 : 1}
                strokeDasharray={`${drawn} ${100 - drawn}`}
                strokeDashoffset={-(start + reserve / 2)}
                className="transition-[opacity,stroke-width] duration-150"
                onMouseEnter={() => setActive(index)}
                onMouseLeave={() => setActive(null)}
              />
            );
          })}
        </g>
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
    props: { size: 10, stroke: 5, gap: 0.6, round: false, figure: "lg" },
  },
  {
    name: "A · Room",
    note: "13rem and a thinner ring; the figure steps up two sizes.",
    props: { size: 13, stroke: 3.5, gap: 0.8, round: false, figure: "2xl" },
  },
  {
    name: "B · Rounded",
    note: "Round ends on every segment, with the gap widened so they do not touch.",
    props: { size: 13, stroke: 4, gap: 1, round: true, figure: "2xl" },
  },
  {
    name: "C · Wide",
    note: "A fat rounded ring; the smallest zones become dots.",
    props: { size: 13, stroke: 7, gap: 1.2, round: true, figure: "2xl" },
  },
  {
    name: "D · Gauge",
    note: "Three quarters of a circle, open at the foot, the way the sample draws its score.",
    props: { size: 11, stroke: 5, gap: 1, round: true, arc: 270, figure: "2xl" },
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
