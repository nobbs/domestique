/**
 * Four takes on the sensors card after the sample's list cards: the ledger as
 * shipped, a table with average and max as columns, bars showing the average
 * against the max, and outlined tiles with a quiet mark.
 *
 * Storybook only: nothing here is imported by the application.
 */

import type { Meta, StoryObj } from "@storybook/react-vite";
import { IconActivity, IconBolt, IconGauge, IconHeart, IconRotate } from "@tabler/icons-react";
import type { ReactNode } from "react";
import { StoryProviders } from "../../../storybook/fixtures";

interface Row {
  label: string;
  value: string;
  scale: string;
  note?: string;
}

const SENSORS: Row[] = [
  { label: "Speed", value: "21.3", scale: "km/h average" },
  { label: "Max speed", value: "64.7", scale: "km/h" },
  { label: "Heart rate", value: "123", scale: "bpm average" },
  { label: "Max heart rate", value: "159", scale: "bpm" },
  { label: "Cadence", value: "70", scale: "rpm average" },
  { label: "Max cadence", value: "129", scale: "rpm" },
];
const POWER: Row[] = [
  {
    label: "Estimated power",
    value: "149",
    scale: "watts while pedalling",
    note: "86% of its estimated samples",
  },
  { label: "Threshold power", value: "249", scale: "watts set on the device" },
];
const LOAD: Row[] = [
  { label: "hrTSS", value: "271", scale: "heart rate" },
  { label: "TRIMP", value: "306", scale: "Banister" },
];

/** Average against max, for the three sensors that report both. */
const PAIRS = [
  {
    label: "Speed",
    unit: "km/h",
    average: 21.3,
    max: 64.7,
    icon: IconGauge,
    colour: "var(--series-speed)",
  },
  {
    label: "Heart rate",
    unit: "bpm",
    average: 123,
    max: 159,
    icon: IconHeart,
    colour: "var(--series-heart-rate)",
  },
  {
    label: "Cadence",
    unit: "rpm",
    average: 70,
    max: 129,
    icon: IconRotate,
    colour: "var(--series-cadence)",
  },
];

function Card({ title, children }: { title: string; children: ReactNode }) {
  return (
    <div className="flex w-[26rem] flex-col gap-3 rounded-2xl bg-[var(--panel)] p-4 shadow-[var(--shadow)]">
      <h2 className="font-semibold text-base">{title}</h2>
      {children}
    </div>
  );
}

function Heading({ children }: { children: ReactNode }) {
  return (
    <h3 className="pt-1 font-semibold text-[10px] text-[var(--ink-2)] uppercase tracking-[0.08em]">
      {children}
    </h3>
  );
}

/* ------------------------------------------------------------------------ */

function LedgerRow({ label, value, scale, note }: Row) {
  return (
    <div className="flex items-baseline justify-between gap-4 border-[var(--rule)] border-b py-2 last:border-b-0">
      <span className="text-[var(--ink-2)] text-sm">{label}</span>
      <span className="text-right">
        <span className="font-semibold text-base tabular-nums">{value}</span>{" "}
        <span className="text-[var(--ink-2)] text-xs">{scale}</span>
        {note ? <span className="text-[var(--ink-2)] text-xs opacity-70"> · {note}</span> : null}
      </span>
    </div>
  );
}

/** As shipped: one row per figure, name left, figure and scale right. */
function Ledger() {
  return (
    <Card title="Sensors">
      <div>
        {SENSORS.map((row) => (
          <LedgerRow key={row.label} {...row} />
        ))}
      </div>
      <Heading>Power</Heading>
      <div>
        {POWER.map((row) => (
          <LedgerRow key={row.label} {...row} />
        ))}
      </div>
      <Heading>Load</Heading>
      <div>
        {LOAD.map((row) => (
          <LedgerRow key={row.label} {...row} />
        ))}
      </div>
    </Card>
  );
}

/* ------------------------------------------------------------------------ */

/** A · Columns: the payer table — one row per sensor, average and max as columns. */
function Columns() {
  return (
    <Card title="Sensors">
      <div className="grid grid-cols-[1fr_auto_auto] gap-x-6">
        <div className="text-[var(--ink-2)] text-xs">Sensor</div>
        <div className="text-right text-[var(--ink-2)] text-xs">Average</div>
        <div className="text-right text-[var(--ink-2)] text-xs">Max</div>
        {PAIRS.map((pair) => (
          <div
            key={pair.label}
            className="col-span-3 grid grid-cols-subgrid items-baseline border-[var(--rule)] border-t py-2"
          >
            <span className="text-sm">{pair.label}</span>
            <span className="text-right font-semibold tabular-nums">
              {pair.average}
              <span className="ml-1 font-normal text-[var(--ink-2)] text-xs">{pair.unit}</span>
            </span>
            <span className="text-right text-[var(--ink-2)] tabular-nums">{pair.max}</span>
          </div>
        ))}
      </div>
      <Heading>Power</Heading>
      <div>
        {POWER.map((row) => (
          <LedgerRow key={row.label} {...row} />
        ))}
      </div>
      <Heading>Load</Heading>
      <div>
        {LOAD.map((row) => (
          <LedgerRow key={row.label} {...row} />
        ))}
      </div>
    </Card>
  );
}

/* ------------------------------------------------------------------------ */

/** B · Bars: "why people join" — the average as a bar against the max. */
function Bars() {
  return (
    <Card title="Sensors">
      <div className="flex flex-col gap-3">
        {PAIRS.map((pair) => (
          <div key={pair.label} className="grid gap-1">
            <div className="flex items-baseline justify-between text-sm">
              <span className="text-[var(--ink-2)]">{pair.label}</span>
              <span className="font-semibold tabular-nums">
                {pair.average}
                <span className="ml-1 font-normal text-[var(--ink-2)] text-xs">
                  {pair.unit} · max {pair.max}
                </span>
              </span>
            </div>
            <div className="h-2 overflow-hidden rounded-full bg-[var(--muted)]">
              <div
                className="h-full rounded-full bg-[var(--accent)]"
                style={{ width: `${(pair.average / pair.max) * 100}%` }}
              />
            </div>
          </div>
        ))}
      </div>
      <Heading>Power</Heading>
      <div>
        {POWER.map((row) => (
          <LedgerRow key={row.label} {...row} />
        ))}
      </div>
      <Heading>Load</Heading>
      <div>
        {LOAD.map((row) => (
          <LedgerRow key={row.label} {...row} />
        ))}
      </div>
    </Card>
  );
}

/* ------------------------------------------------------------------------ */

function Tile({
  icon,
  label,
  value,
  unit,
  scale,
  max,
  colour,
}: {
  icon: ReactNode;
  label: string;
  value: string;
  unit?: string;
  scale?: string;
  max?: string;
  /** The series' own colour, as the chart draws it; the tile is tinted with it. */
  colour: string;
}) {
  return (
    <div
      className="grid gap-0.5 rounded-xl border p-3"
      style={{
        borderColor: `color-mix(in oklab, ${colour} 25%, transparent)`,
        background: `color-mix(in oklab, ${colour} 7%, transparent)`,
      }}
    >
      <span className="flex items-center gap-1.5 text-[var(--ink-2)] text-xs">
        <span style={{ color: colour }}>{icon}</span>
        {label}
      </span>
      <span className="font-semibold text-xl leading-tight tabular-nums" style={{ color: colour }}>
        {value}
        {unit ? <span className="ml-1 font-normal text-[var(--ink-2)] text-sm">{unit}</span> : null}
      </span>
      {max ? (
        <span className="whitespace-nowrap text-[var(--ink-2)] text-xs tabular-nums">
          max <span className="text-[var(--ink)]">{max}</span>
        </span>
      ) : scale ? (
        <span className="text-[var(--ink-2)] text-xs">{scale}</span>
      ) : null}
    </div>
  );
}

/** C · Tiles: "who owes it" — one outlined tile per sensor, its max folded in beneath the average. */
function Tiles() {
  return (
    <Card title="Sensors">
      <div className="grid grid-cols-3 gap-2">
        {PAIRS.map((pair) => {
          const Mark = pair.icon;
          return (
            <Tile
              key={pair.label}
              icon={<Mark size={14} stroke={1.8} aria-hidden="true" />}
              label={pair.label}
              value={String(pair.average)}
              unit={pair.unit}
              max={String(pair.max)}
              colour={pair.colour}
            />
          );
        })}
      </div>
      <Heading>Power</Heading>
      <div className="grid grid-cols-2 gap-2">
        {POWER.map((row) => (
          <Tile
            key={row.label}
            icon={<IconBolt size={14} stroke={1.8} aria-hidden="true" />}
            label={row.label}
            value={row.value}
            scale={row.scale}
            colour="var(--series-power)"
          />
        ))}
      </div>
      <Heading>Load</Heading>
      <div className="grid grid-cols-2 gap-2">
        {LOAD.map((row) => (
          <Tile
            key={row.label}
            icon={<IconActivity size={14} stroke={1.8} aria-hidden="true" />}
            label={row.label}
            value={row.value}
            scale={row.scale}
            colour="var(--alert)"
          />
        ))}
      </div>
    </Card>
  );
}

/* ------------------------------------------------------------------------ */

const meta = {
  title: "Spikes/Sensors Card",
  parameters: { layout: "fullscreen" },
} satisfies Meta;

export default meta;
type Story = StoryObj<typeof meta>;

const VARIANTS = [
  {
    name: "Today · Ledger",
    note: "One row per figure, name left, figure and scale right.",
    Card: Ledger,
  },
  {
    name: "A · Columns",
    note: "The payer table: one row per sensor, average and max as two columns.",
    Card: Columns,
  },
  {
    name: "B · Bars",
    note: "The average drawn as a bar against the max; power and load stay rows.",
    Card: Bars,
  },
  {
    name: "C · Tiles",
    note: "One tile per sensor, the max folded in under the average.",
    Card: Tiles,
  },
] as const;

export const SideBySide: Story = {
  render: () => (
    <StoryProviders>
      <div className="flex flex-wrap items-start gap-6 bg-[var(--base)] p-6">
        {VARIANTS.map(({ name, note, Card: Variant }) => (
          <div key={name} className="grid gap-2">
            <div>
              <h2 className="font-semibold text-sm">{name}</h2>
              <p className="text-[var(--ink-2)] text-xs">{note}</p>
            </div>
            <Variant />
          </div>
        ))}
      </div>
    </StoryProviders>
  ),
};
