import type { Meta, StoryObj } from "@storybook/react-vite";
import { StageChangeBadge } from "./StageChangeBadge";

const meta = {
  title: "Features/Routes/Stage Change Badge",
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

export const New: Story = { args: { change: "new" } };
export const Updated: Story = { args: { change: "updated" } };
export const Unchanged: Story = { args: { change: null } };
