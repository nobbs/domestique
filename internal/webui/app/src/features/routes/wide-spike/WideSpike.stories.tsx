/**
 * The wide panel, on the whole page, with a route open.
 *
 * The panel does not exist yet and this is what it is for: the route panel
 * shrank to a pill on the promise that the elevation profile, the forecast and
 * the climbs would land somewhere with room. Four ideas about where.
 *
 * Shown as a full page rather than as four cards, because the question is not
 * which sheet is nicest — it is how much map is left once one of them is
 * standing on it. The pill is up in the corner in every frame for the same
 * reason: the two panels are competing for one screen, and a sheet judged on
 * its own has been let off the only constraint that matters.
 */

import type { Meta, StoryObj } from "@storybook/react-vite";
import type { ComponentType, ReactNode } from "react";
import { useState } from "react";
import { MenuBar } from "../../../components/MenuBar";
import { buildCells } from "../../../components/route/forecast-spike/cells";
import type { Highlight } from "../../../lib/highlight";
import type { UnitSystem } from "../../../lib/units";
import { StoryProviders } from "../../../storybook/fixtures";
import {
  spikeBands,
  spikeClimbs,
  spikeCoordinates,
  spikeHighestMetres,
  spikeProfile,
  spikeRoute,
  spikeRuns,
  spikeSamples,
  spikeStartAt,
  spikeSubtitle,
  spikeSurface,
  spikeWeather,
} from "../panel-spike/fixture";
import { LengthsCard } from "../panel-spike/LengthsCard";
import { MapPane } from "../panel-spike/MapPane";
import { SpikePanel } from "../panel-spike/SpikePanel";
import { SelectorSheet } from "./SelectorSheet";
import { SplitSheet } from "./SplitSheet";
import { StackSheet } from "./StackSheet";
import type { SheetProps } from "./shared";
import { TimelineSheet } from "./TimelineSheet";

/** About a laptop, which is the smallest screen this panel is meant for. */
const PAGE = { width: 1440, height: 900 };

const cells = buildCells(spikeSamples, spikeWeather, spikeCoordinates);

function Page({
  Sheet,
  unitSystem = "metric",
}: {
  Sheet: ComponentType<SheetProps>;
  unitSystem?: UnitSystem;
}) {
  const [collapsed, setCollapsed] = useState(true);
  const [highlight, setHighlight] = useState<Highlight | null>(null);
  const [activeMetres, setActiveMetres] = useState<number | null>(null);

  return (
    <div
      className="flex flex-col overflow-hidden rounded-lg bg-[var(--ground)] text-[var(--ink)] ring-1 ring-[var(--rule)]"
      style={{ width: PAGE.width, height: PAGE.height }}
    >
      <MenuBar />
      <MapPane
        coordinates={spikeCoordinates}
        width={PAGE.width}
        height={PAGE.height - 56}
        sheet={
          <Sheet
            route={spikeRoute}
            profile={spikeProfile}
            surface={spikeSurface}
            runs={spikeRuns}
            bands={spikeBands}
            climbs={spikeClimbs}
            cells={cells}
            samples={spikeSamples}
            startAt={spikeStartAt}
            activeMetres={activeMetres}
            onActiveChange={setActiveMetres}
            highlight={highlight}
            onHighlightChange={setHighlight}
            unitSystem={unitSystem}
          />
        }
      >
        {/*
         * Resting, which is how it will usually be found once there is a sheet:
         * the reader who opened the wide panel is reading the wide panel.
         */}
        <SpikePanel
          Card={LengthsCard}
          collapsed={collapsed}
          onCollapsedChange={setCollapsed}
          route={spikeRoute}
          subtitle={spikeSubtitle}
          movingSeconds={spikeRoute.movingSeconds}
          highestMetres={spikeHighestMetres}
          bands={spikeBands}
          runs={spikeRuns}
          surface={spikeSurface}
          surfaceAbsence="Surface not classified yet."
          climbs={spikeClimbs}
          highlight={highlight}
          onHighlightChange={setHighlight}
          unitSystem={unitSystem}
          onClose={() => {}}
          sourceBaseUrls={{ veloplanner: "https://veloplanner.com" }}
          libraryCount={47}
        />
      </MapPane>
    </div>
  );
}

function Framed({ name, note, children }: { name: string; note: string; children: ReactNode }) {
  return (
    <figure className="grid gap-1">
      <figcaption className="text-sm">
        <span className="font-semibold">{name}</span>{" "}
        <span className="text-[var(--ink-2)]">— {note}</span>
      </figcaption>
      {children}
    </figure>
  );
}

const SHEETS = [
  {
    key: "selector",
    name: "1 — Selector",
    note: "Komoot's model: one chart, and a control that says which",
    Sheet: SelectorSheet,
  },
  {
    key: "stack",
    name: "2 — Stack",
    note: "profile, ground and forecast on one distance axis",
    Sheet: StackSheet,
  },
  {
    key: "split",
    name: "3 — Split",
    note: "the chart, beside the ride as a list of things that happen",
    Sheet: SplitSheet,
  },
  {
    key: "timeline",
    name: "4 — Timeline",
    note: "the same ride against the clock rather than the tape",
    Sheet: TimelineSheet,
  },
] as const;

function one(key: (typeof SHEETS)[number]["key"]) {
  const entry = SHEETS.find((candidate) => candidate.key === key) ?? SHEETS[0];

  return () => (
    <StoryProviders>
      <div className="p-6">
        <Framed name={entry.name} note={entry.note}>
          <Page Sheet={entry.Sheet} />
        </Framed>
      </div>
    </StoryProviders>
  );
}

const meta = {
  title: "Spikes/Wide Panel",
  parameters: { layout: "fullscreen" },
} satisfies Meta;

export default meta;
type Story = StoryObj<typeof meta>;

export const SelectorOne: Story = { render: one("selector") };
export const StackTwo: Story = { render: one("stack") };
export const SplitThree: Story = { render: one("split") };
export const TimelineFour: Story = { render: one("timeline") };

/** All four, for the judgement that needs them next to each other. */
export const Alternatives: Story = {
  render: () => (
    <StoryProviders>
      <div className="grid gap-8 p-6">
        {SHEETS.map((entry) => (
          <Framed key={entry.key} name={entry.name} note={entry.note}>
            <Page Sheet={entry.Sheet} />
          </Framed>
        ))}
      </div>
    </StoryProviders>
  ),
};
