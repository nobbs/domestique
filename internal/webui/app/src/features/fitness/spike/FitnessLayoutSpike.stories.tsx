/**
 * Three wide layouts for the fitness page after the sample's billing dashboard:
 * a strip over a main column and rail, the headline figures moved into the rail,
 * and a strip over a full-width chart with the evidence in three columns.
 *
 * Storybook only. The charts are the real ones over a synthetic season.
 */

import type { Meta, StoryObj } from "@storybook/react-vite";
import {
  IconActivityHeartbeat,
  IconBolt,
  IconCalendarWeek,
  IconChartBar,
  IconHeartRateMonitor,
  IconTrendingUp,
} from "@tabler/icons-react";
import type { ReactNode } from "react";
import type { FitnessDay, FitnessWeek, PowerCurvePoint } from "../../../api/types";
import { PanelHeading } from "../../../components/PanelHeading";
import { StoryProviders } from "../../../storybook/fixtures";
import { DecouplingPanel, type DecouplingRide } from "../DecouplingPanel";
import { reading } from "../form";
import { PowerDuration } from "../PowerDuration";
import { SeasonChart } from "../SeasonChart";
import { ZonePanel } from "../ZonePanel";

const DAYS = 180;
const START = Date.UTC(2026, 2, 21);
const iso = (index: number) => new Date(START + index * 86_400_000).toISOString().slice(0, 10);

const season: FitnessDay[] = (() => {
  let fitness = 38;
  let fatigue = 40;
  return Array.from({ length: DAYS }, (_, index) => {
    const weekday = index % 7;
    const block = Math.floor(index / 28) % 4 === 3 ? 0.55 : 1;
    const load = [0, 70, 55, 95, 0, 160, 120][weekday]! * block * (1 + 0.25 * Math.sin(index / 9));
    fitness += (load - fitness) / 42;
    fatigue += (load - fatigue) / 7;
    const trimp = load * 0.6;
    return {
      date: iso(index),
      tssLoad: load,
      tssFitness: fitness,
      tssFatigue: fatigue,
      tssForm: fitness - fatigue,
      trimpLoad: trimp,
      trimpFitness: fitness * 0.6,
      trimpFatigue: fatigue * 0.6,
      trimpForm: (fitness - fatigue) * 0.6,
    };
  });
})();

const readings = season.map((day) => reading(day, "tss"));
const dates = readings.map((one) => one.date);

const weeks: FitnessWeek[] = Array.from({ length: Math.ceil(DAYS / 7) }, (_, index) => ({
  weekStart: iso(index * 7),
  zoneSeconds: [1800, 9000 + 1200 * (index % 4), 2400, 900 + 300 * (index % 3), 240].map(
    (seconds) => seconds * (Math.floor((index * 7) / 28) % 4 === 3 ? 0.5 : 1),
  ),
}));

const curve = (scale: number): PowerCurvePoint[] =>
  [
    [5, 910],
    [30, 540],
    [60, 410],
    [300, 305],
    [1200, 262],
    [3600, 231],
  ].map(([seconds, watts]) => ({ seconds: seconds!, watts: Math.round(watts! * scale) }));

const rides: DecouplingRide[] = Array.from({ length: 40 }, (_, index) => ({
  id: `ride-${index}`,
  date: iso(index * 4 + 2),
  percent: 7.5 - index * 0.12 + Math.sin(index) * 1.4,
  movingSeconds: 7200 + (index % 3) * 1800,
}));

const FIGURES = [
  {
    icon: <IconTrendingUp size={18} stroke={1.8} />,
    label: "Fitness",
    value: "68",
    chip: "+30",
    tone: "var(--good)",
    note: "since 21 Mar · peak 71",
  },
  {
    icon: <IconActivityHeartbeat size={18} stroke={1.8} />,
    label: "Ramp rate",
    value: "+2.4",
    unit: "a week",
    chip: "Building",
    tone: "var(--good)",
    note: "+4% of fitness",
  },
  {
    icon: <IconCalendarWeek size={18} stroke={1.8} />,
    label: "Next 7 days",
    value: "480–560",
    unit: "TSS",
    chip: "to build",
    tone: "var(--accent)",
    note: "last 4 weeks averaged 470",
  },
  {
    icon: <IconHeartRateMonitor size={18} stroke={1.8} />,
    label: "Form",
    value: "−12%",
    unit: "of fitness",
    chip: "Optimal",
    tone: "var(--accent)",
    note: "fresh after 4 days of rest",
  },
] as const;

type Figure = (typeof FIGURES)[number];

const tint = (tone: string) => `color-mix(in oklab, ${tone} 14%, transparent)`;

function Chip({ tone, children }: { tone: string; children: ReactNode }) {
  return (
    <span
      className="whitespace-nowrap rounded-md px-1.5 py-0.5 font-medium text-xs"
      style={{ background: tint(tone), color: tone }}
    >
      {children}
    </span>
  );
}

function Mark({ children }: { children: ReactNode }) {
  return (
    <span
      aria-hidden="true"
      className="grid size-9 shrink-0 place-items-center rounded-md bg-[radial-gradient(circle_at_50%_35%,#6e6e6e,#3d3d3d_85%)] text-[var(--panel)]"
    >
      {children}
    </span>
  );
}

function Card({ children, className = "" }: { children: ReactNode; className?: string }) {
  return (
    <section
      className={`flex min-w-0 flex-col gap-4 rounded-2xl bg-[var(--panel)] p-5 shadow-[var(--shadow)] ${className}`}
    >
      {children}
    </section>
  );
}

function Strip() {
  return (
    <Card className="grid gap-0 py-4 sm:grid-cols-2 lg:grid-cols-4 lg:divide-x lg:divide-[var(--rule)]">
      {FIGURES.map((figure) => (
        <div key={figure.label} className="flex items-center gap-3 px-4 py-1 first:pl-0 last:pr-0">
          <Mark>{figure.icon}</Mark>
          <div className="flex min-w-0 flex-1 flex-col">
            <span className="text-[var(--ink-2)] text-xs">{figure.label}</span>
            <span className="flex items-baseline justify-between gap-2">
              <span className="font-semibold text-xl tabular-nums tracking-tight">
                {figure.value}
                {"unit" in figure ? (
                  <span className="ml-1 font-normal text-[var(--ink-2)] text-xs">
                    {figure.unit}
                  </span>
                ) : null}
              </span>
              <Chip tone={figure.tone}>{figure.chip}</Chip>
            </span>
            <span className="truncate text-[var(--ink-2)] text-xs">{figure.note}</span>
          </div>
        </div>
      ))}
    </Card>
  );
}

function RailFigure({ figure }: { figure: Figure }) {
  return (
    <Card className="gap-2">
      <span className="flex items-center justify-between text-[var(--ink-2)] text-sm">
        {figure.label}
        <Chip tone={figure.tone}>{figure.chip}</Chip>
      </span>
      <span className="font-semibold text-3xl tabular-nums tracking-tight">
        {figure.value}
        {"unit" in figure ? (
          <span className="ml-1.5 font-normal text-[var(--ink-2)] text-sm">{figure.unit}</span>
        ) : null}
      </span>
      <span className="text-[var(--ink-2)] text-xs">{figure.note}</span>
    </Card>
  );
}

const Season = () => (
  <Card>
    <PanelHeading
      icon={<IconTrendingUp size={18} stroke={1.8} />}
      title="Fitness, form and load"
      subtitle="six months, three projected weeks"
    />
    <SeasonChart readings={readings} outlook={undefined} scaleName="stress score" />
  </Card>
);

const Power = () => (
  <Card>
    <PanelHeading
      icon={<IconBolt size={18} stroke={1.8} />}
      title="Power duration"
      subtitle="against the 6 months before"
    />
    <PowerDuration current={curve(1)} previous={curve(0.94)} previousName="the 6 months before" />
  </Card>
);

const Decoupling = () => (
  <Card>
    <PanelHeading
      icon={<IconHeartRateMonitor size={18} stroke={1.8} />}
      title="Decoupling"
      aside={<Chip tone="var(--good)">3.1% median</Chip>}
    />
    <DecouplingPanel rides={rides} dates={dates} />
  </Card>
);

const Zones = () => (
  <Card>
    <PanelHeading icon={<IconChartBar size={18} stroke={1.8} />} title="Time in zone" />
    <ZonePanel weeks={weeks} dates={dates} />
  </Card>
);

function Page({ children }: { children: ReactNode }) {
  return (
    <StoryProviders>
      <div className="min-h-dvh bg-[var(--base)] px-6 py-8 text-[var(--ink)]">
        <div className="mx-auto flex w-full max-w-[1400px] flex-col gap-5">
          <header>
            <h1 className="font-semibold text-2xl tracking-tight">Fitness</h1>
            <p className="text-[var(--ink-2)] text-sm">As of 16 Sept, on the stress score scale</p>
          </header>
          {children}
        </div>
      </div>
    </StoryProviders>
  );
}

const meta = {
  title: "Spikes/Fitness Layout",
  parameters: { layout: "fullscreen" },
} satisfies Meta;

export default meta;
type Story = StoryObj<typeof meta>;

/** A · Strip and rail: figures in one strip; charts in the main column, the evidence in a 22rem rail. */
export const StripAndRail: Story = {
  render: () => (
    <Page>
      <Strip />
      <div className="grid gap-5 lg:grid-cols-[minmax(0,1fr)_22rem]">
        <div className="flex min-w-0 flex-col gap-5">
          <Season />
          <Power />
        </div>
        <div className="flex min-w-0 flex-col gap-5">
          <Decoupling />
          <Zones />
        </div>
      </div>
    </Page>
  ),
};

/** B · Figures in the rail: each figure its own rail card beside the chart; evidence in two columns below. */
export const FiguresInRail: Story = {
  render: () => (
    <Page>
      <div className="grid gap-5 lg:grid-cols-[minmax(0,1fr)_20rem]">
        <div className="flex min-w-0 flex-col gap-5">
          <Season />
          <Power />
        </div>
        <div className="flex min-w-0 flex-col gap-5">
          {FIGURES.map((figure) => (
            <RailFigure key={figure.label} figure={figure} />
          ))}
        </div>
      </div>
      <div className="grid gap-5 md:grid-cols-2">
        <Decoupling />
        <Zones />
      </div>
    </Page>
  ),
};

/** C · Strip and three columns: the season chart takes the full width; the evidence shares one row. */
export const StripAndColumns: Story = {
  render: () => (
    <Page>
      <Strip />
      <Season />
      <div className="grid gap-5 lg:grid-cols-3">
        <Power />
        <Decoupling />
        <Zones />
      </div>
    </Page>
  ),
};
