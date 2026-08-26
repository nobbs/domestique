import type { Meta, StoryObj } from "@storybook/react-vite";
import { expect, fn, userEvent } from "storybook/test";
import { coordinates, route } from "../../storybook/fixtures";
import { ResultRow } from "./ResultRow";

const meta = {
  title: "Features/Routes/Result Row",
  component: ResultRow,
  tags: ["autodocs"],
  args: {
    route,
    shape: { coordinates },
    change: null,
    onSelect: fn(),
    unitSystem: "metric",
  },
  decorators: [
    (Story) => (
      <ul className="w-96 bg-[var(--panel)] p-2">
        <Story />
      </ul>
    ),
  ],
} satisfies Meta<typeof ResultRow>;

export default meta;
type Story = StoryObj<typeof meta>;

export const Default: Story = {};

export const New: Story = { args: { change: "new" } };

export const Updated: Story = { args: { change: "updated" } };

export const NoGeometryYet: Story = { args: { shape: undefined } };

export const ReportsAPick: Story = {
  play: async ({ canvas, args }) => {
    await userEvent.click(canvas.getByRole("button", { name: new RegExp(route.title) }));

    await expect(args.onSelect).toHaveBeenCalled();
  },
};
