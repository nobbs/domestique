import type { Meta, StoryObj } from "@storybook/react-vite";
import { coordinates } from "../../storybook/fixtures";
import { RouteGlyph } from "./RouteGlyph";

const meta = {
  title: "Components/Route/Route Glyph",
  component: RouteGlyph,
  tags: ["autodocs"],
  decorators: [
    (Story) => (
      <div className="size-24 bg-[var(--panel)] p-4">
        <Story />
      </div>
    ),
  ],
} satisfies Meta<typeof RouteGlyph>;

export default meta;
type Story = StoryObj<typeof meta>;

export const Default: Story = {
  args: { coordinates, title: "Alpine loop", band: 3 },
};
