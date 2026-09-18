import type { Meta, StoryObj } from "@storybook/react-vite";
import { StoryProviders } from "../../storybook/fixtures";
import { VolumePage } from "./VolumePage";

// The rides are seeded by `StoryProviders` under the key the page asks with,
// so nothing here reaches the network. Assertions wait rather than read once,
// since a slow machine settles on its own schedule.
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

/** A year of riding: weeks charted, months listed, the year and its records beside. */
export const Default: Story = {};
