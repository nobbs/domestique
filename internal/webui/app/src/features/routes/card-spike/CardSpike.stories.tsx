/**
 * Five positions on how the route card should look, side by side.
 *
 * Storybook only: nothing here is imported by the application. What is being
 * compared is the card's dividing, labelling and weighting — see the note on
 * each variant in `variants.tsx` for the bet it is making.
 */

import type { Meta, StoryObj } from "@storybook/react-vite";
import { useState } from "react";
import type { Highlight } from "../../../lib/highlight";
import { StoryProviders } from "../../../storybook/fixtures";
import { AirCard, BandCard, HierarchyCard, LedgerCard, RailCard, SlideCard } from "./variants";

const meta = {
  title: "Spikes/Route Card",
  tags: ["autodocs"],
  parameters: { layout: "fullscreen", chromatic: { disableSnapshot: true } },
} satisfies Meta;

export default meta;
type Story = StoryObj<typeof meta>;

const VARIANTS = [
  {
    key: "air",
    name: "A · Air",
    note: "No rules. Space divides; sections name themselves in small caps.",
    Card: AirCard,
  },
  {
    key: "bands",
    name: "B · Bands",
    note: "Folds sit on their own ground. Headline above, two drawers below.",
    Card: BandCard,
  },
  {
    key: "ledger",
    name: "C · Ledger",
    note: "Commits to being a table: dotted leaders, rules only between sections.",
    Card: LedgerCard,
  },
  {
    key: "hierarchy",
    name: "D · Hierarchy",
    note: "Two figures decide the ride; the other five become one quiet line.",
    Card: HierarchyCard,
  },
  {
    key: "rail",
    name: "E · Rail",
    note: "Divides down rather than across, so an open section is visibly inside something.",
    Card: RailCard,
  },
] as const;

/** All five against the same route, on the ground the map would give them. */
export const SideBySide: Story = {
  render: () => {
    // One highlight across all five, so pressing a class in one shows what the
    // treatment does to the others.
    const [highlight, setHighlight] = useState<Highlight | null>(null);

    return (
      <StoryProviders>
        <div className="flex flex-wrap items-start gap-6 bg-[var(--base)] p-6">
          {VARIANTS.map(({ key, name, note, Card }) => (
            <div key={key} className="grid w-[23rem] gap-2">
              <div>
                <h2 className="text-sm font-semibold">{name}</h2>
                <p className="text-xs text-[var(--ink-2)]">{note}</p>
              </div>
              <Card highlight={highlight} onHighlightChange={setHighlight} />
            </div>
          ))}
        </div>
      </StoryProviders>
    );
  },
};

function only(Card: (typeof VARIANTS)[number]["Card"]): Story {
  return {
    render: () => {
      const [highlight, setHighlight] = useState<Highlight | null>(null);

      return (
        <StoryProviders>
          <div className="bg-[var(--base)] p-6">
            <Card highlight={highlight} onHighlightChange={setHighlight} />
          </div>
        </StoryProviders>
      );
    },
  };
}

export const AirOnly = only(AirCard);
export const BandsOnly = only(BandCard);
export const LedgerOnly = only(LedgerCard);
export const HierarchyOnly = only(HierarchyCard);
export const RailOnly = only(RailCard);
export const SlideOnly = only(SlideCard);
