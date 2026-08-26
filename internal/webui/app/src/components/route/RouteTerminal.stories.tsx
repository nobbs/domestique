import type { Meta, StoryObj } from "@storybook/react-vite";
import type { BoundingBox } from "../../api/types";
import { coordinates } from "../../storybook/fixtures";
import { MapViewport } from "../map/MapViewport";
import { MapWidget } from "../map/MapWidget";
import { RouteTerminal } from "./RouteTerminal";

const styleUrl = "https://tiles.openfreemap.org/styles/bright";
const bounds: BoundingBox = [7.995, 48.995, 8.045, 49.025];
const accent = "#236fc7";
const start = coordinates[0] ?? [8, 49, 100];
const finish = coordinates.at(-1) ?? [8.039, 49.0195, 295];

const meta = {
  title: "Components/Route/Route Terminal",
  component: RouteTerminal,
  tags: ["autodocs"],
  args: { kind: "start", position: start, offset: 0, accent },
  decorators: [
    (Story) => (
      <div className="h-[34rem] overflow-hidden rounded-xl">
        <Story />
      </div>
    ),
  ],
} satisfies Meta<typeof RouteTerminal>;

export default meta;
type Story = StoryObj<typeof meta>;

/** Both pictograms at the ends they mark, which is how they are ever seen. */
export const Default: Story = {
  render: () => (
    <MapWidget styleUrl={styleUrl}>
      <MapViewport bounds={bounds} maxZoom={14} />
      <RouteTerminal kind="start" position={start} offset={0} accent={accent} />
      <RouteTerminal kind="finish" position={finish} offset={0} accent={accent} />
    </MapWidget>
  ),
};

/**
 * A loop, where both ends sit on one point and the opposite nudges are the
 * only thing keeping the two markers apart.
 */
export const SharedTerminal: Story = {
  render: () => (
    <MapWidget styleUrl={styleUrl}>
      <MapViewport bounds={bounds} maxZoom={14} />
      <RouteTerminal kind="start" position={start} offset={-16} accent={accent} />
      <RouteTerminal kind="finish" position={start} offset={16} accent={accent} />
    </MapWidget>
  ),
};
