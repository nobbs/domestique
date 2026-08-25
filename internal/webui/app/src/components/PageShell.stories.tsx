import type { Meta, StoryObj } from "@storybook/react-vite";
import { PageShell } from "./Layout";

const meta = {
  title: "Components/Page Shell",
  component: PageShell,
  tags: ["autodocs"],
} satisfies Meta<typeof PageShell>;

export default meta;
type Story = StoryObj<typeof meta>;

export const Default: Story = {
  args: { children: <p className="text-sm">A readable page, outside the map workspace.</p> },
};
