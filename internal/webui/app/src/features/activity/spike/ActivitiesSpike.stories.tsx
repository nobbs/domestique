/**
 * Three positions on how the activities index should read. Storybook only;
 * see the note on each variant in `indexVariants.tsx` for the bet it makes.
 */

import type { Meta, StoryObj } from "@storybook/react-vite";
import { StoryProviders } from "../../../storybook/fixtures";
import { CardsIndex, LedgerIndex, WeeksIndex } from "./indexVariants";

const meta = {
  title: "Spikes/Activities Index",
  parameters: { layout: "fullscreen", chromatic: { disableSnapshot: true } },
} satisfies Meta;

export default meta;
type Story = StoryObj<typeof meta>;

function page(Variant: () => React.JSX.Element): Story {
  return {
    render: () => (
      <StoryProviders>
        <div className="min-h-dvh bg-[var(--base)] p-6">
          <Variant />
        </div>
      </StoryProviders>
    ),
  };
}

/** A · A training log: one row per ride, months as the only rules, each month's totals in its header. */
export const Ledger = page(LedgerIndex);
/** B · Browsing: the glyph and one large distance per card, three across. */
export const Cards = page(CardsIndex);
/** C · The rhythm: one row per week, seven day columns, a chip per ride sized by its distance. */
export const Weeks = page(WeeksIndex);
