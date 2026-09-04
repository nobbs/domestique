import type { Meta, StoryObj } from "@storybook/react-vite";
import { expect, userEvent } from "storybook/test";
import { StoryProviders } from "../../storybook/fixtures";
import { VolumePage } from "./VolumePage";

// The rides are seeded by `StoryProviders` under the key the page asks with,
// so nothing here reaches the network. Assertions wait rather than read once:
// these stories are also captured by a cloud browser, which settles on its own
// schedule.
const meta = {
  title: "Features/Volume/Page",
  component: VolumePage,
  tags: ["autodocs"],
  decorators: [
    (Story) => (
      <StoryProviders>
        <Story />
      </StoryProviders>
    ),
  ],
} satisfies Meta<typeof VolumePage>;

export default meta;
type Story = StoryObj<typeof meta>;

/** A year of riding, a week at a time. */
export const Default: Story = {};

/** The same year gathered into months, which is where a trend shows. */
export const ByMonth: Story = {
  play: async ({ canvas }) => {
    await userEvent.click(await canvas.findByRole("button", { name: "Month" }));

    await expect(await canvas.findByRole("heading", { name: "By month" })).toBeInTheDocument();
  },
};
