import type { Meta, StoryObj } from "@storybook/react-vite";
import { expect, userEvent } from "storybook/test";
import { ThemeToggle } from "./ThemeToggle";

const meta = {
  title: "Components/ThemeToggle",
  component: ThemeToggle,
  tags: ["autodocs"],
  decorators: [
    (Story) => (
      <div className="bg-[var(--panel)] p-3 text-[var(--ink)]">
        <Story />
      </div>
    ),
  ],
} satisfies Meta<typeof ThemeToggle>;

export default meta;
type Story = StoryObj<typeof meta>;

export const Ready: Story = {};

/**
 * The glyph is all a sighted reader has, so the name has to carry both the
 * scheme in force and the one a press would choose.
 *
 * It starts from whatever the browser running this story last stored rather
 * than from a fixed scheme — the assertion is the step, not the starting point.
 */
export const StepsToTheNextScheme: Story = {
  play: async ({ canvas }) => {
    const button = canvas.getByRole("button", { name: /^Theme: / });
    const before = button.getAttribute("aria-label");

    await userEvent.click(button);

    await expect(canvas.getByRole("button", { name: /^Theme: / })).not.toHaveAccessibleName(
      before ?? "",
    );
  },
};
