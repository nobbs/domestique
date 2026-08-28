import type { Meta, StoryObj } from "@storybook/react-vite";
import { liveMap } from "../../storybook/fixtures";
import { MapOverlay } from "./MapOverlay";
import { MapWidget } from "./MapWidget";

const styleUrl = "https://tiles.openfreemap.org/styles/bright";

const meta = {
  title: "Components/Map/Map Overlay",
  // `MapOverlay` portals into the container `useMap()` reports, which only a
  // real `<Map>` provides — the deterministic placeholder never mounts one,
  // so this stays with the live canvas, the same way the stories reviewing
  // real cartography do, rather than joining the other "chrome" stories.
  parameters: liveMap,
  component: MapOverlay,
  tags: ["autodocs"],
  args: { children: null },
  decorators: [
    (Story) => (
      <div className="relative h-[34rem] overflow-hidden rounded-xl">
        <Story />
      </div>
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
