import type { Meta, StoryObj } from "@storybook/react-vite";
import { StoryProviders } from "../storybook/fixtures";
import { Layout } from "./Layout";

const meta = {
  title: "Components/Layout",
  component: Layout,
  tags: ["autodocs"],
  decorators: [
    (Story) => (
      <StoryProviders>
        <Story />
      </StoryProviders>
    ),
  ],
} satisfies Meta<typeof Layout>;

export default meta;
type Story = StoryObj<typeof meta>;

export const Workspace: Story = {
  args: {
    map: <div className="h-full bg-[linear-gradient(135deg,var(--base),var(--ground))]" />,
    children: (
      <div className="w-80 rounded-xl bg-[var(--panel)] p-4 text-sm shadow-[var(--shadow)]">
        Route library controls
      </div>
    ),
  },
};
