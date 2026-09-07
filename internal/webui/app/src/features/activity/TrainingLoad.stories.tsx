import type { Meta, StoryObj } from "@storybook/react-vite";
import { TrainingLoad } from "./TrainingLoad";

const meta = {
  title: "Features/Activity/Training Load",
  component: TrainingLoad,
  tags: ["autodocs"],
  decorators: [
    (Story) => (
      <div className="max-w-2xl p-6">
        <Story />
      </div>
    ),
  ],
} satisfies Meta<typeof TrainingLoad>;

export default meta;
type Story = StoryObj<typeof meta>;

/** A steady endurance ride: one long zone, and short visits either side of it. */
export const Endurance: Story = {
  args: {
    metrics: {
      zoneSeconds: [540, 7200, 1260, 240, 40],
      zoneBoundsBpm: [144.5, 153, 161.5, 170],
      trimp: 142.6,
      heartRateTss: 88.4,
      powerTss: 91.2,
      normalizedPowerWatts: 214,
      intensityFactor: 0.74,
    },
  },
};

/** Intervals, where the hardest zones hold long enough to be worth comparing. */
export const Intervals: Story = {
  args: {
    metrics: {
      zoneSeconds: [300, 1500, 600, 1800, 900],
      zoneBoundsBpm: [144.5, 153, 161.5, 170],
      trimp: 198.2,
    },
  },
};

/** A rider with no threshold entered: zones cut from the maximum instead. */
export const CutFromTheMaximum: Story = {
  args: {
    metrics: {
      zoneSeconds: [1200, 2400, 300, 0, 0],
      zoneBoundsBpm: [114, 133, 152, 171],
      trimp: 96.8,
      heartRateTss: 61.3,
    },
  },
};
