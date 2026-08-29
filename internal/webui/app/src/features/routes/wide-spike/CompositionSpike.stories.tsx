/**
 * Four ways of arranging the two panels, on the whole page.
 *
 * The individual pieces have been argued over enough; what has not is how they
 * sit together. With the wide sheet open the screen currently carries two
 * floating objects in two visual registers — a dense little card in one corner
 * and an airy dock along the foot — and between them steepness and ground
 * appear twice, as the card's upright bars and again as the chart's banding
 * and the sheet's ribbon. That is the same duplication already removed twice
 * inside the sheet, happening a second time between the panels.
 *
 * Each alternative below answers it differently: by dividing the content, by
 * merging the furniture, by unifying only the surface, or by moving the
 * furniture off the map altogether.
 */

import type { Meta, StoryObj } from "@storybook/react-vite";
import type { ReactNode } from "react";
import { useState } from "react";
import { MenuBar } from "../../../components/MenuBar";
import { buildCells } from "../../../components/route/forecast-spike/cells";
import type { Highlight } from "../../../lib/highlight";
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
import { LengthsCard, lengthFigure } from "../panel-spike/LengthsCard";
import { MapPane } from "../panel-spike/MapPane";
import { StackedColumn } from "../panel-spike/SidewaysCard";
import { SpikePanel } from "../panel-spike/SpikePanel";
import { bandEntries, surfaceEntries } from "../panel-spike/shared";
import { ClimbsSection } from "./ClimbsSection";
import { RouteIdentity } from "./RouteIdentity";
import { StackClimbsSheet } from "./StackClimbsSheet";

const PAGE = { width: 1440, height: 900 };
const cells = buildCells(spikeSamples, spikeWeather, spikeCoordinates);

/** Everything the sheet needs, which is the same in every arrangement. */
const sheet = {
  route: spikeRoute,
  profile: spikeProfile,
  surface: spikeSurface,
  runs: spikeRuns,
  bands: spikeBands,
  climbs: spikeClimbs,
  cells,
  samples: spikeSamples,
  startAt: spikeStartAt,
  unitSystem: "metric" as const,
};

const identity = {
  route: spikeRoute,
  subtitle: spikeSubtitle,
  movingSeconds: spikeRoute.movingSeconds,
  highestMetres: spikeHighestMetres,
  climbs: spikeClimbs,
  unitSystem: "metric" as const,
};

function Page({ children }: { children: ReactNode }) {
  return (
    <StoryProviders>
      <div
        className="flex flex-col overflow-hidden rounded-lg bg-[var(--ground)] text-[var(--ink)] ring-1 ring-[var(--rule)]"
        style={{ width: PAGE.width, height: PAGE.height }}
      >
        {children}
      </div>
    </StoryProviders>
  );
}

/** State every arrangement needs, so none of them owns it. */
function useComposition() {
  const [highlight, setHighlight] = useState<Highlight | null>(null);
  const [activeMetres, setActiveMetres] = useState<number | null>(null);

  return {
    highlight,
    onHighlightChange: setHighlight,
    activeMetres,
    onActiveChange: setActiveMetres,
  };
}

/**
 * **1 — Negotiated.** Both panels stay; the content moves between them.
 *
 * The split is by *question* rather than by room. The dock draws everything
 * against the distance axis and so answers *where* — where it is steep, where
 * the gravel is, where the rain arrives. The card answers *how much* and *what
 * of*: thirteen kilometres of gravel, seven climbs, seven and a half hours.
 *
 * Both mixes are therefore in both panels, and that is not the duplication it
 * looks like. `gradientShares` and `gradientMix` exist as separate functions
 * for this exact reason — one totals the route per class, the other keeps the
 * order it is ridden in — and the legend's own note says the two cannot be
 * read for each other's question. Thirteen kilometres of gravel decides the
 * bike; its falling on the second col decides the day.
 *
 * The climbs went to the card on that rule. They had been a column beside the
 * lanes, which filed them with the drawn readings, and a climb is not one: it
 * is a thing about the route, like its distance and its ascent, and the panel
 * naming those is the one already there. It also leaves the dock as three
 * lanes on one axis and nothing else, which is all it ever claimed to be.
 *
 * Folded by default, so the card costs a line and opens to a list — the same
 * bargain `ClimbsList` already makes in the panel this is a sketch for.
 *
 * The rule is a negotiation rather than a fixed layout: with the dock shut the
 * card would take the drawn readings back, since there would be nowhere else
 * for them. That is the part this frame cannot show and the part most worth
 * arguing about.
 */
export const Negotiated: StoryObj = {
  render: () => {
    const shared = useComposition();

    return (
      <Page>
        <MenuBar />
        <MapPane
          coordinates={spikeCoordinates}
          width={PAGE.width}
          height={PAGE.height - 56}
          sheet={
            <StackClimbsSheet {...sheet} {...shared} weatherFrame="capped" withClimbs={false} />
          }
        >
          <div
            data-compact-workspace=""
            // The height bound the real shell already puts on this panel: its
            // `aside` is `max-h-[calc(100%-1.5rem)] overflow-y-auto`. Kept here
            // as the same safety net, so a route with more to say scrolls its
            // card rather than sliding it under the dock.
            className="grid max-h-[calc(100%-0.75rem)] w-[21rem] gap-2 overflow-y-auto rounded-xl bg-[var(--panel)] p-3 shadow-[var(--shadow)] ring-1 ring-black/5"
          >
            <RouteIdentity {...identity} named climbLine={false} columns={3} />
            {/*
             * The mixes as lengths, which is a different question from the one
             * the dock's ribbon answers. `gradientShares` and `gradientMix`
             * were split for exactly this: how much of the ride is gravel, and
             * where the gravel is. Thirteen kilometres of it decides the bike;
             * its falling on the second col decides the day.
             */}
            <div className="flex items-start gap-3 border-t border-[var(--rule)] pt-2">
              <StackedColumn
                name="Gradient"
                entries={bandEntries(spikeBands, spikeRoute.distanceMetres)}
                absence="No elevation data."
                figure={lengthFigure}
                highlight={shared.highlight}
                onHighlightChange={shared.onHighlightChange}
                unitSystem="metric"
              />
              <StackedColumn
                name="Surface"
                entries={surfaceEntries(spikeSurface)}
                absence="Surface not classified yet."
                figure={lengthFigure}
                highlight={shared.highlight}
                onHighlightChange={shared.onHighlightChange}
                unitSystem="metric"
              />
            </div>
            <ClimbsSection
              climbs={spikeClimbs}
              cells={cells}
              unitSystem="metric"
              onSelect={shared.onActiveChange}
            />
          </div>
        </MapPane>
      </Page>
    );
  },
};

/**
 * **2 — One dock.** No card at all.
 *
 * If the drawn readings belong to the sheet, and the figures are five short
 * lines, there is not much left to justify a second floating object. The
 * identity moves into the dock's own left column and the corner of the map
 * goes back to being map — which is the largest single gain of any of these,
 * and the most map any arrangement here leaves.
 *
 * What it costs is the route's *presence*. The pill was how a reader knew, at
 * a glance and without reading, that they were looking at one route rather
 * than the library; folded into a dock, that is now something they have to
 * read at the bottom of the screen.
 */
export const OneDock: StoryObj = {
  render: () => {
    const shared = useComposition();

    return (
      <Page>
        <MenuBar />
        <MapPane
          coordinates={spikeCoordinates}
          width={PAGE.width}
          height={PAGE.height - 56}
          sheet={
            <StackClimbsSheet
              {...sheet}
              {...shared}
              weatherFrame="capped"
              lead={<RouteIdentity {...identity} named />}
            />
          }
        >
          <span />
        </MapPane>
      </Page>
    );
  },
};

/**
 * **3 — Quiet glass.** The same two panels, one visual language.
 *
 * Takes the arrangement as given and fixes only how it looks: both panels get
 * the same radius, the same hairline instead of a shadow, and a translucent
 * ground so the map stays legible under them and the two read as layers of one
 * surface rather than as two cards dropped on a picture.
 *
 * It leaves the duplication alone on purpose. If the composition still feels
 * wrong once both panels agree with each other, the problem was never the
 * styling — which is worth knowing before spending a redesign on it.
 */
export const QuietGlass: StoryObj = {
  render: () => {
    const shared = useComposition();
    const [collapsed, setCollapsed] = useState(false);

    return (
      <Page>
        <MenuBar />
        {/*
         * Applied from outside rather than as a prop on each panel: this is a
         * question about a surface treatment, and threading it through two
         * component APIs to try it would be building the answer before asking.
         */}
        <div className="contents [&_section]:bg-[color-mix(in_srgb,var(--panel)_86%,transparent)] [&_section]:shadow-none [&_section]:ring-0 [&_section]:backdrop-blur-md [&_section]:[border:1px_solid_var(--rule)]">
          <MapPane
            coordinates={spikeCoordinates}
            width={PAGE.width}
            height={PAGE.height - 56}
            sheet={<StackClimbsSheet {...sheet} {...shared} weatherFrame="capped" />}
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
              highlight={shared.highlight}
              onHighlightChange={shared.onHighlightChange}
              unitSystem="metric"
              onClose={() => {}}
              sourceBaseUrls={{ veloplanner: "https://veloplanner.com" }}
              libraryCount={47}
            />
          </MapPane>
        </div>
      </Page>
    );
  },
};

/**
 * **4 — Banded.** The furniture comes off the map entirely.
 *
 * The identity becomes a full-width strip directly under the menu, and the
 * sheet stays along the foot, so the map is a clean rectangle between two
 * horizontal bands rather than a picture with things standing on it. Nothing
 * overlaps, nothing needs a shadow to sit above anything, and the camera never
 * has to be told what is covering it.
 *
 * The cost is real: those bands take their height from the map permanently,
 * where a floating panel only borrows it and can be folded away. It is the
 * most orderly arrangement here and the least generous one.
 */
export const Banded: StoryObj = {
  render: () => {
    const shared = useComposition();

    return (
      <Page>
        <MenuBar />
        <div className="border-b border-[var(--rule)] bg-[var(--panel)] px-4 py-2">
          <RouteIdentity {...identity} layout="row" named />
        </div>
        <MapPane
          coordinates={spikeCoordinates}
          width={PAGE.width}
          height={PAGE.height - 56 - 58}
          sheet={<StackClimbsSheet {...sheet} {...shared} weatherFrame="capped" />}
        >
          <span />
        </MapPane>
      </Page>
    );
  },
};

const meta = {
  title: "Spikes/Composition",
  parameters: { layout: "fullscreen" },
} satisfies Meta;

export default meta;
