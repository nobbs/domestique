/**
 * Four takes on the ride page's hero after the sample's billing dashboard.
 *
 * Storybook only: nothing here is imported by the application.
 */

import type { Meta, StoryObj } from "@storybook/react-vite";
import { StoryProviders } from "../../../storybook/fixtures";
import { HeroFigure, PillsHero, StripHero, TilesHero } from "./heroVariants";

const meta = {
  title: "Spikes/Activity Hero (Elera)",
  parameters: { layout: "fullscreen" },
} satisfies Meta;

export default meta;
type Story = StoryObj<typeof meta>;

const VARIANTS = [
  {
    key: "strip",
    name: "A · Strip",
    note: "The dashboard's top row, full width over the map; a chip qualifies every cell.",
    Hero: StripHero,
  },
  {
    key: "pills",
    name: "B · Pills",
    note: "The aging card: each figure a tinted pill, its label, verdict and reading beneath.",
    Hero: PillsHero,
  },
  {
    key: "tiles",
    name: "C · Tiles",
    note: "Outlined tiles with a quiet mark in the corner; no fill, no shadow inside the card.",
    Hero: TilesHero,
  },
  {
    key: "hero",
    name: "D · Hero",
    note: "One big figure, a moving-share bar, dot rows for the rest, a sentence at the foot.",
    Hero: HeroFigure,
  },
] as const;

export const SideBySide: Story = {
  render: () => (
    <StoryProviders>
      <div className="grid gap-10 bg-[var(--base)] p-6">
        {VARIANTS.map(({ key, name, note, Hero }) => (
          <div key={key} className="mx-auto grid w-full max-w-5xl gap-3">
            <div>
              <h2 className="font-semibold text-sm">{name}</h2>
              <p className="text-[var(--ink-2)] text-xs">{note}</p>
            </div>
            <Hero />
          </div>
        ))}
      </div>
    </StoryProviders>
  ),
};
