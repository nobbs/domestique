import type { Meta, StoryObj } from "@storybook/react-vite";
import { expect } from "storybook/test";
import { SyncCard } from "./SyncCard";

const meta = {
  title: "Features/Sync/Sync Card",
  component: SyncCard,
  tags: ["autodocs"],
  args: { id: "now", heading: "Now" },
  decorators: [
    (Story) => (
      <div className="w-96 bg-[var(--base)] p-4">
        <Story />
      </div>
    ),
  ],
} satisfies Meta<typeof SyncCard>;

export default meta;
type Story = StoryObj<typeof meta>;

export const Default: Story = {
  args: { children: <p className="text-sm text-[var(--ink-2)]">Nothing is running.</p> },
  play: async ({ canvas }) => {
    await expect(canvas.getByRole("region", { name: "Now" })).toBeInTheDocument();
  },
};
