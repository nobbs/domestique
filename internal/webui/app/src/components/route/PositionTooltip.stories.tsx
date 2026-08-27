import type { Meta, StoryObj } from "@storybook/react-vite";
import type { BoundingBox } from "../../api/types";
import type { ProfileSample } from "../../lib/profile";
import { sampleAt } from "../../lib/profile";
import { liveMap, profile as maybeProfile, surface } from "../../storybook/fixtures";
import { MapViewport } from "../map/MapViewport";
import { MapWidget } from "../map/MapWidget";
import { PositionTooltip } from "./PositionTooltip";

const styleUrl = "https://tiles.openfreemap.org/styles/bright";
const bounds: BoundingBox = [7.995, 48.995, 8.045, 49.025];

// The fixture coordinates always build a profile; narrowed once here so the
// stories below can read `endMetres` and pick samples without threading null.
if (!maybeProfile) {
  throw new Error("the storybook fixture coordinates should build a profile");
}
const profile = maybeProfile;

function sample(metres: number): ProfileSample {
  const found = sampleAt(profile, metres);
  if (!found) {
    throw new Error(`no sample at ${metres}m`);
  }

  return found;
}

const midway = sample(profile.endMetres / 2);

const meta = {
  title: "Components/Route/Position Tooltip",
  parameters: liveMap,
  component: PositionTooltip,
  tags: ["autodocs"],
  args: {
    position: midway,
    content: midway,
    endMetres: profile.endMetres,
    surfaceSummary: surface,
    announce: false,
    unitSystem: "metric",
  },
  decorators: [
    (Story) => (
      <div className="h-[34rem] overflow-hidden rounded-xl">
        <Story />
      </div>
    ),
  ],
} satisfies Meta<typeof PositionTooltip>;

export default meta;
type Story = StoryObj<typeof meta>;

function OnMap({ children }: { children: React.ReactNode }) {
  return (
    <MapWidget styleUrl={styleUrl}>
      <MapViewport bounds={bounds} maxZoom={14} />
      {children}
    </MapWidget>
  );
}

export const Default: Story = {
  render: (args) => (
    <OnMap>
      <PositionTooltip {...args} />
    </OnMap>
  ),
};

/**
 * The corner the box opens from follows the room it has, so the same tooltip
 * at either end of the route hangs off opposite sides of its dot.
 */
export const AtBothEnds: Story = {
  render: (args) => (
    <OnMap>
      <PositionTooltip {...args} position={sample(0)} content={sample(0)} />
      <PositionTooltip
        {...args}
        position={sample(profile.endMetres)}
        content={sample(profile.endMetres)}
      />
    </OnMap>
  ),
};

/** Imperial units, and no surface to name — the last line drops out entirely. */
export const ImperialWithoutSurface: Story = {
  render: (args) => (
    <OnMap>
      <PositionTooltip {...args} surfaceSummary={null} unitSystem="imperial" />
    </OnMap>
  ),
};

/**
 * Standing in for the profile readout while it is folded away, which is the
 * one case where this tooltip carries the `aria-live` announcement itself.
 */
export const Announcing: Story = {
  render: (args) => (
    <OnMap>
      <PositionTooltip {...args} announce={true} />
    </OnMap>
  ),
};
