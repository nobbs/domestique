import type { Meta, StoryObj } from "@storybook/react-vite";
import type { PlanRoutePreview } from "../../api/types";
import { StoryProviders } from "../../storybook/fixtures";
import { PlannerSidebar } from "./PlannerSidebar";
import { initialPlannerState, type PlannerWaypoint } from "./planner";

const meta = {
  title: "Features/Planner/Sidebar",
  component: PlannerSidebar,
  decorators: [
    (Story) => (
      <StoryProviders>
        <div className="flex h-[640px] w-[24rem] flex-col">
          <Story />
        </div>
      </StoryProviders>
    ),
  ],
  args: {
    state: initialPlannerState,
    preview: null,
    planId: null,
    published: false,
    changed: false,
    saving: false,
    saveError: null,
    onSave: () => {},
    dispatch: () => {},
  },
} satisfies Meta<typeof PlannerSidebar>;

export default meta;
type Story = StoryObj<typeof meta>;

function waypoints(count: number, straight: number[] = []): PlannerWaypoint[] {
  return Array.from({ length: count }, (_, id) => ({
    id,
    longitude: 8 + id * 0.02,
    latitude: 49 + (id % 3) * 0.01,
    ...(straight.includes(id) ? { straight: true } : {}),
  }));
}

function preview(count: number): PlanRoutePreview {
  return {
    geometry: { type: "LineString", coordinates: [] },
    distanceMetres: count * 4_200,
    ascentMetres: count * 35,
    movingSeconds: count * 720,
    waypointProgress: Array.from({ length: count }, (_, index) => ({
      distanceMetres: index * 4_200,
      movingSeconds: index * 720,
    })),
  };
}

export const NewPlan: Story = {};

export const Draft: Story = {
  args: {
    state: {
      ...initialPlannerState,
      name: "Saturday gravel",
      profile: "gravel",
      cues: true,
      waypoints: waypoints(4, [2]),
      avoid: [{ id: 0, longitude: 8.05, latitude: 49.05, radiusMetres: 250 }],
    },
    preview: preview(4),
    planId: 4,
    changed: true,
    turnCount: 12,
  },
};

export const LongListFolded: Story = {
  args: {
    state: { ...initialPlannerState, name: "Long loop", waypoints: waypoints(15, [6, 11]) },
    preview: preview(15),
    planId: 5,
    published: true,
    focusId: 8,
  },
};

export const SaveFailed: Story = {
  args: {
    ...Draft.args,
    saveError: "The plan changed since it was loaded.",
  },
};
