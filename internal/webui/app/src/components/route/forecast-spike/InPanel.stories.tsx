/**
 * A and B where they would actually live: inside the route panel, under the
 * chart, at the width the panel actually is.
 *
 * The comparison story runs at whatever the viewport gives it, which flatters
 * both — the panel is `w-[32.5rem]`, so the same twenty readings have about a
 * third of the room. Everything about the two layouts that only shows up when
 * cells get narrow shows up here.
 *
 * The current strip is the first of the three, drawn by the real component
 * against the same forecast, so the question is a before-and-after rather
 * than a description of one.
 */

import type { Meta, StoryObj } from "@storybook/react-vite";
import { IconChevronsRight } from "@tabler/icons-react";
import { useQueryClient } from "@tanstack/react-query";
import type { ReactNode } from "react";
import { useMemo, useState } from "react";
import { weatherQuery } from "../../../api/queries";
import { formatAscent, formatElevation } from "../../../lib/format";
import { buildProfile } from "../../../lib/profile";
import type { UnitSystem } from "../../../lib/units";
import { StoryProviders } from "../../../storybook/fixtures";
import { StartTimePicker } from "../../StartTimePicker";
import { ElevationProfile } from "../ElevationProfile";
import { ForecastStrip } from "../ForecastStrip";
import { buildCells } from "./cells";
import { FilmstripBand } from "./FilmstripBand";
import { START_AT, spikeCoordinates, spikePoints, spikeSamples } from "./fixture";
import { LanesBand } from "./LanesBand";

/**
 * Puts this day's forecast in the cache the real strip reads from.
 *
 * `StoryProviders` seeds the three-sample fixture and owns its own client, so
 * there is nowhere else to reach it from. Seeding during render rather than in
 * an effect is on purpose: an effect runs after the strip has already asked
 * and been told there is nothing, and the story would draw its "before" as an
 * empty space.
 */
function SeedForecast({ children }: { children: ReactNode }) {
  const client = useQueryClient();
  const { queryKey } = weatherQuery(spikeSamples);
  if (client.getQueryData(queryKey) === undefined) {
    client.setQueryData(queryKey, { points: spikePoints });
  }

  return children;
}

/**
 * Measures the width the band gets, which is the whole point of this story.
 *
 * A slot of its own rather than the panel's outer element, because the panel
 * has padding and a band must not draw itself into it — the same mistake the
 * comparison story made when it measured its padded container.
 */
function PanelBand({ render }: { render: (width: number) => ReactNode }) {
  const [element, setElement] = useState<HTMLDivElement | null>(null);
  const width = element?.clientWidth ?? 0;

  return <div ref={setElement}>{width > 0 ? render(width) : null}</div>;
}

/**
 * The panel's own chrome around one band: the heading row the profile sits
 * under, the chart, the start-time control, and then whichever forecast is
 * being tried. Identical for all three, so the only thing that differs
 * between them is the band.
 */
function PanelPreview({
  label,
  note,
  unitSystem,
  band,
}: {
  label: string;
  note: string;
  unitSystem: UnitSystem;
  band: (width: number) => ReactNode;
}) {
  const [startAt, setStartAt] = useState<Date | null>(START_AT);
  const profile = useMemo(() => buildProfile(spikeCoordinates), []);
  const range = profile
    ? `${formatElevation(profile.minElevationMetres, unitSystem)}–${formatElevation(profile.maxElevationMetres, unitSystem)}`
    : "";

  return (
    <section className="flex w-[32.5rem] shrink-0 flex-col gap-4 rounded-lg bg-[var(--panel)] p-4 shadow-[var(--shadow)]">
      <div>
        <p className="font-semibold text-[var(--ink)] text-sm">{label}</p>
        <p className="text-[var(--ink-2)] text-xs">{note}</p>
      </div>
      <div className="border-[var(--rule)] border-t pt-4">
        <h3 className="flex w-full items-center gap-2 font-semibold">
          <span>Elevation</span>
          <span className="min-w-0 text-[var(--ink-2)] text-xs">
            {range} · {formatAscent(3_600, unitSystem)} up
          </span>
          <IconChevronsRight className="rotate-90" aria-hidden="true" size={16} stroke={2} />
        </h3>
        <div className="mt-3 grid gap-3">
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
          <StartTimePicker value={startAt} onChange={setStartAt} movingSeconds={34_200} />
          <PanelBand render={band} />
        </div>
      </div>
    </section>
  );
}

function Comparison({ unitSystem }: { unitSystem: UnitSystem }) {
  const cells = useMemo(() => buildCells(spikeSamples, spikePoints, spikeCoordinates), []);
  const profile = useMemo(() => buildProfile(spikeCoordinates), []);
  const endMetres = profile?.endMetres ?? 0;
  const band = { cells, startMetres: 0, endMetres, unitSystem };

  return (
    <StoryProviders>
      <SeedForecast>
        <div className="flex flex-col items-start gap-6 bg-[var(--base)] p-6">
          <PanelPreview
            label="Now"
            note="The strip on main: rain as opacity, wind as a letter."
            unitSystem={unitSystem}
            band={() => (
              <ForecastStrip
                samples={spikeSamples}
                coordinates={spikeCoordinates}
                startMetres={0}
                endMetres={endMetres}
                unitSystem={unitSystem}
              />
            )}
          />
          <PanelPreview
            label="A — Lanes"
            note="One lane per measure."
            unitSystem={unitSystem}
            band={(width) => <LanesBand {...band} width={width} />}
          />
          <PanelPreview
            label="B — Filmstrip"
            note="One tile per moment."
            unitSystem={unitSystem}
            band={(width) => <FilmstripBand {...band} width={width} />}
          />
        </div>
      </SeedForecast>
    </StoryProviders>
  );
}

const meta = {
  title: "Spikes/Forecast In Panel",
  component: Comparison,
  parameters: { layout: "fullscreen" },
} satisfies Meta<typeof Comparison>;

export default meta;
type Story = StoryObj<typeof meta>;

/** All three at the panel's real width, side by side. */
export const InThePanel: Story = { args: { unitSystem: "metric" } };
