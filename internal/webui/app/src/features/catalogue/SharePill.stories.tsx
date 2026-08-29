import type { Meta, StoryObj } from "@storybook/react-vite";
import { SharePill } from "./SharePill";

const meta = {
  title: "Features/Catalogue/Share Pill",
  component: SharePill,
  tags: ["autodocs"],
  args: { colour: "var(--grade-2)", label: "6%", share: 0.48 },
  decorators: [
    (Story) => (
      <div className="flex flex-wrap gap-1 bg-[var(--panel)] p-4">
        <Story />
      </div>
    ),
  ],
} satisfies Meta<typeof SharePill>;

export default meta;
type Story = StoryObj<typeof meta>;

/** Just under half the route, so the bar sits just under halfway. */
export const Default: Story = {};

/** The whole route: a solid rule, which is how "all of it" reads at a glance. */
export const Whole: Story = { args: { label: "flat", share: 1, colour: "var(--grade-0)" } };

/** A stub, for the part of a ride that is steep enough to be remembered. */
export const Sliver: Story = { args: { label: "12%+", share: 0.06, colour: "var(--grade-4)" } };

/** The same badge carrying a surface class rather than a gradient band. */
export const Surface: Story = {
  args: { label: "Gravel", share: 0.34, colour: "var(--surface-gravel)" },
};
