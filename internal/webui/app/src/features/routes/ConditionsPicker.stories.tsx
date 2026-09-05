/**
 * The choice and its key, at the width the dock gives them.
 */

import type { Meta, StoryObj } from "@storybook/react-vite";
import { useState } from "react";
import type { MeasureKey } from "../../lib/measures";
import { weatherSamples } from "../../storybook/fixtures";
import { ConditionsChoices, ConditionsKey } from "./ConditionsPicker";

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
    <div className="grid gap-2">
      <ConditionsChoices
        measure={measure}
        onMeasureChange={setMeasure}
        samples={samples}
        movingSeconds={movingSeconds}
      />
      <ConditionsKey measure={measure} samples={samples} />
    </div>
  );
}

const meta = {
  title: "Components/Route/Conditions Picker",
  component: Picking,
  tags: ["autodocs"],
  decorators: [
    (Story) => (
      <div className="max-w-3xl bg-[var(--panel)] p-6">
        <Story />
      </div>
    ),
  ],
} satisfies Meta<typeof Picking>;

export default meta;
type Story = StoryObj<typeof meta>;

/** Off, which is where every route starts. */
export const Off: Story = { args: {} };

/** Rain, whose lowest band the map paints nothing for — and the key says so. */
export const Rain: Story = { args: { initial: "rain" } };

/**
 * The one measure with two keys: the corridor's bands for how hard the wind
 * blows, and the route's own ramp for what it does to the rider.
 */
export const Wind: Story = { args: { initial: "wind" } };

/** Five bands rather than four, and every one of them painted. */
export const Temperature: Story = { args: { initial: "temperature" } };

/** No start time chosen: the choices are inert, and the line says what is missing. */
export const NoForecast: Story = { args: { samples: [] } };

/** Nothing has predicted a moving time, which is not the reader's to fix. */
export const Unpredicted: Story = {
  args: { samples: [], movingSeconds: undefined },
};
