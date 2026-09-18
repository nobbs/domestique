/**
 * Five ways the weekly chart's hover readout could read one week.
 *
 * Storybook only: the boxes are drawn still, at the size the chart floats them.
 */

import type { Meta, StoryObj } from "@storybook/react-vite";
import { IconMountain, IconRoute, IconStopwatch } from "@tabler/icons-react";
import type { ReactNode } from "react";

interface Week {
  label: string;
  range: string;
  outdoorKm: number;
  indoorKm: number;
  time: string;
  ascent: string;
  rides: number;
}

const MIXED: Week = {
  label: "30 Mar",
  range: "30 Mar – 5 Apr",
  outdoorKm: 102,
  indoorKm: 41,
  time: "6 h 12 min",
  ascent: "1,184 m",
  rides: 4,
};
const OUTDOOR: Week = {
  ...MIXED,
  label: "7 Sep",
  range: "7 – 13 Sep",
  indoorKm: 0,
  outdoorKm: 177,
};

const OUT = "var(--ground-outdoor)";
const IN = "var(--ground-indoor)";
const km = (value: number) => `${value} km`;
const grounds = (week: Week) =>
  [
    { name: "Outdoor", km: week.outdoorKm, colour: OUT },
    { name: "Indoor", km: week.indoorKm, colour: IN },
  ].filter((ground) => ground.km > 0);
const total = (week: Week) => week.outdoorKm + week.indoorKm;

const BOX = "w-max rounded-md bg-[var(--panel)] px-2.5 py-2 text-xs shadow-md ring-1 ring-black/10";

function Swatch({ colour }: { colour: string }) {
  return (
    <span
      aria-hidden="true"
      className="size-2 shrink-0 rounded-[2px]"
      style={{ background: colour }}
    />
  );
}

/** A · Today's, tidied: date, the week's line, a row per ground ridden. */
function Current({ week }: { week: Week }) {
  return (
    <div className={BOX}>
      <div className="mb-1 font-medium">{week.label}</div>
      <div className="mb-1 text-[var(--ink-2)] tabular-nums">
        {km(total(week))} · {week.time} · {week.ascent} · {week.rides} rides
      </div>
      {grounds(week).map((ground) => (
        <div key={ground.name} className="flex items-center gap-2">
          <svg width={12} height={4} aria-hidden="true">
            <line x1={0} x2={12} y1={2} y2={2} stroke={ground.colour} strokeWidth={2} />
          </svg>
          <span className="font-semibold tabular-nums">{km(ground.km)}</span>
          <span className="text-[var(--ink-2)]">{ground.name}</span>
        </div>
      ))}
    </div>
  );
}

/** B · Headline: the week's distance large, its split as a bar, the rest beneath. */
function Headline({ week }: { week: Week }) {
  return (
    <div className={`${BOX} flex w-56 flex-col gap-1.5 px-3 py-2.5`}>
      <div className="flex items-baseline justify-between gap-3">
        <span className="text-[var(--ink-2)]">{week.range}</span>
        <span className="text-[var(--ink-2)]">{week.rides} rides</span>
      </div>
      <div className="font-semibold text-xl tabular-nums tracking-tight">{km(total(week))}</div>
      <div className="flex h-1.5 gap-0.5 overflow-hidden rounded-full">
        {grounds(week).map((ground) => (
          <div key={ground.name} style={{ flexGrow: ground.km, background: ground.colour }} />
        ))}
      </div>
      <div className="flex gap-3">
        {grounds(week).map((ground) => (
          <span key={ground.name} className="flex items-center gap-1 tabular-nums">
            <Swatch colour={ground.colour} />
            {km(ground.km)}
          </span>
        ))}
      </div>
      <div className="text-[var(--ink-2)] tabular-nums">
        {week.time} · {week.ascent}
      </div>
    </div>
  );
}

/** C · Ledger: a small table, ground by ground, with its share, the total ruled off beneath. */
function Ledger({ week }: { week: Week }) {
  return (
    <div className={`${BOX} w-56 px-3 py-2.5`}>
      <div className="mb-1.5 font-medium">Week of {week.label}</div>
      <div className="grid grid-cols-[auto_1fr_auto] items-center gap-x-2 gap-y-1 tabular-nums">
        {grounds(week).map((ground) => (
          <div key={ground.name} className="contents">
            <Swatch colour={ground.colour} />
            <span className="text-[var(--ink-2)]">{ground.name}</span>
            <span className="text-right">
              {km(ground.km)}{" "}
              <span className="text-[var(--ink-2)]">
                {Math.round((ground.km / total(week)) * 100)}%
              </span>
            </span>
          </div>
        ))}
        <div className="col-span-3 my-0.5 border-[var(--rule)] border-t" />
        <span />
        <span className="font-medium">Total</span>
        <span className="text-right font-semibold">{km(total(week))}</span>
      </div>
      <div className="mt-1.5 text-[var(--ink-2)] tabular-nums">
        {week.rides} rides · {week.time} · {week.ascent}
      </div>
    </div>
  );
}

/** D · Ink: the page's dark primary as the box, one compact line per figure. */
function Ink({ week }: { week: Week }) {
  return (
    <div className="w-max rounded-lg bg-[#2d2b28] px-3 py-2 text-[#f4f1ea] text-xs shadow-lg">
      <div className="mb-1 font-medium">{week.range}</div>
      {grounds(week).map((ground) => (
        <div key={ground.name} className="flex items-center justify-between gap-6 tabular-nums">
          <span className="flex items-center gap-1.5 opacity-80">
            <Swatch colour={ground.colour} />
            {ground.name}
          </span>
          {km(ground.km)}
        </div>
      ))}
      <div className="mt-1 flex justify-between gap-6 border-white/15 border-t pt-1 font-semibold tabular-nums">
        <span>{week.rides} rides</span>
        {km(total(week))}
      </div>
    </div>
  );
}

/** E · Tiles: the figure strip in miniature, a tile per measure with the split under distance. */
function Tiles({ week }: { week: Week }) {
  const tile = (icon: ReactNode, value: string, under?: ReactNode) => (
    <div className="flex flex-col gap-0.5 rounded-md bg-[color-mix(in_oklab,var(--ink-2)_8%,transparent)] px-2 py-1.5">
      <span className="flex items-center gap-1 font-semibold tabular-nums">
        {icon}
        {value}
      </span>
      {under}
    </div>
  );
  return (
    <div className={`${BOX} flex w-64 flex-col gap-1.5 p-2`}>
      <div className="flex justify-between px-0.5">
        <span className="font-medium">{week.range}</span>
        <span className="text-[var(--ink-2)]">{week.rides} rides</span>
      </div>
      <div className="grid grid-cols-3 gap-1">
        {tile(
          <IconRoute size={13} stroke={1.8} />,
          km(total(week)),
          <span className="flex flex-col text-[10px] text-[var(--ink-2)] tabular-nums">
            {grounds(week).map((ground) => (
              <span key={ground.name} className="flex items-center gap-1">
                <Swatch colour={ground.colour} />
                {km(ground.km)}
              </span>
            ))}
          </span>,
        )}
        {tile(<IconStopwatch size={13} stroke={1.8} />, week.time)}
        {tile(<IconMountain size={13} stroke={1.8} />, week.ascent)}
      </div>
    </div>
  );
}

const VARIANTS = [
  { name: "A · Current, tidied", Box: Current },
  { name: "B · Headline", Box: Headline },
  { name: "C · Ledger", Box: Ledger },
  { name: "D · Ink", Box: Ink },
  { name: "E · Tiles", Box: Tiles },
] as const;

const meta = {
  title: "Spikes/Volume Readout",
  parameters: { layout: "fullscreen" },
} satisfies Meta;

export default meta;
type Story = StoryObj<typeof meta>;

/** Each variant over a mixed week and an outdoor-only one. */
export const SideBySide: Story = {
  render: () => (
    <div className="grid min-h-dvh grid-cols-[10rem_repeat(2,max-content)] items-start gap-x-10 gap-y-8 bg-[var(--base)] p-8 text-[var(--ink)]">
      <span />
      <span className="text-[var(--ink-2)] text-xs">Mixed week</span>
      <span className="text-[var(--ink-2)] text-xs">Outdoor only</span>
      {VARIANTS.map(({ name, Box }) => (
        <div key={name} className="contents">
          <span className="font-medium text-sm">{name}</span>
          <Box week={MIXED} />
          <Box week={OUTDOOR} />
        </div>
      ))}
    </div>
  ),
};
