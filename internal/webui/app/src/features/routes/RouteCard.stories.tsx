import type { Meta, StoryObj } from "@storybook/react-vite";
import { expect, fn, userEvent } from "storybook/test";
import { coordinates, route } from "../../storybook/fixtures";
import { RouteCard } from "./RouteCard";

const meta = {
  title: "Features/Routes/Route Card",
  component: RouteCard,
  tags: ["autodocs"],
  args: {
    route,
    shape: { coordinates },
    readAt: "19:38",
    change: null,
    onOpen: fn(),
    unitSystem: "metric",
  },
  decorators: [
    (Story) => (
      <ul className="w-96 bg-[var(--panel)] p-2">
        <Story />
      </ul>
    ),
  ],
} satisfies Meta<typeof RouteCard>;

export default meta;
type Story = StoryObj<typeof meta>;

export const Default: Story = {};

export const New: Story = { args: { change: "new" } };

export const NoReadTimeYet: Story = { args: { readAt: null } };

export const OpensTheRoute: Story = {
  play: async ({ canvas, args }) => {
    await userEvent.click(canvas.getByRole("button", { name: "Open route" }));

    await expect(args.onOpen).toHaveBeenCalled();
  },
};
