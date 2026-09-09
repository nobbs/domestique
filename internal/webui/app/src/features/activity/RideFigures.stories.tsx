import type { Meta, StoryObj } from "@storybook/react-vite";
import type { Activity } from "../../api/types";
import { RideFigures } from "./RideFigures";

/** A two-hour ride, Wahoo's unless overridden. */
function ride(overrides: Partial<Activity> = {}): Activity {
  return {
    id: "1",
    startedAt: "2026-09-01T06:00:00Z",
    distanceMetres: 62_000,
    movingSeconds: 7_800,
    elapsedSeconds: 8_400,
    ascentMetres: 640,
    typeId: 0,
    locationId: 0,
    provider: "wahoo",
    metrics: { powerTss: 91.4, intensityFactor: 0.74 },
    ...overrides,
  };
}

const meta = {
  title: "Features/Activity/Figures",
  component: RideFigures,
  tags: ["autodocs"],
  decorators: [
    (Story) => (
      <div className="max-w-md p-6">
        <Story />
      </div>
    ),
  ],
} satisfies Meta<typeof RideFigures>;

export default meta;
type Story = StoryObj<typeof meta>;

/** A ride Wahoo recorded: no provider badge. */
export const Wahoo: Story = {
  args: { ride: ride() },
};

/** A ride the rider's own Zwift account recorded, badged as such. */
export const Zwift: Story = {
  args: { ride: ride({ provider: "zwift" }) },
};
