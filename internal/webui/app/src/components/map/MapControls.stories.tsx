import type { Meta, StoryObj } from "@storybook/react-vite";
import { liveMap } from "../../storybook/fixtures";
import { MapControls } from "./MapControls";
import { MapWidget } from "./MapWidget";

const styleUrl = "https://tiles.openfreemap.org/styles/bright";

const meta = {
  title: "Components/Map/Map Controls",
  parameters: liveMap,
  component: MapControls,
  tags: ["autodocs"],
  decorators: [
    (Story) => (
      <div className="h-[34rem] overflow-hidden rounded-xl">
        <Story />
      </div>
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
