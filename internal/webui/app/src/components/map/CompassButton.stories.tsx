import type { Meta, StoryObj } from "@storybook/react-vite";
import { ChromeMap } from "../../storybook/mapMock";
import { Button } from "../Button";
import { CompassButton, CompassIcon } from "./CompassButton";
import { MapWidget } from "./MapWidget";

const styleUrl = "https://tiles.openfreemap.org/styles/bright";

const meta = {
  title: "Components/Map/Compass Button",
  component: CompassButton,
  tags: ["autodocs"],
} satisfies Meta<typeof CompassButton>;

export default meta;
type Story = StoryObj<typeof meta>;

export const Default: Story = {
  decorators: [
    (Story) => (
      <ChromeMap>
        <div className="h-[34rem] overflow-hidden rounded-xl">
          <Story />
        </div>
      </ChromeMap>
    ),
  ],
  render: () => (
    <MapWidget styleUrl={styleUrl}>
      <CompassButton />
    </MapWidget>
  ),
};

/** The needle across bearings, and flattened by pitch in the last two poses. */
export const Poses: Story = {
  render: () => (
    <div className="flex items-center gap-4 p-6">
      {[
        { bearing: 0, pitch: 0 },
        { bearing: 45, pitch: 0 },
        { bearing: 135, pitch: 0 },
        { bearing: 220, pitch: 0 },
        { bearing: 300, pitch: 0 },
        { bearing: 0, pitch: 60 },
        { bearing: 45, pitch: 60 },
      ].map(({ bearing, pitch }) => (
        <Button
          key={`${bearing}-${pitch}`}
          variant="panel"
          icon={<CompassIcon bearing={bearing} pitch={pitch} />}
          aria-label={`Compass at bearing ${bearing}, pitch ${pitch}`}
          title={`${bearing}° / pitch ${pitch}°`}
        />
      ))}
    </div>
  ),
};
