import type { Meta, StoryObj } from "@storybook/react-vite";
import { expect, userEvent } from "storybook/test";
import { ToggleGroup } from "../ui/toggle-group";
import { LegendChip } from "./LegendChip";

const meta = {
  title: "Components/Route/Legend Chip",
  component: LegendChip,
  tags: ["autodocs"],
  args: {
    value: "band:2",
    paintClassName: "bg-[var(--grade-2)]",
    title: "6 to 9%",
    ariaLabel: "Moderate, 6 to 9%, 22% of the route",
    label: "Moderate",
    share: "22%",
    swatch: { band: 2 },
  },
  decorators: [
    (Story) => (
      <ToggleGroup className="flex-col items-stretch bg-[var(--panel)] p-2">
        <ul className="flex flex-wrap gap-x-2 gap-y-1 text-xs text-[var(--ink-2)] tabular-nums">
          <Story />
        </ul>
      </ToggleGroup>
    ),
  ],
} satisfies Meta<typeof LegendChip>;

export default meta;
type Story = StoryObj<typeof meta>;

export const GradientBand: Story = {};

export const SurfaceClass: Story = {
  args: {
    value: "surface:gravel",
    paintClassName: "bg-[var(--surface-gravel)]",
    title: "Loose stone or crushed rock",
    ariaLabel: "Gravel, loose stone or crushed rock, 30% of the route",
    label: "Gravel",
    share: "30%",
    swatch: { surface: "gravel" },
  },
};

export const PressReportsItself: Story = {
  play: async ({ canvas }) => {
    const chip = canvas.getByRole("button", { name: /^Moderate/ });
    await expect(chip).toHaveAttribute("aria-pressed", "false");

    await userEvent.click(chip);

    await expect(chip).toHaveAttribute("aria-pressed", "true");
  },
};
