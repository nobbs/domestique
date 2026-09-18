import type { Meta, StoryObj } from "@storybook/react-vite";
import { useQueryClient } from "@tanstack/react-query";
import type { ReactNode } from "react";
import { expect, userEvent } from "storybook/test";
import { getGetPlanQueryKey, getListPlansQueryKey } from "../../api/generated";
import { routeGeometryQuery, routesQuery, webUIConfigQuery } from "../../api/queries";
import { coordinates, route, routeGeometryFixture, StoryProviders } from "../../storybook/fixtures";
import { CataloguePage } from "./CataloguePage";

// No map: the catalogue is a ledger, and the
// geometry it fetches for the glyphs is already seeded by `StoryProviders`
// under the same keys the atlas caches it with.
//
// Every assertion below waits rather than reading once: a click or a keystroke
// settles on the machine's own schedule, and a `getBy` that reads a beat early
// fails the story for the machine it ran on rather than for anything the page did.
const meta = {
  title: "Features/Catalogue/Page",
  component: CataloguePage,
  tags: ["autodocs"],
  decorators: [
    (Story) => (
      <StoryProviders>
        <Story />
      </StoryProviders>
    ),
  ],
} satisfies Meta<typeof CataloguePage>;

export default meta;
type Story = StoryObj<typeof meta>;

/** The library as the reader first meets it: by name, ascending. */
export const Default: Story = {};

/** Ranked by a measurement, which is what the page is for. */
export const RankedByDistance: Story = {
  play: async ({ canvas }) => {
    await userEvent.click(await canvas.findByRole("button", { name: "Distance" }));

    await expect(
      await canvas.findByRole("button", { name: "Distance", pressed: true }),
    ).toBeInTheDocument();
  },
};

/** A search that matches nothing says which control narrowed it away. */
export const NothingMatches: Story = {
  play: async ({ canvas }) => {
    // No inter-key delay: every keystroke rewrites the address and re-renders
    // the table, and eight of those at the default cadence is a long wait.
    await userEvent.type(await canvas.findByRole("searchbox"), "montreal", { delay: null });

    await expect(await canvas.findByText("Nothing here is called that.")).toBeInTheDocument();
  },
};

const DRAFTS = [
  { id: 7, name: "Saturday gravel", hoursAgo: 3, waypoints: 6 },
  { id: 9, name: "Taunus long one", hoursAgo: 30, waypoints: 21 },
].map(({ id, name, hoursAgo, waypoints }) => ({
  id,
  name,
  profile: "gravel" as const,
  published: false,
  version: 1,
  distanceMetres: 40_000 + id * 1_000,
  ascentMetres: 300 + id * 20,
  waypointCount: waypoints,
  updatedAt: new Date(Date.now() - hoursAgo * 3_600_000).toISOString(),
}));

/** Signs the story in as an admin on a planning deployment, with a published plan and two drafts. */
function AsPlanner({ children }: { children: ReactNode }) {
  const client = useQueryClient();
  client.setQueryData(webUIConfigQuery().queryKey, (current) =>
    current
      ? { ...current, planning: true, identity: { display: "admin@example.test", admin: true } }
      : current,
  );
  client.setQueryData(routesQuery().queryKey, [
    route,
    { ...route, provider: "local", sourceRouteId: 8, title: "Weekday loop", contentHash: "plan-8" },
  ]);
  client.setQueryData(
    routeGeometryQuery("local", 8, route.stageOrder).queryKey,
    routeGeometryFixture,
  );
  client.setQueryData(getListPlansQueryKey(), { data: { plans: DRAFTS } });
  for (const draft of DRAFTS) {
    client.setQueryData(getGetPlanQueryKey(draft.id), {
      data: { ...draft, geometry: { type: "LineString", coordinates } },
    });
  }

  return children;
}

/** An admin: a pencil on the published plan, and a shelf switch to the drafts. */
export const AdminDrafts: Story = {
  decorators: [
    (Story) => (
      <AsPlanner>
        <Story />
      </AsPlanner>
    ),
  ],
  play: async ({ canvas }) => {
    await expect(
      await canvas.findByRole("link", { name: "Edit plan Weekday loop" }),
    ).toBeInTheDocument();
    await userEvent.click(await canvas.findByRole("button", { name: "Drafts · 2" }));
    await expect(await canvas.findByRole("link", { name: /Saturday gravel/ })).toBeInTheDocument();
  },
};
