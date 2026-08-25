import type { Meta, StoryObj } from "@storybook/react-vite";
import { ScaleControl } from "react-map-gl/maplibre";
import { MapControlCluster } from "./MapControlCluster";
import { MapWidget } from "./MapWidget";

const styleUrl = "https://tiles.openfreemap.org/styles/bright";

const meta = {
  title: "Components/Map/Map Control Cluster",
  component: MapControlCluster,
  tags: ["autodocs"],
  args: { children: null },
  decorators: [
    (Story) => (
      <div className="h-[34rem] overflow-hidden rounded-xl">
        <Story />
      </div>
    ),
  ],
} satisfies Meta<typeof MapControlCluster>;

export default meta;
type Story = StoryObj<typeof meta>;

export const Default: Story = {
  render: () => (
    <MapWidget styleUrl={styleUrl}>
      <ScaleControl position="bottom-right" unit="metric" />
      <MapControlCluster>
        <button className="map-credits__toggle" type="button">
          Clustered control
        </button>
      </MapControlCluster>
    </MapWidget>
  ),
};
