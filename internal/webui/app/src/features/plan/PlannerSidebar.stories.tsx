import type { Meta, StoryObj } from "@storybook/react-vite";
import { MemoryRouter } from "react-router";
import { PlannerSidebar } from "./PlanPage";
import { initialPlannerState } from "./planner";

const meta = {
  title: "Features/Planner/Sidebar",
  component: PlannerSidebar,
  decorators: [
    (Story) => (
      <MemoryRouter>
        <div className="max-w-sm p-4">
          <Story />
        </div>
      </MemoryRouter>
    ),
  ],
  args: {
    state: initialPlannerState,
    plans: [],
    preview: null,
    planId: null,
    published: false,
    saving: false,
    saveError: null,
    onSave: () => {},
    dispatch: () => {},
  },
} satisfies Meta<typeof PlannerSidebar>;

export default meta;
type Story = StoryObj<typeof meta>;

export const NewPlan: Story = { args: { saveError: null } };

export const DraftList: Story = {
  args: {
    state: {
      ...initialPlannerState,
      name: "Saturday gravel",
      waypoints: [
        { id: 0, longitude: 8, latitude: 49 },
        { id: 1, longitude: 8.1, latitude: 49.1 },
      ],
    },
    plans: [
      {
        id: 4,
        name: "Saturday gravel",
        profile: "gravel",
        published: false,
        version: 2,
        distanceMetres: 32_000,
        ascentMetres: 510,
        waypointCount: 2,
        updatedAt: "2026-09-15T09:00:00Z",
      },
      {
        id: 5,
        name: "Weekday loop",
        profile: "fastbike",
        published: true,
        version: 1,
        distanceMetres: 18_000,
        ascentMetres: 120,
        waypointCount: 3,
        updatedAt: "2026-09-15T09:00:00Z",
      },
    ],
    planId: 4,
    saveError: null,
  },
};
