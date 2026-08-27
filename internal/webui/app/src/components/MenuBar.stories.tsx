import type { Meta, StoryObj } from "@storybook/react-vite";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { expect } from "storybook/test";
import { statusQuery } from "../api/queries";
import type { Status, TargetStatus } from "../api/types";
import { StoryProviders } from "../storybook/fixtures";
import { MenuBar } from "./MenuBar";

const meta = {
  title: "Components/MenuBar",
  component: MenuBar,
  tags: ["autodocs"],
  decorators: [
    (Story) => (
      <StoryProviders>
        <div className="bg-[var(--base)]">
          <Story />
        </div>
      </StoryProviders>
    ),
  ],
} satisfies Meta<typeof MenuBar>;

export default meta;
type Story = StoryObj<typeof meta>;

export const Ready: Story = {};

function unauthorized(): TargetStatus {
  return {
    id: "rider-b",
    authorisation: "not_authorized",
    convergence: "unauthorized",
    stages: { current: 0, pending: 4 },
  };
}

/**
 * A status the shared `StoryProviders` fixture does not carry — each of these
 * three stories overrides the query cache with the exact shape its own
 * assertion needs. What the status is *read* as has its own unit suite in
 * `lib/syncState.test.ts`; what these demonstrate is how the bar paints it.
 *
 * It nests its own `QueryClientProvider` inside the meta-level `StoryProviders`
 * decorator rather than trying to replace that decorator, which a per-story
 * `decorators` array cannot do. `useQuery` reads whichever provider is
 * closest, so the fixture's client — still mounted one level out — is simply
 * shadowed for the query this story cares about. It carries no `MemoryRouter`
 * of its own for the same reason: `StoryProviders` already mounted one, and
 * react-router refuses to render a second one nested inside it.
 */
function withStatus(value?: Status): NonNullable<Meta<typeof MenuBar>["decorators"]> {
  return [
    (Story) => {
      // `enabled: false` because every story here seeds what it wants read.
      // Without it the story that seeds nothing — the state before an answer
      // has arrived — is the one story that reaches for the network, and it
      // would be asking a Storybook that serves no API.
      const client = new QueryClient({
        defaultOptions: {
          queries: { enabled: false, retry: false, staleTime: Number.POSITIVE_INFINITY },
        },
      });
      if (value) {
        client.setQueryData(statusQuery().queryKey, value);
      }

      return (
        <QueryClientProvider client={client}>
          <Story />
        </QueryClientProvider>
      );
    },
  ];
}

/** The three destinations, and the way to the sync page among them. */
export const LinksToSync: Story = {
  decorators: withStatus({
    ready: true,
    converged: true,
    targets: [],
    sync: {
      state: "idle",
      lastCompletedAt: "2026-08-18T06:30:00Z",
      sourceStages: 0,
      created: 0,
      updated: 0,
      deleted: 0,
      schedule: { source: true, targets: true },
      phases: {},
      surface: { classified: 0, total: 0, incomplete: 0 },
    },
  }),
  play: async ({ canvas }) => {
    const link = canvas.getByRole("link", { name: /^Sync/ });
    await expect(link).toHaveAttribute("href", "/sync");
    await expect(link).toHaveAccessibleName(/^Sync/);
    await expect(canvas.getByRole("link", { name: "Map" })).toHaveAttribute("href", "/");
    await expect(canvas.getByRole("link", { name: "Settings" })).toHaveAttribute(
      "href",
      "/settings",
    );
    await expect(canvas.getByText("domestique")).toBeInTheDocument();
  },
};

/**
 * The dot is the state and nothing else — the link's own colour is reserved for
 * saying which page is being read. A dot is nothing to a screen reader or to
 * anyone who cannot tell these two apart, so the name says what it meant.
 */
export const UnauthorizedAccount: Story = {
  decorators: withStatus({
    ready: true,
    converged: true,
    targets: [unauthorized()],
    sync: {
      state: "idle",
      lastCompletedAt: "2026-08-18T06:30:00Z",
      sourceStages: 0,
      created: 0,
      updated: 0,
      deleted: 0,
      schedule: { source: true, targets: true },
      phases: {},
      surface: { classified: 0, total: 0, incomplete: 0 },
    },
  }),
  play: async ({ canvas }) => {
    const link = canvas.getByRole("link", { name: "Sync · An account is not connected" });
    await expect(link).toHaveAttribute("data-tone", "alert");
  },
};

/**
 * A status request still in flight — or one that never arrives — must not paint
 * the bar in a state nobody has.
 */
export const StatusNotYetKnown: Story = {
  decorators: withStatus(),
  play: async ({ canvas }) => {
    const link = canvas.getByRole("link", { name: "Sync" });
    await expect(link).not.toHaveAttribute("data-tone");
    await expect(link).not.toHaveAttribute("title");
  },
};
