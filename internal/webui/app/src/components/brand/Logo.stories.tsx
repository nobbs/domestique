import type { Meta, StoryObj } from "@storybook/react-vite";
import { Logo } from "./Logo";

const meta = {
  title: "Components/Brand/Logo",
  component: Logo,
  tags: ["autodocs"],
  decorators: [
    (Story) => (
      <div className="bg-[var(--panel)] p-6 text-[var(--accent)]">
        <Story />
      </div>
    ),
  ],
} satisfies Meta<typeof Logo>;

export default meta;
type Story = StoryObj<typeof meta>;

export const Default: Story = { args: { size: 72 } };
