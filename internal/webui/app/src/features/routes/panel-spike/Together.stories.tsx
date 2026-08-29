/**
 * The pair, on one ride: **E** for the route and **B** for the weather.
 *
 * They are shown one above the other rather than in a single frame, because
 * where the wide panel sits is not decided and a story that put the filmstrip
 * across the foot of the map would be deciding it in passing. What this does
 * settle is the thing the choice actually turns on — each at the width it will
 * really get.
 *
 * Both are drawn against one ride — the panel spike's own loop, now carrying
 * the forecast spike's day — so the distance under the pill and the axis the
 * tiles are laid along describe the same afternoon.
 *
 * That width is the whole argument for the pairing. The forecast comparison
 * ran in the route panel and **A — Lanes** won there, because at five hundred
 * pixels for two hundred and twenty kilometres the filmstrip's tiles fall to
 * eleven and a tile that narrow holds nothing; the morning came out as a run
 * of empty boxes. B was the better band at full width and lost on a constraint
 * that is now going away — the forecast is moving to the wide panel, and this
 * is what it looks like once it has the room the comparison denied it.
 */

import type { Meta, StoryObj } from "@storybook/react-vite";
import { useState } from "react";
import { buildCells } from "../../../components/route/forecast-spike/cells";
import { FilmstripBand } from "../../../components/route/forecast-spike/FilmstripBand";
import { formatMovingTime } from "../../../lib/format";
import type { Highlight } from "../../../lib/highlight";
import type { UnitSystem } from "../../../lib/units";
import { StoryProviders } from "../../../storybook/fixtures";
import {
  spikeBands,
  spikeClimbs,
  spikeCoordinates,
  spikeHighestMetres,
  spikeRoute,
  spikeRuns,
  spikeSamples,
  spikeSubtitle,
  spikeSurface,
  spikeWeather,
} from "./fixture";
import { LengthsCard } from "./LengthsCard";
import { MapPane } from "./MapPane";
import { SpikePanel } from "./SpikePanel";

/** About what a wide panel gets on a laptop, once the window has its margins. */
const WIDE = 1_040;

const cells = buildCells(spikeSamples, spikeWeather, spikeCoordinates);

function Ride({ unitSystem = "metric" }: { unitSystem?: UnitSystem }) {
  const [collapsed, setCollapsed] = useState(false);
  const [highlight, setHighlight] = useState<Highlight | null>(null);

  return (
    <div className="grid gap-8">
      <figure className="grid gap-1">
        <figcaption className="text-sm">
          <span className="font-semibold">E — Lengths</span>{" "}
          <span className="text-[var(--ink-2)]">— on the map, where it really sits</span>
        </figcaption>
        <MapPane coordinates={spikeCoordinates}>
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
      </figure>
      <figure className="grid gap-1">
        <figcaption className="text-sm">
          <span className="font-semibold">B — Filmstrip</span>{" "}
          <span className="text-[var(--ink-2)]">
            — the same ride's weather, at the width the wide panel gives it
          </span>
        </figcaption>
        <div
          className="rounded-xl bg-[var(--panel)] p-4 shadow-[var(--shadow)] ring-1 ring-black/5"
          style={{ width: WIDE, maxWidth: "100%" }}
        >
          <p className="mb-2 text-xs text-[var(--ink-2)]">
            Setting off 06:00 · {formatMovingTime(spikeRoute.movingSeconds)} moving
          </p>
          <FilmstripBand
            cells={cells}
            width={WIDE - 32}
            startMetres={0}
            endMetres={spikeRoute.distanceMetres}
            unitSystem={unitSystem}
          />
        </div>
      </figure>
    </div>
  );
}

const meta = {
  title: "Spikes/Chosen Pair",
  parameters: { layout: "fullscreen" },
} satisfies Meta;

export default meta;
type Story = StoryObj<typeof meta>;

/** The chosen pair, both against the same day. */
export const TogetherEAndB: Story = {
  render: () => (
    <StoryProviders>
      <div className="p-6">
        <Ride />
      </div>
    </StoryProviders>
  ),
};
