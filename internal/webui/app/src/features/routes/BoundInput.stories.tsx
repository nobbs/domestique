import type { Meta, StoryObj } from "@storybook/react-vite";
import { useState } from "react";
import { expect, userEvent } from "storybook/test";
import { BoundInput } from "./BoundInput";

/** Holds the stored value, as the range row it normally sits in does. */
function Field({ initial }: { initial: number | null }) {
  const [stored, setStored] = useState<number | null>(initial);

  return (
    <BoundInput
      label="Min"
      stored={stored}
      onChange={setStored}
      toDisplay={(value) => value}
      toStored={(value) => value}
    />
  );
}

const meta = {
  title: "Features/Routes/Bound Input",
  tags: ["autodocs"],
  decorators: [
    (Story) => (
      <div className="w-32 bg-[var(--panel)] p-4">
        <Story />
      </div>
    ),
  ],
} satisfies Meta;

export default meta;
type Story = StoryObj<typeof meta>;

export const Empty: Story = { render: () => <Field initial={null} /> };

export const Filled: Story = { render: () => <Field initial={42} /> };

/** A typed decimal keeps its trailing point rather than being re-rounded away. */
export const KeepsAPartialDecimal: Story = {
  render: () => <Field initial={null} />,
  play: async ({ canvas }) => {
    const input = canvas.getByLabelText("Min");
    await userEvent.type(input, "1.");

    await expect(input).toHaveValue("1.");
  },
};
