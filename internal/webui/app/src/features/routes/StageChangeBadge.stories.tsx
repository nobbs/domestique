import type { Meta, StoryObj } from "@storybook/react-vite";
import { expect } from "storybook/test";
import { StageChangeBadge } from "./StageChangeBadge";

const meta = {
  title: "Features/Atlas/Stage Change Badge",
  component: StageChangeBadge,
  tags: ["autodocs"],
  decorators: [
    (Story) => (
      <div className="bg-[var(--base)] p-4">
        <Story />
      </div>
    ),
  ],
} satisfies Meta<typeof StageChangeBadge>;

export default meta;
type Story = StoryObj<typeof meta>;

export const New: Story = {
  args: { change: "new" },
  play: async ({ canvas }) => {
    await expect(canvas.getByText("New")).toBeInTheDocument();
  },
};

export const Updated: Story = {
  args: { change: "updated" },
  play: async ({ canvas }) => {
    await expect(canvas.getByText("Updated")).toBeInTheDocument();
  },
};

/** The word is what a reader without colour actually reads: text, never colour alone. */
export const Unchanged: Story = {
  args: { change: null },
  play: async ({ canvas }) => {
    await expect(canvas.queryByText("New")).not.toBeInTheDocument();
    await expect(canvas.queryByText("Updated")).not.toBeInTheDocument();
  },
};
