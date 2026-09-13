import type { Meta, StoryObj } from "@storybook/react-vite";
import { SwitchableView } from "./SwitchableView";

const meta = {
  title: "Components/SwitchableView",
  component: SwitchableView,
  tags: ["autodocs"],
  decorators: [
    (Story) => (
      <div className="max-w-md rounded-xl bg-[var(--panel)] p-4 text-[var(--ink)] ring-1 ring-black/5">
        <Story />
      </div>
    ),
  ],
} satisfies Meta<typeof SwitchableView>;

export default meta;
type Story = StoryObj<typeof meta>;

const heading = <h3 className="font-medium text-sm">Subject</h3>;

/** Two ways to show one subject; the second is not rendered until it is picked. */
export const TwoViews: Story = {
  args: {
    label: "View",
    heading,
    views: [
      {
        value: "summary",
        label: "Summary",
        content: () => <p className="text-sm">The summary.</p>,
      },
      { value: "detail", label: "Detail", content: () => <p className="text-sm">The detail.</p> },
    ],
  },
};

/** One view has nothing to switch between, so no switch is drawn. */
export const OneView: Story = {
  args: {
    label: "View",
    heading,
    views: [
      { value: "only", label: "Only", content: () => <p className="text-sm">The only view.</p> },
    ],
  },
};
