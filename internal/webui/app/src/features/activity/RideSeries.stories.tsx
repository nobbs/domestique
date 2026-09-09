import type { Meta, StoryObj } from "@storybook/react-vite";
import { type RideSeriesKey, SeriesChips, type SeriesState } from "./RideSeries";

/** Every series off, which is where a ride page opens. */
const OFF: Record<RideSeriesKey, SeriesState> = {
  heartRate: "off",
  cadence: "off",
  speed: "off",
  temperature: "off",
  power: "off",
  targetPower: "off",
  estimatedPower: "off",
};

const meta = {
  title: "Features/Activity/Series Chips",
  component: SeriesChips,
  tags: ["autodocs"],
  args: { states: OFF, drawn: [], activeIndex: null, onToggle: () => {} },
  decorators: [
    (Story) => (
      <div className="max-w-3xl bg-[var(--panel)] p-6">
        <Story />
      </div>
    ),
  ],
} satisfies Meta<typeof SeriesChips>;

export default meta;
type Story = StoryObj<typeof meta>;

/** How a ride page opens: nothing drawn, and nothing fetched to draw it. */
export const Resting: Story = { args: { states: OFF } };

/**
 * Two series drawn with the cursor somewhere on the ride, a third still
 * arriving, and a power meter the bicycle never carried.
 */
export const Reading: Story = {
  args: {
    states: { ...OFF, heartRate: "drawn", cadence: "drawn", speed: "loading", power: "absent" },
    drawn: [
      {
        key: "heartRate",
        label: "Heart rate",
        unit: "bpm",
        colour: "var(--series-heart-rate)",
        values: [148],
      },
      {
        key: "cadence",
        label: "Cadence",
        unit: "rpm",
        colour: "var(--series-cadence)",
        values: [86],
      },
    ],
    activeIndex: 0,
  },
};
