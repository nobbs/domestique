/**
 * Five ways the climbs sidebar could fold instead of the vertical tab.
 *
 * Storybook only. See `variants.tsx` for the bet each one makes.
 */

import type { Meta, StoryObj } from "@storybook/react-vite";
import type { ReactNode } from "react";
import { StoryProviders } from "../../../storybook/fixtures";
import {
  BracketsOnlyFold,
  CountChipFold,
  EdgeHandleFold,
  HeaderOnlyFold,
  RailItemFold,
} from "./variants";

const meta = {
  title: "Spikes/Climbs Fold",
  tags: ["autodocs"],
  parameters: { layout: "fullscreen", chromatic: { disableSnapshot: true } },
} satisfies Meta;

export default meta;
type Story = StoryObj<typeof meta>;

const VARIANTS = [
  {
    name: "A · Count chip",
    note: "A small pill in the panel's corner; no column at all when folded.",
    Fold: CountChipFold,
  },
  {
    name: "B · Header only",
    note: "The header row survives the fold and shrinks to fit; the table does not.",
    Fold: HeaderOnlyFold,
  },
  {
    name: "C · Rail item",
    note: "A third rail item, beside Profile and Forecast.",
    Fold: RailItemFold,
  },
  {
    name: "D · Brackets only",
    note: "The chart's own brackets open it; a tiny text link is the discoverable way in.",
    Fold: BracketsOnlyFold,
  },
  {
    name: "E · Edge handle",
    note: "A slim chevron strip at the right edge; the count only shows on hover.",
    Fold: EdgeHandleFold,
  },
] as const;

export const SideBySide: Story = {
  render: () => (
    <StoryProviders>
      <div className="grid gap-8 bg-[var(--ground)] p-6">
        {VARIANTS.map(({ name, note, Fold }) => (
          <div key={name} className="grid gap-2">
            <div>
              <h2 className="text-sm font-semibold">{name}</h2>
              <p className="text-xs text-[var(--ink-2)]">{note}</p>
            </div>
            <Fold />
          </div>
        ))}
      </div>
    </StoryProviders>
  ),
};

function only(Fold: () => ReactNode): Story {
  return {
    render: () => (
      <StoryProviders>
        <div className="bg-[var(--ground)] p-6">
          <Fold />
        </div>
      </StoryProviders>
    ),
  };
}

export const CountChip = only(CountChipFold);
export const HeaderOnly = only(HeaderOnlyFold);
export const RailItem = only(RailItemFold);
export const BracketsOnly = only(BracketsOnlyFold);
export const EdgeHandle = only(EdgeHandleFold);
