/**
 * The ride page's hero as the figure strip Fitness and Volume lead with.
 *
 * Storybook only. Every variant reads the real headline figures of one synthetic ride.
 */

import type { Meta, StoryObj } from "@storybook/react-vite";
import {
  IconActivityHeartbeat,
  IconArrowBarToDown,
  IconClock,
  IconFlame,
  IconMountain,
  IconRoute,
} from "@tabler/icons-react";
import type { ReactNode } from "react";
import type { Activity, ActivityRouteMatch } from "../../../api/types";
import { PanelMark } from "../../../components/PanelHeading";
import { StoryProviders } from "../../../storybook/fixtures";
import { colour, type Headline, RideFigures, rideHeadlines } from "../RideFigures";
import { ASCENT_METRES, ELAPSED_SECONDS, METRICS, MOVING_SECONDS, TOTAL_METRES } from "./data";

const RIDE: Activity = {
  id: "42",
  startedAt: "2026-09-12T09:04:00Z",
  distanceMetres: TOTAL_METRES,
  movingSeconds: MOVING_SECONDS,
  elapsedSeconds: ELAPSED_SECONDS + 600,
  ascentMetres: ASCENT_METRES,
  descentMetres: ASCENT_METRES - 12,
  caloriesKcal: 3106,
  typeId: 0,
  locationId: 0,
  indoor: false,
  provider: "wahoo",
  metrics: METRICS,
  routeMatch: {
    provider: "veloplanner",
    sourceRouteId: 1,
    stageOrder: 0,
    routeCoverage: 0.97,
    rideCoverage: 1,
    direction: "forward",
  } as ActivityRouteMatch,
};

const FIGURES = rideHeadlines(RIDE);

const MARKS: Record<string, ReactNode> = {
  Distance: <IconRoute size={18} stroke={1.8} />,
  Moving: <IconClock size={18} stroke={1.8} />,
  Climbed: <IconMountain size={18} stroke={1.8} />,
  Descended: <IconArrowBarToDown size={18} stroke={1.8} />,
  Calories: <IconFlame size={18} stroke={1.8} />,
};
const mark = (figure: Headline) =>
  MARKS[figure.label] ?? <IconActivityHeartbeat size={18} stroke={1.8} />;

const tint = (tone: Headline["tone"]) =>
  `color-mix(in oklab, ${tone === "quiet" ? "var(--ink-2)" : colour(tone)} 14%, transparent)`;

function Header() {
  return (
    <div>
      <h1 className="font-semibold text-lg leading-tight">Villa Rustica 100km</h1>
      <p className="text-[var(--ink-2)] text-sm">12 Sept 2026, 11:04 · 20–26°, wind 8.5 km/h</p>
    </div>
  );
}

function Value({ figure }: { figure: Headline }) {
  return (
    <>
      {figure.value}
      {figure.unit ? (
        <span className="ml-1 font-normal text-[var(--ink-2)] text-xs">{figure.unit}</span>
      ) : null}
    </>
  );
}

function Chip({ figure }: { figure: Headline }) {
  return figure.verdict ? (
    <span
      className="whitespace-nowrap rounded-md px-1.5 py-0.5 font-medium text-xs"
      style={{ background: tint(figure.verdict.tone), color: colour(figure.verdict.tone) }}
    >
      {figure.verdict.label}
    </span>
  ) : null;
}

/** A · Today's hero: tinted figure pills, verdict and reading beneath. */
function Today() {
  return (
    <div className="grid gap-4 rounded-2xl bg-[var(--panel)] p-5 shadow-[var(--shadow)]">
      <Header />
      <RideFigures ride={RIDE} />
    </div>
  );
}

/** One strip cell: mark, label, value with its chip, the reading under it. */
function Cell({ figure, tinted }: { figure: Headline; tinted?: boolean }) {
  return (
    <div className="flex min-w-0 items-center gap-3 lg:px-4 lg:first:pl-0 lg:last:pr-0">
      <PanelMark>{mark(figure)}</PanelMark>
      <div className="flex min-w-0 flex-1 flex-col">
        <span className="text-[var(--ink-2)] text-xs">{figure.label}</span>
        <span className="flex flex-wrap items-baseline justify-between gap-x-2 gap-y-0.5">
          <span
            className="whitespace-nowrap font-semibold text-xl tabular-nums tracking-tight"
            style={tinted ? { color: colour(figure.tone) } : undefined}
          >
            <Value figure={figure} />
          </span>
          {tinted ? null : <Chip figure={figure} />}
        </span>
        <span className="truncate text-[var(--ink-2)] text-xs">
          {tinted && figure.verdict ? (
            <span className="font-medium" style={{ color: colour(figure.verdict.tone) }}>
              {figure.verdict.label}
              {figure.note ? " · " : ""}
            </span>
          ) : null}
          {figure.note ?? " "}
        </span>
      </div>
    </div>
  );
}

/** B · Strip: the header above one six-cell strip, verdicts as chips beside the value. */
function Strip() {
  return (
    <div className="flex flex-col gap-4 rounded-2xl bg-[var(--panel)] p-5 shadow-[var(--shadow)]">
      <Header />
      <div className="grid grid-cols-2 gap-4 sm:grid-cols-3 lg:grid-cols-6 lg:gap-0 lg:divide-x lg:divide-[var(--rule)]">
        {FIGURES.map((figure) => (
          <Cell key={figure.label} figure={figure} />
        ))}
      </div>
    </div>
  );
}

/** C · Tinted strip: no chips; the value wears its tone, the verdict leads the reading. */
function TintedStrip() {
  return (
    <div className="flex flex-col gap-4 rounded-2xl bg-[var(--panel)] p-5 shadow-[var(--shadow)]">
      <Header />
      <div className="grid grid-cols-2 gap-4 sm:grid-cols-3 lg:grid-cols-6 lg:gap-0 lg:divide-x lg:divide-[var(--rule)]">
        {FIGURES.map((figure) => (
          <Cell key={figure.label} figure={figure} tinted />
        ))}
      </div>
    </div>
  );
}

/** D · Title beside a three-by-two strip, as the page's own figure strip sits under the page title. */
function TwoRows() {
  return (
    <div className="grid gap-5 rounded-2xl bg-[var(--panel)] p-5 shadow-[var(--shadow)] lg:grid-cols-[16rem_1fr]">
      <div className="flex flex-col justify-center gap-2 lg:border-[var(--rule)] lg:border-r lg:pr-5">
        <Header />
      </div>
      <div className="grid grid-cols-2 gap-x-0 gap-y-4 sm:grid-cols-3 [&>*:not(:nth-child(3n+1))]:sm:border-[var(--rule)] [&>*:not(:nth-child(3n+1))]:sm:border-l [&>*]:sm:px-4">
        {FIGURES.map((figure) => (
          <Cell key={figure.label} figure={figure} />
        ))}
      </div>
    </div>
  );
}

const meta = {
  title: "Spikes/Ride Hero Strip",
  parameters: { layout: "fullscreen" },
} satisfies Meta;

export default meta;
type Story = StoryObj<typeof meta>;

export const AllVariants: Story = {
  render: () => (
    <StoryProviders>
      <div className="flex min-h-dvh flex-col gap-8 bg-[var(--base)] p-8 text-[var(--ink)]">
        {[
          ["A · Today", <Today key="a" />],
          ["B · Strip with chips", <Strip key="b" />],
          ["C · Tinted strip", <TintedStrip key="c" />],
          ["D · Title beside two rows", <TwoRows key="d" />],
        ].map(([name, node]) => (
          <section key={name as string} className="flex max-w-[1400px] flex-col gap-2">
            <span className="text-[var(--ink-2)] text-xs">{name}</span>
            {node}
          </section>
        ))}
      </div>
    </StoryProviders>
  ),
};
