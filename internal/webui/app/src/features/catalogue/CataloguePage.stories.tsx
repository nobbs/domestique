import type { Meta, StoryObj } from "@storybook/react-vite";
import { expect, userEvent } from "storybook/test";
import { StoryProviders } from "../../storybook/fixtures";
import { CataloguePage } from "./CataloguePage";

// No map, so nothing here needs `liveMap`: the catalogue is a table, and the
// geometry it fetches for the glyphs is already seeded by `StoryProviders`
// under the same keys the atlas caches it with.
//
// Every assertion below waits rather than reading once. These stories are
// captured by a cloud browser as well as run by the test runner, and a click
// or a keystroke there settles on its own schedule; a `getBy` that reads a
// beat early fails the story for the machine it ran on rather than for
// anything the page did.
const meta = {
  title: "Features/Catalogue/Page",
  component: CataloguePage,
  tags: ["autodocs"],
  decorators: [
    (Story) => (
      <StoryProviders>
        <Story />
      </StoryProviders>
    ),
  ],
} satisfies Meta<typeof CataloguePage>;

export default meta;
type Story = StoryObj<typeof meta>;

/** The library as the reader first meets it: by name, ascending. */
export const Default: Story = {};

/** Ranked by a measurement, which is what the page is for. */
export const RankedByDistance: Story = {
  play: async ({ canvas }) => {
    await userEvent.click(await canvas.findByRole("button", { name: "Distance" }));

    await expect(await canvas.findByRole("columnheader", { name: /Distance/ })).toHaveAttribute(
      "aria-sort",
      "descending",
    );
  },
};

/** A search that matches nothing says which control narrowed it away. */
export const NothingMatches: Story = {
  play: async ({ canvas }) => {
    // No inter-key delay: every keystroke rewrites the address and re-renders
    // the table, and eight of those at the default cadence is a long time to
    // hold a capture open.
    await userEvent.type(await canvas.findByRole("searchbox"), "montreal", { delay: null });

    await expect(await canvas.findByText("Nothing here is called that.")).toBeInTheDocument();
  },
};
