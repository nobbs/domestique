import type { Meta, StoryObj } from "@storybook/react-vite";
import type { ActivityTrackWorld, Position } from "../../api/types";
import { buildActivityProfile } from "../../lib/profile";
import { ZwiftWorldMap } from "./ZwiftWorldMap";

/**
 * Stand-in artwork, drawn here rather than fetched: the real image comes from
 * Zwift's CDN through this service, and nothing in a story reaches either.
 */
const ARTWORK = `data:image/svg+xml,${encodeURIComponent(
  `<svg xmlns="http://www.w3.org/2000/svg" width="800" height="500" viewBox="0 0 800 500">
    <rect width="800" height="500" fill="#1d4f5c"/>
    <path d="M60 380 C 200 120, 380 460, 520 200 S 760 160, 760 120" fill="none"
      stroke="#2f7f6f" stroke-width="26"/>
    <circle cx="180" cy="150" r="60" fill="#2f7f6f"/>
    <circle cx="640" cy="380" r="90" fill="#2f7f6f"/>
  </svg>`,
)}`;

const WORLD: ActivityTrackWorld = {
  id: 9,
  name: "Makuri Islands",
  mapUrl: ARTWORK,
  bounds: { north: -10.73746, west: 165.76591, south: -10.85234, east: 165.88222 },
};

/** A lap around the middle of the world, with the ground rising over it. */
const COORDINATES: Position[] = Array.from({ length: 120 }, (_, step) => {
  const angle = (step / 119) * 2 * Math.PI;
  const { north, west, south, east } = WORLD.bounds;

  return [
    (west + east) / 2 + ((east - west) / 3) * Math.cos(angle),
    (north + south) / 2 + ((north - south) / 3) * Math.sin(angle),
    120 + 60 * Math.sin(angle * 2),
  ] as Position;
});

const meta = {
  title: "Features/Activity/Zwift World Map",
  component: ZwiftWorldMap,
  args: {
    world: WORLD,
    coordinates: COORDINATES,
    profile: buildActivityProfile(COORDINATES),
    activeMetres: null,
    onActiveChange: () => {},
    expanded: false,
    onExpandedChange: () => {},
  },
  decorators: [
    (Story) => (
      <div className="h-96 w-full max-w-3xl overflow-hidden rounded-2xl ring-1 ring-black/5">
        <Story />
      </div>
    ),
  ],
} satisfies Meta<typeof ZwiftWorldMap>;

export default meta;
type Story = StoryObj<typeof meta>;

/** The ride as the page opens it: the whole lap, no cursor anywhere. */
export const Ridden: Story = {};

/** The shared cursor a quarter of the way round, as a chart hover sets it. */
export const Cursored: Story = {
  args: { activeMetres: (buildActivityProfile(COORDINATES)?.totalDistanceMetres ?? 0) / 4 },
};
