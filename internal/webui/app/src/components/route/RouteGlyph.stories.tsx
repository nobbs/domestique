import type { Meta, StoryObj } from "@storybook/react-vite";
import { expect } from "storybook/test";
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
  play: async ({ canvas, args }) => {
    // Labels the shape for assistive technology.
    await expect(canvas.getByRole("img", { name: new RegExp(args.title) })).toBeInTheDocument();
    // Carries the band as data, for the ramp in the stylesheet to colour.
    await expect(
      canvas.getByRole("img", { name: new RegExp(args.title) }).querySelector("polyline"),
    ).toHaveAttribute("data-band", String(args.band));
  },
};

/** Renders a presentational placeholder instead of an empty graphic. */
export const NoGeometry: Story = {
  args: { coordinates: [], title: "Empty", band: 0 },
  play: async ({ canvas }) => {
    await expect(canvas.queryByRole("img")).not.toBeInTheDocument();
    await expect(canvas.getByRole("presentation")).toBeInTheDocument();
  },
};
