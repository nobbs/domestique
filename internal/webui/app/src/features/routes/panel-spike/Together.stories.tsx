/**
 * The pair, on one ride: **E** for the route and **B** for the weather.
 *
 * They are shown one above the other rather than in a single frame, because
 * where the wide panel sits is not decided and a story that put the filmstrip
 * across the foot of the map would be deciding it in passing. What this does
 * settle is the thing the choice actually turns on — each at the width it will
 * really get.
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
import { spikePoints, spikeSamples } from "../../../components/route/forecast-spike/fixture";
import { formatMovingTime } from "../../../lib/format";
import type { Highlight } from "../../../lib/highlight";
import type { UnitSystem } from "../../../lib/units";
import { StoryProviders } from "../../../storybook/fixtures";
import { LengthsCard } from "./LengthsCard";
import { MapPane } from "./MapPane";
import { SpikePanel } from "./SpikePanel";
import {
  rideBands,
  rideClimbs,
  rideCoordinates,
  rideHighestMetres,
  rideRoute,
  rideRuns,
  rideSurface,
} from "./together";

/** About what a wide panel gets on a laptop, once the window has its margins. */
const WIDE = 1_040;

const cells = buildCells(spikeSamples, spikePoints, rideCoordinates);

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
        <MapPane coordinates={rideCoordinates}>
          <SpikePanel
            Card={LengthsCard}
            collapsed={collapsed}
            onCollapsedChange={setCollapsed}
            route={rideRoute}
            subtitle="Haute-Savoie · read 19:38"
            movingSeconds={rideRoute.movingSeconds}
            highestMetres={rideHighestMetres}
            bands={rideBands}
            runs={rideRuns}
            surface={rideSurface}
            surfaceAbsence="Surface not classified yet."
            climbs={rideClimbs}
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
            Setting off 06:00 · {formatMovingTime(rideRoute.movingSeconds)} moving
          </p>
          <FilmstripBand
            cells={cells}
            width={WIDE - 32}
            startMetres={0}
            endMetres={rideRoute.distanceMetres}
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
