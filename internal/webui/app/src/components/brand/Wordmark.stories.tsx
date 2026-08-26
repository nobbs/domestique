import type { Meta, StoryObj } from "@storybook/react-vite";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { expect } from "storybook/test";
import { statusQuery } from "../../api/queries";
import type { Status, TargetStatus } from "../../api/types";
import { StoryProviders } from "../../storybook/fixtures";
import { Wordmark } from "./Wordmark";

const meta = {
  title: "Components/Brand/Wordmark",
  component: Wordmark,
  tags: ["autodocs"],
  decorators: [
    (Story) => (
      <StoryProviders>
        <div className="bg-[var(--base)] p-6">
          <Story />
        </div>
      </StoryProviders>
    ),
  ],
} satisfies Meta<typeof Wordmark>;

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
 * assertion needs, in place of `Wordmark.test.tsx`'s `renderWordmark` helper.
 *
 * It nests its own `QueryClientProvider` inside the meta-level `StoryProviders`
 * decorator rather than trying to replace that decorator, which a per-story
 * `decorators` array cannot do. `useQuery` reads whichever provider is
 * closest, so the fixture's client — still mounted one level out — is simply
 * shadowed for the query this story cares about. It carries no `MemoryRouter`
 * of its own for the same reason: `StoryProviders` already mounted one, and
 * react-router refuses to render a second one nested inside it.
 */
function withStatus(value?: Status): NonNullable<Meta<typeof Wordmark>["decorators"]> {
  return [
    (Story) => {
      const client = new QueryClient({
        defaultOptions: { queries: { retry: false, staleTime: Number.POSITIVE_INFINITY } },
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

/** One quiet way to the sync page. */
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
    // A mark rather than a word, so what says `Sync` is the name it is given.
    await expect(link).toHaveAccessibleName(/^Sync/);
    await expect(canvas.getByText("domestique")).toBeInTheDocument();
  },
};

/**
 * The row has room for one mark, so the state is the link's colour — and a
 * colour is nothing to a screen reader or to anyone who cannot tell these two
 * apart. The name says what the colour meant.
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
 * The map behind this panel is what the reader came for. A status request
 * still in flight — or one that never arrives — must not paint the corner of
 * it in a state nobody has.
 */
export const StatusNotYetKnown: Story = {
  decorators: withStatus(),
  play: async ({ canvas }) => {
    const link = canvas.getByRole("link", { name: "Sync" });
    await expect(link).not.toHaveAttribute("data-tone");
    await expect(link).not.toHaveAttribute("title");
  },
};
