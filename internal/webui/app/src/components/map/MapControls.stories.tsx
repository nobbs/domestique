import type { Meta, StoryObj } from "@storybook/react-vite";
import { ChromeMap } from "../../storybook/mapMock";
import { MapControls } from "./MapControls";
import { MapWidget } from "./MapWidget";

const styleUrl = "https://tiles.openfreemap.org/styles/bright";

const meta = {
  title: "Components/Map/Map Controls",
  component: MapControls,
  tags: ["autodocs"],
  decorators: [
    (Story) => (
      <ChromeMap>
        <div className="h-[34rem] overflow-hidden rounded-xl">
          <Story />
        </div>
      </ChromeMap>
    ),
  ],
} satisfies Meta<typeof MapControls>;

export default meta;
type Story = StoryObj<typeof meta>;

export const Default: Story = {
  render: () => (
    <MapWidget styleUrl={styleUrl}>
      <MapControls />
    </MapWidget>
  ),
};
