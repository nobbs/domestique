import type { Meta, StoryObj } from "@storybook/react-vite";
import { useState } from "react";
import type { Highlight } from "../../lib/highlight";
import type { MixEntry } from "../../lib/mix";
import { bandEntries, surfaceEntries } from "../../lib/mix";
import { bands, route, surface } from "../../storybook/fixtures";
import { MixRow } from "./MixRow";

function Pair({ gradient, surfaceMix }: { gradient: MixEntry[]; surfaceMix: MixEntry[] }) {
  const [highlight, setHighlight] = useState<Highlight | null>(null);

  return (
    <div className="grid gap-0.5">
      <MixRow
        classesLabel="Gradient bands"
        entries={gradient}
        absence="No elevation data."
        tagSide="above"
        highlight={highlight}
        onHighlightChange={setHighlight}
        unitSystem="metric"
      />
      <MixRow
        classesLabel="Surface classes"
        entries={surfaceMix}
        absence="Surface not classified yet."
        tagSide="below"
        highlight={highlight}
        onHighlightChange={setHighlight}
        unitSystem="metric"
      />
    </div>
  );
}

const meta = {
  title: "Features/Atlas/Mix Row",
  tags: ["autodocs"],
  decorators: [
    (Story) => (
      <div className="w-[23rem] bg-[var(--panel)] p-3">
        <Story />
      </div>
    ),
  ],
} satisfies Meta;

export default meta;
type Story = StoryObj<typeof meta>;

/** The real gradient and surface mixes, as `RoutePanel` renders them. */
export const Default: Story = {
  render: () => (
    <Pair
      gradient={bandEntries(bands, route.distanceMetres)}
      surfaceMix={surfaceEntries(surface)}
    />
  ),
};

/**
 * A sliver next to a dominant class — the case that used to press two tags
 * together until their text ran into each other (`54.4 km1.0 km`).
 */
export const SliverBesideADominantClass: Story = {
  render: () => (
    <Pair
      gradient={bandEntries(bands, route.distanceMetres)}
      surfaceMix={[
        {
          highlight: { type: "surface", kind: "asphalt" },
          label: "Asphalt",
          description: "Sealed road surface.",
          share: 0.988,
          metres: 54_400,
          colour: "var(--surface-asphalt)",
        },
        {
          highlight: { type: "surface", kind: "paving" },
          label: "Paving",
          description: "Paved but unsealed surface.",
          share: 0.012,
          metres: 1_000,
          colour: "var(--surface-paving)",
        },
      ]}
    />
  ),
};

/** A class under a kilometre, shown in metres rather than a fraction of a kilometre. */
export const SubKilometreClass: Story = {
  render: () => (
    <Pair
      gradient={bandEntries(bands, route.distanceMetres)}
      surfaceMix={[
        {
          highlight: { type: "surface", kind: "asphalt" },
          label: "Asphalt",
          description: "Sealed road surface.",
          share: 0.988,
          metres: 54_400,
          colour: "var(--surface-asphalt)",
        },
        {
          highlight: { type: "surface", kind: "ground" },
          label: "Ground",
          description: "Unpaved natural ground.",
          share: 0.012,
          metres: 200,
          colour: "var(--surface-ground)",
        },
      ]}
    />
  ),
};

/** No classes at all — both rows fall back to a faded placeholder bar. */
export const NoData: Story = {
  render: () => <Pair gradient={[]} surfaceMix={[]} />,
};

/** Surface missing — its faded bar sits below the gradient's real one. */
export const SurfaceMissing: Story = {
  render: () => <Pair gradient={bandEntries(bands, route.distanceMetres)} surfaceMix={[]} />,
};

/** Gradient missing — its faded bar sits above the surface's real one. */
export const GradientMissing: Story = {
  render: () => <Pair gradient={[]} surfaceMix={surfaceEntries(surface)} />,
};
