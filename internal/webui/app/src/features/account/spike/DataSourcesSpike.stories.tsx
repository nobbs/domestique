/**
 * Four treatments for the Data sources card. Storybook only, with the credits
 * the snapshot shows written in, so nothing is fetched.
 */

import type { Meta, StoryObj } from "@storybook/react-vite";
import { IconBook, IconCloudRain, IconMap, IconRoad } from "@tabler/icons-react";
import { Panel } from "@/components/PanelHeading";

const SOURCES = [
  {
    label: "Map",
    use: "Draws the basemap",
    icon: IconMap,
    credits: [
      "OpenFreeMap",
      "© OpenMapTiles Data from OpenStreetMap",
      "© MapTiler",
      "© OpenStreetMap contributors",
    ],
  },
  {
    label: "Surface",
    use: "Classifies what each stretch is paved with",
    icon: IconRoad,
    credits: ["© OpenStreetMap contributors", "ODbL"],
  },
  {
    label: "Weather",
    use: "Forecasts wind and rain along a ride",
    icon: IconCloudRain,
    credits: ["Open-Meteo.com"],
  },
] as const;

const WASH = "bg-[color-mix(in_oklab,var(--ink-2)_7%,transparent)]";

function Card({ children }: { children: React.ReactNode }) {
  return (
    <div className="max-w-3xl p-6">
      <Panel
        icon={<IconBook size={18} stroke={1.8} />}
        title="Data sources"
        subtitle="what this service draws, classifies and forecasts with"
      >
        {children}
      </Panel>
    </div>
  );
}

/** A · Today: a two-column ledger. */
function Today() {
  return (
    <dl className="flex flex-col divide-y divide-[var(--rule)] text-sm">
      {SOURCES.map((s) => (
        <div key={s.label} className="grid grid-cols-[6rem_1fr] gap-4 py-2 first:pt-0 last:pb-0">
          <dt className="text-[var(--ink-2)]">{s.label}</dt>
          <dd>{s.credits.join(" · ")}</dd>
        </div>
      ))}
    </dl>
  );
}

/** B · Inset rows: the Sync rows' shape, a glyph mark, what it is used for, credits muted beneath. */
function Inset() {
  return (
    <ul className={`flex flex-col overflow-hidden rounded-xl ${WASH}`}>
      {SOURCES.map((s) => (
        <li
          key={s.label}
          className="flex items-center gap-3 border-[var(--panel)] border-b-2 px-3.5 py-3 text-sm last:border-b-0"
        >
          <span className="grid size-8 shrink-0 place-items-center rounded-[9px] bg-[var(--panel)] text-[var(--ink)]">
            <s.icon size={16} stroke={1.8} aria-hidden="true" />
          </span>
          <div className="flex min-w-0 flex-col">
            <span className="font-semibold">
              {s.label}
              <span className="ml-1.5 font-normal text-[var(--ink-2)]">{s.use}</span>
            </span>
            <span className="text-[var(--ink-2)] text-xs">{s.credits.join(" · ")}</span>
          </div>
        </li>
      ))}
    </ul>
  );
}

/** C · Tiles: one tile per source side by side, credits as a stacked list. */
function Tiles() {
  return (
    <div className="grid gap-3 sm:grid-cols-3">
      {SOURCES.map((s) => (
        <div key={s.label} className={`flex flex-col gap-3 rounded-xl p-4 ${WASH}`}>
          <div className="flex items-center gap-2.5">
            <span className="grid size-8 place-items-center rounded-[9px] bg-[var(--panel)]">
              <s.icon size={16} stroke={1.8} aria-hidden="true" />
            </span>
            <span className="font-semibold text-sm">{s.label}</span>
          </div>
          <p className="text-[var(--ink-2)] text-xs">{s.use}</p>
          <ul className="mt-auto flex flex-col gap-1 text-xs">
            {s.credits.map((c) => (
              <li key={c}>{c}</li>
            ))}
          </ul>
        </div>
      ))}
    </div>
  );
}

/** D · Chips: label and use on the left, each credit its own pill on the right. */
function Chips() {
  return (
    <div className="flex flex-col divide-y divide-[var(--rule)]">
      {SOURCES.map((s) => (
        <div
          key={s.label}
          className="grid gap-3 py-3.5 first:pt-0 last:pb-0 sm:grid-cols-[14rem_1fr] sm:items-center"
        >
          <div className="flex items-center gap-3">
            <s.icon
              size={20}
              stroke={1.6}
              className="shrink-0 text-[var(--ink-2)]"
              aria-hidden="true"
            />
            <div className="flex flex-col">
              <span className="font-semibold text-sm">{s.label}</span>
              <span className="text-[var(--ink-2)] text-xs">{s.use}</span>
            </div>
          </div>
          <div className="flex flex-wrap gap-1.5">
            {s.credits.map((c) => (
              <span key={c} className={`rounded-full px-2.5 py-1 text-xs ${WASH}`}>
                {c}
              </span>
            ))}
          </div>
        </div>
      ))}
    </div>
  );
}

const VARIANTS = { today: Today, inset: Inset, tiles: Tiles, chips: Chips } as const;

function Spike({ variant }: { variant: keyof typeof VARIANTS }) {
  const Body = VARIANTS[variant];
  return (
    <Card>
      <Body />
    </Card>
  );
}

const meta = {
  title: "Spikes/Data Sources",
  component: Spike,
  parameters: { layout: "fullscreen" },
} satisfies Meta<typeof Spike>;

export default meta;
type Story = StoryObj<typeof meta>;

export const A_Today: Story = { args: { variant: "today" } };
export const B_Inset: Story = { args: { variant: "inset" } };
export const C_Tiles: Story = { args: { variant: "tiles" } };
export const D_Chips: Story = { args: { variant: "chips" } };
