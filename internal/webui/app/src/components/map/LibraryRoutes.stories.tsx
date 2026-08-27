import type { Meta, StoryObj } from "@storybook/react-vite";
import type { BoundingBox } from "../../api/types";
import { coordinates, liveMap, StoryProviders } from "../../storybook/fixtures";
import { LibraryRoutes } from "./LibraryRoutes";
import { MapViewport } from "./MapViewport";
import { MapWidget } from "./MapWidget";

const styleUrl = "https://tiles.openfreemap.org/styles/bright";
const bounds: BoundingBox = [7.995, 48.995, 8.045, 49.025];

const meta = {
  title: "Components/Map/Library Routes",
  parameters: liveMap,
  component: LibraryRoutes,
  tags: ["autodocs"],
  decorators: [
    (Story) => (
      <StoryProviders>
        <div className="h-[34rem] overflow-hidden rounded-xl">
          <Story />
        </div>
      </StoryProviders>
    ),
  ],
} satisfies Meta<typeof LibraryRoutes>;

export default meta;
type Story = StoryObj<typeof meta>;

/** Library lines layered over the base map without page controls or route panels. */
export const Lines: Story = {
  args: { lines: [], selectedKey: null },
  render: () => (
    <MapWidget styleUrl={styleUrl} ariaLabel="Map with route-library lines">
      <MapViewport bounds={bounds} maxZoom={14} />
      <LibraryRoutes
        lines={[
          { key: "alpine-loop", coordinates },
          {
            key: "valley-loop",
            coordinates: coordinates.map(([longitude, latitude, elevation]) => [
              longitude + 0.002,
              latitude - 0.008,
              elevation,
            ]),
          },
        ]}
        selectedKey={null}
      />
    </MapWidget>
  ),
};
