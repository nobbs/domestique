import type { Meta, StoryObj } from "@storybook/react-vite";
import { IconMinus, IconPlus } from "@tabler/icons-react";
import { expect, fn, userEvent } from "storybook/test";
import { Button } from "./Button";
import { ButtonGroup } from "./ButtonGroup";

const meta = {
  title: "Components/Button Group",
  component: ButtonGroup,
  tags: ["autodocs"],
  args: { children: null },
} satisfies Meta<typeof ButtonGroup>;

export default meta;
type Story = StoryObj<typeof meta>;

const zoomIn = fn();
const zoomOut = fn();

/** The map's zoom pair: one frame, one rule between them. */
export const Zoom: Story = {
  render: () => (
    <ButtonGroup>
      <Button
        variant="ghost"
        icon={<IconPlus stroke={2} />}
        onClick={zoomIn}
        aria-label="Zoom in"
      />
      <Button
        variant="ghost"
        icon={<IconMinus stroke={2} />}
        onClick={zoomOut}
        aria-label="Zoom out"
      />
    </ButtonGroup>
  ),
  play: async ({ canvas }) => {
    await userEvent.click(canvas.getByRole("button", { name: "Zoom in" }));
    await userEvent.click(canvas.getByRole("button", { name: "Zoom out" }));
    await expect(zoomIn).toHaveBeenCalled();
    await expect(zoomOut).toHaveBeenCalled();
  },
};

/** Disabled while the map it drives has not loaded. */
export const Disabled: Story = {
  render: () => (
    <ButtonGroup>
      <Button variant="ghost" icon={<IconPlus stroke={2} />} disabled aria-label="Zoom in" />
      <Button variant="ghost" icon={<IconMinus stroke={2} />} disabled aria-label="Zoom out" />
    </ButtonGroup>
  ),
};
