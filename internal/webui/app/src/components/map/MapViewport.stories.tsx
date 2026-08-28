import type { Meta, StoryObj } from "@storybook/react-vite";
import type { BoundingBox } from "../../api/types";
import { ChromeMap } from "../../storybook/mapMock";
import { MapViewport } from "./MapViewport";
import { MapWidget } from "./MapWidget";

const styleUrl = "https://tiles.openfreemap.org/styles/bright";
const bounds: BoundingBox = [7.995, 48.995, 8.045, 49.025];

const meta = {
  title: "Components/Map/Map Viewport",
  component: MapViewport,
  tags: ["autodocs"],
  args: { bounds, maxZoom: 14 },
  decorators: [
    (Story) => (
      <ChromeMap>
        <div className="h-[34rem] overflow-hidden rounded-xl">
          <Story />
        </div>
      </ChromeMap>
    ),
  ],
} satisfies Meta<typeof MapViewport>;

export default meta;
type Story = StoryObj<typeof meta>;

export const Default: Story = {
  render: () => (
    <MapWidget styleUrl={styleUrl}>
      <MapViewport bounds={bounds} maxZoom={14} />
    </MapWidget>
  ),
};
