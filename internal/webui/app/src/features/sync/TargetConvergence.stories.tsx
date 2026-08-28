import type { Meta, StoryObj } from "@storybook/react-vite";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { expect } from "storybook/test";
import { StoryProviders, StubbedFetch } from "../../storybook/fixtures";
import { SyncCard } from "./SyncCard";
import { TargetConvergence } from "./TargetConvergence";

const meta = {
  title: "Features/Sync/Target Convergence",
  component: TargetConvergence,
  tags: ["autodocs"],
  decorators: [
    (Story) => (
      <SyncCard id="accounts" heading="What the accounts hold">
        <Story />
      </SyncCard>
    ),
  ],
} satisfies Meta<typeof TargetConvergence>;

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
    await expect(canvas.getByText(/what a head unit has downloaded/)).toBeInTheDocument();
  },
};

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
    await expect(canvas.getByLabelText("Loading accounts")).toBeInTheDocument();
  },
};

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
      await canvas.findByText("The service did not say what the accounts hold."),
    ).toBeInTheDocument();
  },
};
