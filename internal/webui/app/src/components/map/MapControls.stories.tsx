import type { Meta, StoryObj } from "@storybook/react-vite";
import { expect, spyOn, userEvent } from "storybook/test";
import { MapControls } from "./MapControls";
import { MapWidget } from "./MapWidget";

const styleUrl = "https://tiles.openfreemap.org/styles/bright";

const meta = {
  title: "Components/Map/Map Controls",
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

/**
 * The "you are here" pin, which only exists once a location has been found —
 * so the story has to answer for the platform before it can show the marker.
 *
 * Spied on rather than stubbed whole: `userEvent` reads other things off
 * `navigator`, and replacing the object outright takes those with it. The spy
 * comes from `storybook/test` rather than `vitest`, which has no runtime to
 * attach to when the story is opened in the Storybook dev server.
 */
export const Located: Story = {
  render: () => (
    <MapWidget styleUrl={styleUrl}>
      <MapControls />
    </MapWidget>
  ),
  play: async ({ canvas }) => {
    const found = spyOn(navigator.geolocation, "getCurrentPosition").mockImplementation(
      (onFound) => {
        onFound({ coords: { latitude: 49.01, longitude: 8.02 } } as GeolocationPosition);
      },
    );
    try {
      // Awaited: `MapWidget` holds its children back until the style has
      // loaded, so the button is not in the document when `play` first runs.
      await userEvent.click(await canvas.findByRole("button", { name: "Find my location" }));

      await expect(await canvas.findByRole("img", { name: "Your location" })).toBeInTheDocument();
    } finally {
      found.mockRestore();
    }
  },
};
