/**
 * The card, the dock and the climbs, judged as one layout.
 *
 * Storybook only. See `workspace.tsx` for what the three decisions are and why
 * none of them can be judged on its own.
 */

import type { Meta, StoryObj } from "@storybook/react-vite";
import { StoryProviders } from "../../../storybook/fixtures";
import { Workspace } from "./workspace";

const meta = {
  title: "Spikes/Route Workspace",
  tags: ["autodocs"],
  parameters: { layout: "fullscreen", chromatic: { disableSnapshot: true } },
} satisfies Meta;

export default meta;
type Story = StoryObj<typeof meta>;

/** The slide card: one view at a time, so its height never moves. */
export const WithSlideCard: Story = {
  render: () => (
    <StoryProviders>
      <Workspace card="slide" />
    </StoryProviders>
  ),
};

/**
 * The fold card with only one section left to fold. Shorter than it was with
 * the climbs in it, but it still grows when the mix is opened — which is the
 * whole difference between these two stories.
 */
export const WithFoldCard: Story = {
  render: () => (
    <StoryProviders>
      <Workspace card="fold" />
    </StoryProviders>
  ),
};

/**
 * Nothing folded at all: the mixes are simply on the card.
 *
 * The version this arrives at once the climbs are in the dock — one section
 * left, and a control that only ever hides one thing removed rather than
 * styled.
 */
export const WithPlainCard: Story = {
  render: () => (
    <StoryProviders>
      <Workspace card="plain" />
    </StoryProviders>
  ),
};

/** The same card again, with both mixes laid across the width instead of up it. */
export const WithRowsCard: Story = {
  render: () => (
    <StoryProviders>
      <Workspace card="rows" />
    </StoryProviders>
  ),
};

/**
 * The rows again, with the segments separated.
 *
 * The dock's ground ribbon is a horizontal bar of the same classes in the same
 * colours, and it says where the gravel is. This one says how much there is,
 * sorted by size — so butted together they are the same picture meaning two
 * different things. The gaps are what stop this one reading as a map.
 */
export const WithGappedRowsCard: Story = {
  render: () => (
    <StoryProviders>
      <Workspace card="rows-gapped" />
    </StoryProviders>
  ),
};
