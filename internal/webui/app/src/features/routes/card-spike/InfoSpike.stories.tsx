/**
 * Four takes on the route card after the Elera sample, side by side.
 *
 * Storybook only: nothing here is imported by the application.
 */

import type { Meta, StoryObj } from "@storybook/react-vite";
import { useState } from "react";
import type { Highlight } from "../../../lib/highlight";
import { StoryProviders } from "../../../storybook/fixtures";
import { CalloutCard, LedgerCard, StripCard, TilesCard } from "./infoVariants";

const meta = {
  title: "Spikes/Route Card (Elera)",
  tags: ["autodocs"],
  parameters: { layout: "fullscreen" },
} satisfies Meta;

export default meta;
type Story = StoryObj<typeof meta>;

const VARIANTS = [
  {
    key: "tiles",
    name: "A · Tiles",
    note: "Six figures as quiet tiles on the card's own ground; nothing outranks anything.",
    Card: TilesCard,
  },
  {
    key: "ledger",
    name: "B · Ledger",
    note: "Distance and time as hero figures; the rest are hairline rows beneath.",
    Card: LedgerCard,
  },
  {
    key: "strip",
    name: "C · Strip",
    note: "The dashboard's KPI cells, icon squares and all; the extremes become one line.",
    Card: StripCard,
  },
  {
    key: "callout",
    name: "D · Callout",
    note: "Headline figures move into the subtitle; a tinted bar says what they add up to.",
    Card: CalloutCard,
  },
] as const;

export const SideBySide: Story = {
  render: () => {
    const [highlight, setHighlight] = useState<Highlight | null>(null);

    return (
      <StoryProviders>
        <div className="flex flex-wrap items-start gap-6 bg-[var(--ground)] p-6">
          {VARIANTS.map(({ key, name, note, Card }) => (
            <div key={key} className="grid w-[24rem] gap-2">
              <div>
                <h2 className="font-semibold text-sm">{name}</h2>
                <p className="text-[var(--ink-2)] text-xs">{note}</p>
              </div>
              <Card highlight={highlight} onHighlightChange={setHighlight} />
            </div>
          ))}
        </div>
      </StoryProviders>
    );
  },
};
