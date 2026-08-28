import type { Meta, StoryObj } from "@storybook/react-vite";
import { useState } from "react";
import type { BoundingBox } from "../../api/types";
import { coordinates, liveMap, StoryProviders } from "../../storybook/fixtures";
import { LibraryMap } from "./LibraryMap";

const streets = {
  name: "Streets",
  styleUrl: "https://tiles.openfreemap.org/styles/bright",
  darkCartography: false,
};
const dark = {
  name: "Dark",
  styleUrl: "https://tiles.openfreemap.org/styles/dark",
  darkCartography: true,
};
const basemaps = [streets, dark];
const bounds: BoundingBox = [7.995, 48.995, 8.045, 49.025];

const meta = {
  title: "Features/Atlas/Map",
  parameters: liveMap,
  component: LibraryMap,
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
} satisfies Meta<typeof LibraryMap>;

export default meta;
type Story = StoryObj<typeof meta>;

/** The assembled route-library map, over the same live style as the demo. */
export const Library: Story = {
  args: { styleUrl: streets.styleUrl, lines: [], pickedKey: null, bounds },
  render: () => {
    const [pickedKey, setPickedKey] = useState<string | null>(null);
    const [basemap, setBasemap] = useState(streets);

    return (
      <LibraryMap
        styleUrl={basemap.styleUrl}
        darkBasemap={basemap.darkCartography}
        basemaps={basemaps}
        selectedBasemap={basemap.name}
        onBasemapChange={(name) =>
          setBasemap(basemaps.find((entry) => entry.name === name) ?? streets)
        }
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
        pickedKey={pickedKey}
        bounds={bounds}
        onPick={setPickedKey}
      />
    );
  },
};
