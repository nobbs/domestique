import type { Meta, StoryObj } from "@storybook/react-vite";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { activityHeartRateDistributionQuery } from "../../api/queries";
import type { Activity, ActivityMetrics } from "../../api/types";
import { TrainingLoad } from "./TrainingLoad";

/** A two-hour ride carrying the given metrics. */
function ride(metrics: ActivityMetrics): Activity {
  return {
    id: "1",
    startedAt: "2026-09-01T06:00:00Z",
    distanceMetres: 62_000,
    movingSeconds: 7_800,
    elapsedSeconds: 8_400,
    ascentMetres: 640,
    typeId: 0,
    locationId: 0,
    indoor: false,
    provider: "wahoo",
    metrics: {
      averageHeartRateBpm: 141,
      maxHeartRateBpm: 176,
      averageCadenceRpm: 84,
      maxSpeedKmh: 58.3,
      ...metrics,
    },
  };
}

/**
 * A spread of heart rates around an endurance ride's middle, with a tail of
 * harder efforts, so the Distribution view has something to draw. Trimmed to
 * the first and last beat held, as the service sends it.
 */
function distribution() {
  const held = Array.from({ length: 80 }, (_, index) => {
    const bpm = 100 + index;
    const steady = 260 * Math.exp(-(((bpm - 145) / 9) ** 2));
    const efforts = 45 * Math.exp(-(((bpm - 168) / 5) ** 2));
    return Math.round(steady + efforts);
  });
  const first = held.findIndex((seconds) => seconds > 0);
  const last = held.findLastIndex((seconds) => seconds > 0);
  return { fromBpm: 100 + first, seconds: held.slice(first, last + 1) };
}

const meta = {
  title: "Features/Activity/Effort",
  component: TrainingLoad,
  tags: ["autodocs"],
  decorators: [
    (Story) => {
      const client = new QueryClient({
        defaultOptions: { queries: { retry: false, staleTime: Number.POSITIVE_INFINITY } },
      });
      client.setQueryData(activityHeartRateDistributionQuery("1").queryKey, distribution());
      return (
        <QueryClientProvider client={client}>
          <div className="max-w-4xl p-6">
            <Story />
          </div>
        </QueryClientProvider>
      );
    },
  ],
} satisfies Meta<typeof TrainingLoad>;

export default meta;
type Story = StoryObj<typeof meta>;

/** A steady endurance ride: one long zone, and short visits either side of it. */
export const Endurance: Story = {
  args: {
    ride: ride({
      zoneSeconds: [540, 7200, 1260, 240, 40],
      zoneBoundsBpm: [137.7, 153, 159.8, 170],
      deviceZoneSeconds: [600, 7080, 1300, 200, 100],
      trimp: 142.6,
      heartRateTss: 88.4,
      powerTss: 91.2,
      normalizedPowerWatts: 214,
      intensityFactor: 0.74,
      maxCadenceRpm: 108,
      maxPowerWatts: 612,
      thresholdPowerWatts: 260,
    }),
  },
};

/** Intervals, where the hardest zones hold long enough to be worth comparing. */
export const Intervals: Story = {
  args: {
    ride: ride({
      zoneSeconds: [300, 1500, 600, 1800, 900],
      zoneBoundsBpm: [137.7, 153, 159.8, 170],
      trimp: 198.2,
    }),
  },
};

/** A rider with no threshold entered: zones cut from the maximum instead. */
export const CutFromTheMaximum: Story = {
  args: {
    ride: ride({
      zoneSeconds: [1200, 2400, 300, 0, 0],
      zoneBoundsBpm: [114, 133, 152, 171],
      trimp: 96.8,
      heartRateTss: 61.3,
    }),
  },
};

/** A strap-only ride: the estimate's diagnostics fold into its power tile. */
export const EstimatedPower: Story = {
  args: {
    ride: ride({
      zoneSeconds: [420, 5400, 1440, 300, 60],
      zoneBoundsBpm: [137.7, 153, 159.8, 170],
      estimatedPowerWatts: 187.4,
      estimatedPedallingShare: 0.91,
      trimp: 132.4,
      heartRateTss: 84.6,
    }),
  },
};
