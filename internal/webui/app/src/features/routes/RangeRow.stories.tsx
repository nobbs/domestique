import type { Meta, StoryObj } from "@storybook/react-vite";
import { useState } from "react";
import { expect, userEvent } from "storybook/test";
import type { NumericRange } from "../../lib/filters";
import { RangeRow } from "./RangeRow";

function Field() {
  const [range, setRange] = useState<NumericRange>({ min: null, max: null });

  return <RangeRow legend="Distance" unit="km" range={range} onChange={setRange} />;
}

const meta = {
  title: "Features/Routes/Range Row",
  tags: ["autodocs"],
  decorators: [
    (Story) => (
      <div className="w-64 bg-[var(--panel)] p-4">
        <Story />
      </div>
    ),
  ],
} satisfies Meta;

export default meta;
type Story = StoryObj<typeof meta>;

export const Empty: Story = { render: () => <Field /> };

export const TypingMinReportsIt: Story = {
  render: () => <Field />,
  play: async ({ canvas }) => {
    await userEvent.type(canvas.getByLabelText("Min"), "12");

    await expect(canvas.getByLabelText("Min")).toHaveValue("12");
  },
};
