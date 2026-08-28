import type { Meta, StoryObj } from "@storybook/react-vite";
import { ChromeMap } from "../../storybook/mapMock";
import { MapOverlay } from "./MapOverlay";
import { MapWidget } from "./MapWidget";

const styleUrl = "https://tiles.openfreemap.org/styles/bright";

const meta = {
  title: "Components/Map/Map Overlay",
  component: MapOverlay,
  tags: ["autodocs"],
  args: { children: null },
  decorators: [
    (Story) => (
      <ChromeMap>
        <div className="relative h-[34rem] overflow-hidden rounded-xl">
          <Story />
        </div>
      </ChromeMap>
    ),
  ],
} satisfies Meta<typeof MapOverlay>;

export default meta;
type Story = StoryObj<typeof meta>;

export const Default: Story = {
  render: () => (
    <MapWidget styleUrl={styleUrl}>
      <MapOverlay>
        <p className="absolute top-3 right-3 rounded-lg bg-[var(--panel)] px-3 py-2 text-sm shadow-[var(--shadow)]">
          HTML above the map
        </p>
      </MapOverlay>
    </MapWidget>
  ),
};
