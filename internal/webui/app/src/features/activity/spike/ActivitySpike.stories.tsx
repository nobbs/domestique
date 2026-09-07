/**
 * Four positions on how the ride page should look.
 *
 * Storybook only: nothing here is imported by the application. What is being
 * compared is what the page puts first and how the rest follows — see the
 * note on each variant in `variants.tsx` for the bet it is making.
 */

import type { Meta, StoryObj } from "@storybook/react-vite";
import { liveMap, StoryProviders } from "../../../storybook/fixtures";
import { AtlasPage, BentoPage, HeadlinePage, LanesPage } from "./variants";

const meta = {
  title: "Spikes/Activity Page",
  parameters: { layout: "fullscreen", ...liveMap },
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

/** A · Four figures decide the ride; they are set large beside the map and everything else is quieter. */
export const Headline = page(HeadlinePage);
/** B · A dashboard of tiles: one label and one number each, the map the largest tile. */
export const Bento = page(BentoPage);
/** C · One distance axis: terrain, speed, heart rate, power and weather as lanes under one cursor. */
export const Lanes = page(LanesPage);
/** D · The map is the stage, as on the atlas; a rail card and a docked profile float over it. */
export const Atlas = page(AtlasPage);
