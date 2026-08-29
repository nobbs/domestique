import type { Meta, StoryObj } from "@storybook/react-vite";
import { expect, userEvent } from "storybook/test";
import { StoryProviders } from "../../storybook/fixtures";
import { CataloguePage } from "./CataloguePage";

// No map, so nothing here needs `liveMap`: the catalogue is a table over the
// listing `StoryProviders` already seeds, and it fetches no geometry.
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
    await userEvent.click(canvas.getByRole("button", { name: "Distance" }));

    await expect(canvas.getByRole("columnheader", { name: /Distance/ })).toHaveAttribute(
      "aria-sort",
      "descending",
    );
  },
};

/** A search that matches nothing says which control narrowed it away. */
export const NothingMatches: Story = {
  play: async ({ canvas }) => {
    await userEvent.type(canvas.getByRole("searchbox"), "montreal");

    await expect(canvas.getByText("Nothing here is called that.")).toBeInTheDocument();
  },
};
