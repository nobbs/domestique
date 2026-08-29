/**
 * Four ways to draw the same forecast, on the same day, at the same width.
 *
 * One story rather than four, because the question is not whether any of them
 * works — all four do, on three cells over three kilometres — but which one a
 * reader gets more out of when they are stacked and compared. So they are
 * stacked and compared.
 *
 * The elevation chart is not drawn here. The axis-pinned layouts are meant to
 * be read against terrain, and the terrain is the fixture's own: two climbs,
 * the second of which is where the front arrives.
 */

import type { Meta, StoryObj } from "@storybook/react-vite";
import { useMemo } from "react";
import { buildProfile } from "../../../lib/profile";
import type { UnitSystem } from "../../../lib/units";
import { useElementWidth } from "../../../lib/useElementWidth";
import { ElevationProfile } from "../ElevationProfile";
import { CardsBand } from "./CardsBand";
import { CurvesBand } from "./CurvesBand";
import { buildCells } from "./cells";
import { FilmstripBand } from "./FilmstripBand";
import { spikeCoordinates, spikePoints, spikeSamples } from "./fixture";
import { LanesBand } from "./LanesBand";

const LAYOUTS = [
  {
    key: "lanes",
    title: "A — Lanes",
    blurb: "One lane per measure, everything pinned to the axis.",
    Band: LanesBand,
  },
  {
    key: "filmstrip",
    title: "B — Filmstrip",
    blurb: "One tile per moment, as wide as the ground it covers.",
    Band: FilmstripBand,
  },
  {
    key: "curves",
    title: "C — Curves",
    blurb: "The day as a shape: temperature over rain.",
    Band: CurvesBand,
  },
  {
    key: "cards",
    title: "D — Cards",
    blurb: "The control. Even width, scrolls, off the axis.",
    Band: CardsBand,
  },
] as const;

function Comparison({ unitSystem }: { unitSystem: UnitSystem }) {
  const { ref, width } = useElementWidth<HTMLDivElement>();
  const cells = useMemo(() => buildCells(spikeSamples, spikePoints, spikeCoordinates), []);
  const profile = useMemo(() => buildProfile(spikeCoordinates), []);
  const endMetres = profile?.endMetres ?? 0;

  return (
    <div className="bg-[var(--base)] p-6">
      {/*
       * The ref sits inside the padding: a band measuring its own gutter
       * draws itself wider than the chart it is supposed to line up with.
       */}
      <div className="grid min-w-0 gap-6 [&>*]:min-w-0" ref={ref}>
        <div>
          <p className="mb-1 font-semibold text-[var(--ink)] text-sm">The terrain underneath</p>
          <ElevationProfile
            profile={profile}
            title="Alpine loop"
            surface={null}
            activeMetres={null}
            onActiveChange={() => {}}
            zoomWindow={null}
            onZoomChange={() => {}}
            highlight={null}
            unitSystem={unitSystem}
          />
        </div>
        {LAYOUTS.map(({ key, title, blurb, Band }) => (
          <div key={key}>
            <p className="font-semibold text-[var(--ink)] text-sm">{title}</p>
            <p className="mb-1 text-[var(--ink-2)] text-xs">{blurb}</p>
            <Band
              cells={cells}
              width={width}
              startMetres={0}
              endMetres={endMetres}
              unitSystem={unitSystem}
            />
          </div>
        ))}
      </div>
    </div>
  );
}

const meta = {
  title: "Spikes/Forecast",
  component: Comparison,
  parameters: { layout: "fullscreen" },
} satisfies Meta<typeof Comparison>;

export default meta;
type Story = StoryObj<typeof meta>;

/** All four, metric, on the 165 km day the fixture describes. */
export const Alternatives: Story = { args: { unitSystem: "metric" } };

/** The same four in imperial, which is where the figures get wider. */
export const Imperial: Story = { args: { unitSystem: "imperial" } };
