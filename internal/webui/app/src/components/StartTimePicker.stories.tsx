import type { Meta, StoryObj } from "@storybook/react-vite";
import { useState } from "react";
import { StartTimePicker } from "./StartTimePicker";

function Picker() {
  const [value, setValue] = useState<Date | null>(null);

  return <StartTimePicker value={value} onChange={setValue} movingSeconds={6_420} />;
}

const meta = {
  title: "Components/Start Time Picker",
  component: StartTimePicker,
  tags: ["autodocs"],
  args: { value: null, onChange: () => {} },
  decorators: [
    (Story) => (
      <div className="max-w-md bg-[var(--base)] p-6">
        <Story />
      </div>
    ),
  ],
} satisfies Meta<typeof StartTimePicker>;

export default meta;
type Story = StoryObj<typeof meta>;

export const Default: Story = { render: () => <Picker /> };
