import type { Meta, StoryObj } from "@storybook/react-vite";
import type { BoundingBox } from "../../api/types";
import { buildProfile } from "../../lib/profile";
import { coordinates } from "../../storybook/fixtures";
import { MapViewport } from "../map/MapViewport";
import { MapWidget } from "../map/MapWidget";
import { RouteOverlay } from "./RouteOverlay";

const styleUrl = "https://tiles.openfreemap.org/styles/bright";
const bounds: BoundingBox = [7.995, 48.995, 8.045, 49.025];
const profile = buildProfile(coordinates);

const meta = {
  title: "Components/Route/Route Overlay",
  component: RouteOverlay,
  tags: ["autodocs"],
  args: { coordinates },
  decorators: [
    (Story) => (
      <div className="h-[34rem] overflow-hidden rounded-xl">
        <Story />
      </div>
    ),
  ],
} satisfies Meta<typeof RouteOverlay>;

export default meta;
type Story = StoryObj<typeof meta>;

export const Default: Story = {
  render: () => (
    <MapWidget styleUrl={styleUrl}>
      <MapViewport bounds={bounds} maxZoom={14} />
      <RouteOverlay coordinates={coordinates} profile={profile} activeProfile={profile} />
    </MapWidget>
  ),
};
