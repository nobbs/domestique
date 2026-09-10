import type { Meta, StoryObj } from "@storybook/react-vite";
import type { RouteClimb, RouteClimbAttempt } from "../../api/types";
import { RideClimbs } from "./RideClimbs";

const RIDE_ID = "42";

function attempt(
  overrides: Partial<RouteClimbAttempt> & { activityId: string },
): RouteClimbAttempt {
  return {
    riddenAt: "2026-09-01T06:00:00Z",
    seconds: 420,
    vamMetresPerHour: 900,
    ...overrides,
  };
}

const meta = {
  title: "Features/Activity/Climbs",
  component: RideClimbs,
  tags: ["autodocs"],
  decorators: [
    (Story) => (
      <div className="max-w-md p-6">
        <Story />
      </div>
    ),
  ],
} satisfies Meta<typeof RideClimbs>;

export default meta;
type Story = StoryObj<typeof meta>;

/**
 * A hilly ride matched to a route with three sustained climbs: a personal
 * best on the first, a middling attempt on the second beside an earlier
 * ride's best, and a first-ever attempt on the third.
 */
export const HillyRide: Story = {
  args: {
    activityId: RIDE_ID,
    climbs: [
      {
        startMetres: 0,
        endMetres: 2100,
        distanceMetres: 2100,
        ascentMetres: 190,
        averageGradePercent: 6.4,
        maxGradePercent: 12.1,
        attempts: [
          attempt({
            activityId: RIDE_ID,
            seconds: 372,
            heartRateBpm: 162.4,
            powerWatts: 271.6,
            riddenAt: "2026-09-01T06:00:00Z",
          }),
          attempt({
            activityId: "9001",
            seconds: 398,
            heartRateBpm: 158.1,
            powerWatts: 254.2,
            riddenAt: "2026-06-01T06:00:00Z",
          }),
        ],
      },
      {
        startMetres: 8400,
        endMetres: 9600,
        distanceMetres: 1200,
        ascentMetres: 96,
        averageGradePercent: 5.1,
        maxGradePercent: 8.7,
        attempts: [
          attempt({
            activityId: "9001",
            seconds: 205,
            heartRateBpm: 165.8,
            estimatedPowerWatts: 288.4,
            riddenAt: "2026-06-01T06:00:00Z",
          }),
          attempt({
            activityId: RIDE_ID,
            seconds: 224,
            heartRateBpm: 160.2,
            estimatedPowerWatts: 261.7,
            riddenAt: "2026-09-01T06:00:00Z",
          }),
        ],
      },
      {
        startMetres: 15200,
        endMetres: 16800,
        distanceMetres: 1600,
        ascentMetres: 142,
        averageGradePercent: 5.9,
        maxGradePercent: 9.4,
        attempts: [
          attempt({ activityId: RIDE_ID, seconds: 312, heartRateBpm: 168.9, powerWatts: 289.3 }),
        ],
      },
    ] satisfies RouteClimb[],
  },
};
