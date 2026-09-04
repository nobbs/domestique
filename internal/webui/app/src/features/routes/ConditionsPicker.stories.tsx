/**
 * The choice and its key, at the width the dock gives them.
 */

import type { Meta, StoryObj } from "@storybook/react-vite";
import { useState } from "react";
import type { MeasureKey } from "../../lib/measures";
import { weatherSamples } from "../../storybook/fixtures";
import { ConditionsPicker } from "./ConditionsPicker";

function Picking({
  initial = null,
  samples = weatherSamples,
  movingSeconds = 6_420,
}: {
  initial?: MeasureKey | null;
  samples?: typeof weatherSamples;
  movingSeconds?: number | undefined;
}) {
  const [measure, setMeasure] = useState<MeasureKey | null>(initial);

  return (
    <ConditionsPicker
      measure={measure}
      onMeasureChange={setMeasure}
      samples={samples}
      movingSeconds={movingSeconds}
    />
  );
}

const meta = {
  title: "Components/Route/Conditions Picker",
  component: ConditionsPicker,
  tags: ["autodocs"],
  args: { measure: null, onMeasureChange: () => {}, samples: weatherSamples },
  decorators: [
    (Story) => (
      <div className="max-w-3xl bg-[var(--panel)] p-6">
        <Story />
      </div>
    ),
  ],
} satisfies Meta<typeof ConditionsPicker>;

export default meta;
type Story = StoryObj<typeof meta>;

/** Off, which is where every route starts. */
export const Off: Story = { render: () => <Picking /> };

/** Rain, whose lowest band the map paints nothing for — and the key says so. */
export const Rain: Story = { render: () => <Picking initial="rain" /> };

/** Five bands rather than four, and every one of them painted. */
export const Temperature: Story = { render: () => <Picking initial="temperature" /> };

/** No start time chosen: the choices are inert, and the line says what is missing. */
export const NoForecast: Story = { render: () => <Picking samples={[]} /> };

/** Nothing has predicted a moving time, which is not the reader's to fix. */
export const Unpredicted: Story = {
  render: () => <Picking samples={[]} movingSeconds={undefined} />,
};
