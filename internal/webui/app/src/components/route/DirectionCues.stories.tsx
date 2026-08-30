import type { Meta, StoryObj } from "@storybook/react-vite";
import type { BoundingBox } from "../../api/types";
import { coordinates, liveMap } from "../../storybook/fixtures";
import { CartographyProvider } from "../map/CartographyContext";
import { MapViewport } from "../map/MapViewport";
import { MapWidget } from "../map/MapWidget";
import { DirectionCues } from "./DirectionCues";

const bounds: BoundingBox = [7.995, 48.995, 8.045, 49.025];
const styles = {
  light: "https://tiles.openfreemap.org/styles/bright",
  dark: "https://tiles.openfreemap.org/styles/dark",
};

const meta = {
  title: "Components/Route/Direction Cues",
  parameters: liveMap,
  component: DirectionCues,
  tags: ["autodocs"],
  args: { coordinates },
  decorators: [
    (Story) => (
      <div className="h-[34rem] overflow-hidden rounded-xl">
        <Story />
      </div>
    ),
  ],
} satisfies Meta<typeof DirectionCues>;

export default meta;
type Story = StoryObj<typeof meta>;

export const Default: Story = {
  render: () => (
    <CartographyProvider dark={false}>
      <MapWidget styleUrl={styles.light}>
        <MapViewport bounds={bounds} maxZoom={14} />
        <DirectionCues coordinates={coordinates} />
      </MapWidget>
    </CartographyProvider>
  ),
};

/** The cues invert with the basemap, so they stay readable over dark tiles. */
export const DarkBasemap: Story = {
  render: () => (
    <CartographyProvider dark={true}>
      <MapWidget styleUrl={styles.dark}>
        <MapViewport bounds={bounds} maxZoom={14} />
        <DirectionCues coordinates={coordinates} />
      </MapWidget>
    </CartographyProvider>
  ),
};
