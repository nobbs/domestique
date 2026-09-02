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
import { expect, userEvent, waitFor } from "storybook/test";
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
import { StackClimbsSheet } from "./StackClimbsSheet";
import { StackSheet } from "./StackSheet";
import type { SheetProps } from "./shared";
import { TimelineSheet } from "./TimelineSheet";
import { WEATHER_FRAMES } from "./WeatherFrame";

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
  // Expanded, which is the contested case: both panels open at once is when
  // the two are actually competing for the screen, and a sheet judged against
  // a resting pill has been shown the easy half of the question.
  const [collapsed, setCollapsed] = useState(false);
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
    key: "stack-climbs",
    name: "5 — Stack, with what happens",
    note: "the axis from 2, the climbs-and-weather join from 3, as a lane",
    Sheet: StackClimbsSheet,
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
export const StackClimbsFive: Story = { render: one("stack-climbs") };
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

/**
 * Four ways of framing the forecast, without the map.
 *
 * The map is what the other stories are for. This one asks whether the band
 * reads as a prediction rather than as another lane of measured terrain, and
 * that is a question about the inside of the sheet — putting four pages up to
 * ask it would spend four screens on furniture nobody is looking at.
 */
export const WeatherFrames: Story = {
  render: () => (
    <StoryProviders>
      <div className="grid gap-6 bg-[var(--ground)] p-6">
        {WEATHER_FRAMES.map((frame) => (
          <figure key={frame.variant} className="grid gap-1" style={{ width: PAGE.width - 48 }}>
            <figcaption className="text-sm">
              <span className="font-semibold">{frame.variant}</span>{" "}
              <span className="text-[var(--ink-2)]">— {frame.note}</span>
            </figcaption>
            <StackClimbsSheet
              weatherFrame={frame.variant}
              route={spikeRoute}
              profile={spikeProfile}
              surface={spikeSurface}
              runs={spikeRuns}
              bands={spikeBands}
              climbs={spikeClimbs}
              cells={cells}
              samples={spikeSamples}
              startAt={spikeStartAt}
              activeMetres={null}
              onActiveChange={() => {}}
              highlight={null}
              onHighlightChange={() => {}}
              unitSystem="metric"
            />
          </figure>
        ))}
      </div>
    </StoryProviders>
  ),
};

/**
 * Folding the climbs column has to widen the chart and the forecast with it.
 *
 * Both measure themselves with a `ResizeObserver`, and both are inside the
 * column that grows — so this is exactly the kind of thing that looks fine and
 * silently is not. It cannot be checked by eye in a preview pane either: the
 * story renders in a hidden document there, where the rendering lifecycle is
 * suspended and neither `ResizeObserver` nor `requestAnimationFrame` is
 * delivered, so the lanes visibly widen and the two measured lanes do not
 * follow. This runs in a real browser, where they do.
 */
export const RescalesWhenFolded: Story = {
  render: one("stack-climbs"),
  play: async ({ canvas }) => {
    const chart = canvas.getByRole("img", { name: /Trois Cols/ });
    const before = Number(chart.getAttribute("width"));
    expect(before).toBeGreaterThan(0);

    await userEvent.click(canvas.getByRole("button", { name: "Hide what happens" }));

    await waitFor(() => {
      expect(Number(chart.getAttribute("width"))).toBeGreaterThan(before);
    });
  },
};
