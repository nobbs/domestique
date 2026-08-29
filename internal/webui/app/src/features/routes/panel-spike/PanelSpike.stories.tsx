/**
 * The four alternatives, each on its own map.
 *
 * On a map deliberately, and at the position and size the shell actually gives
 * the panel. The whole argument for resting as a pill is how much ground it
 * gives back, and a card judged on a white page has already won that argument
 * without being asked the question. The map underneath is synthetic — drawn
 * from the fixture's own geometry rather than fetched — which is enough to
 * judge coverage and contrast and is not enough to judge legibility over
 * satellite imagery.
 */

import type { Meta, StoryObj } from "@storybook/react-vite";
import type { ComponentType } from "react";
import { useState } from "react";
import type { Highlight } from "../../../lib/highlight";
import type { UnitSystem } from "../../../lib/units";
import { StoryProviders } from "../../../storybook/fixtures";
import { ColumnsCard } from "./ColumnsCard";
import {
  spikeBands,
  spikeClimbs,
  spikeCoordinates,
  spikeHighestMetres,
  spikeRoute,
  spikeRuns,
  spikeSubtitle,
  spikeSurface,
} from "./fixture";
import { LedgerCard } from "./LedgerCard";
import { LengthsCard } from "./LengthsCard";
import { SentenceCard } from "./SentenceCard";
import { SpikePanel } from "./SpikePanel";
import type { CardProps } from "./shared";
import { TerrainCard } from "./TerrainCard";

const PANE = { width: 860, height: 540 };

/** Rings standing in for contour lines, so the ground has some texture on it. */
const CONTOURS = Array.from({ length: 13 }, (_, index) => 90 + index * 34);

/** The fixture's loop, projected into the pane, so the map has this route on it. */
function routePath(): string {
  const longitudes = spikeCoordinates.map((point) => point[0]);
  const latitudes = spikeCoordinates.map((point) => point[1]);
  const west = Math.min(...longitudes);
  const east = Math.max(...longitudes);
  const south = Math.min(...latitudes);
  const north = Math.max(...latitudes);
  const scale = Math.min((PANE.width - 360) / (east - west), (PANE.height - 140) / (north - south));

  return spikeCoordinates
    .filter((_, index) => index % 8 === 0)
    .map(([longitude, latitude], index) => {
      const x = PANE.width - 60 - (east - longitude) * scale;
      const y = 70 + (north - latitude) * scale;

      return `${index === 0 ? "M" : "L"}${x.toFixed(1)} ${y.toFixed(1)}`;
    })
    .join(" ");
}

/** Ground to stand the panel on: not cartography, just something that is not white. */
function MapBackdrop() {
  return (
    <svg
      className="absolute inset-0 size-full"
      viewBox={`0 0 ${PANE.width} ${PANE.height}`}
      preserveAspectRatio="none"
      aria-hidden="true"
    >
      <rect width={PANE.width} height={PANE.height} className="fill-[var(--base)]" />
      {CONTOURS.map((radius) => (
        <ellipse
          key={radius}
          cx={PANE.width * 0.62}
          cy={PANE.height * 0.5}
          rx={radius}
          ry={radius * 0.64}
          className="fill-none stroke-[var(--rule)]"
          strokeWidth={1}
        />
      ))}
      <path d={routePath()} className="fill-none stroke-[var(--accent)]" strokeWidth={3} />
    </svg>
  );
}

function Preview({
  name,
  note,
  Card,
  unitSystem = "metric",
  startCollapsed = false,
}: {
  name: string;
  note: string;
  Card: ComponentType<CardProps>;
  unitSystem?: UnitSystem;
  startCollapsed?: boolean;
}) {
  const [collapsed, setCollapsed] = useState(startCollapsed);
  const [highlight, setHighlight] = useState<Highlight | null>(null);

  return (
    <figure className="grid gap-1">
      <figcaption className="text-sm">
        <span className="font-semibold">{name}</span>{" "}
        <span className="text-[var(--ink-2)]">— {note}</span>
      </figcaption>
      <div
        className="relative overflow-hidden rounded-lg ring-1 ring-[var(--rule)]"
        style={{ width: PANE.width, height: PANE.height }}
      >
        <MapBackdrop />
        {/* Where the shell puts it, at the size the shell gives it. */}
        <div className="absolute top-3 left-3">
          <SpikePanel
            Card={Card}
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
        </div>
      </div>
    </figure>
  );
}

const meta = {
  title: "Spikes/Route Panel",
  parameters: { layout: "fullscreen" },
} satisfies Meta;

export default meta;
type Story = StoryObj<typeof meta>;

const ALTERNATIVES = [
  {
    key: "A",
    name: "A — Ledger",
    note: "one ruled column; the chips are the bar",
    Card: LedgerCard,
  },
  {
    key: "B",
    name: "B — Terrain",
    note: "both mixes in ride order, cols bracketed, on one axis",
    Card: TerrainCard,
  },
  {
    key: "C",
    name: "C — Sentence",
    note: "generated prose; every class name is the control",
    Card: SentenceCard,
  },
  {
    key: "D",
    name: "D — Columns",
    note: "turned sideways; labels beside their own segment, as shares",
    Card: ColumnsCard,
  },
  {
    key: "E",
    name: "E — Lengths",
    note: "D's arrangement, labelled with ground covered rather than share",
    Card: LengthsCard,
  },
] as const;

/** One alternative on its own, which is how each is actually looked at. */
function only(key: (typeof ALTERNATIVES)[number]["key"], unitSystem: UnitSystem = "metric") {
  const entry = ALTERNATIVES.find((candidate) => candidate.key === key) ?? ALTERNATIVES[0];

  return () => (
    <StoryProviders>
      <div className="p-6">
        <Preview name={entry.name} note={entry.note} Card={entry.Card} unitSystem={unitSystem} />
      </div>
    </StoryProviders>
  );
}

export const LedgerA: Story = { render: only("A") };
export const TerrainB: Story = { render: only("B") };
export const SentenceC: Story = { render: only("C") };
export const ColumnsD: Story = { render: only("D") };
export const LengthsE: Story = { render: only("E") };

/** All four together, for the one judgement that needs them side by side. */
export const Alternatives: Story = {
  render: () => (
    <StoryProviders>
      <div className="grid gap-8 p-6">
        {ALTERNATIVES.map((entry) => (
          <Preview key={entry.key} name={entry.name} note={entry.note} Card={entry.Card} />
        ))}
      </div>
    </StoryProviders>
  ),
};

/** What the map looks like when the reader has put the card away. */
export const AtRest: Story = {
  render: () => (
    <StoryProviders>
      <div className="p-6">
        <Preview
          name="Resting"
          note="the pill: name, distance, ascent — and the map back"
          Card={LedgerCard}
          startCollapsed
        />
      </div>
    </StoryProviders>
  ),
};

/**
 * Miles and feet: where A's chips are tightest, and where E risks reporting one
 * column in two units at once.
 */
export const Imperial: Story = {
  render: () => (
    <StoryProviders>
      <div className="grid gap-8 p-6">
        {ALTERNATIVES.filter((entry) => entry.key === "A" || entry.key === "E").map((entry) => (
          <Preview
            key={entry.key}
            name={entry.name}
            note="imperial"
            Card={entry.Card}
            unitSystem="imperial"
          />
        ))}
      </div>
    </StoryProviders>
  ),
};
