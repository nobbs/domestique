import type { Meta, StoryObj } from "@storybook/react-vite";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { expect } from "storybook/test";
import { StoryProviders, StubbedFetch } from "../../storybook/fixtures";
import { SyncCard } from "./SyncCard";
import { SyncControls } from "./SyncControls";

const meta = {
  title: "Features/Sync/Sync Controls",
  component: SyncControls,
  tags: ["autodocs"],
  decorators: [
    (Story) => (
      <SyncCard id="now" heading="Now">
        <Story />
      </SyncCard>
    ),
  ],
} satisfies Meta<typeof SyncControls>;

export default meta;
type Story = StoryObj<typeof meta>;

export const Default: Story = {
  decorators: [
    (Story) => (
      <StoryProviders>
        <Story />
      </StoryProviders>
    ),
  ],
  play: async ({ canvas }) => {
    await expect(canvas.getByText(/^Nothing is running/)).toBeInTheDocument();
  },
};

/** Both queries the card reads are still on their way. */
export const Pending: Story = {
  decorators: [
    (Story) => {
      const client = new QueryClient({ defaultOptions: { queries: { retry: false } } });

      return (
        <StubbedFetch respond={() => new Promise<Response>(() => {})}>
          <QueryClientProvider client={client}>
            <Story />
          </QueryClientProvider>
        </StubbedFetch>
      );
    },
  ],
  play: async ({ canvas }) => {
    await expect(canvas.getByLabelText("Loading sync controls")).toBeInTheDocument();
  },
};

/** The status query itself failed, not one of the actions on it. */
export const Failed: Story = {
  decorators: [
    (Story) => {
      const client = new QueryClient({ defaultOptions: { queries: { retry: false } } });

      return (
        <StubbedFetch
          respond={async () =>
            new Response(
              JSON.stringify({ error: { code: "unavailable", message: "the service is down" } }),
              { status: 500 },
            )
          }
        >
          <QueryClientProvider client={client}>
            <Story />
          </QueryClientProvider>
        </StubbedFetch>
      );
    },
  ],
  play: async ({ canvas }) => {
    await expect(
      await canvas.findByText("The service did not say what it is doing."),
    ).toBeInTheDocument();
  },
};
