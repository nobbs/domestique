import type { Meta, StoryObj } from "@storybook/react-vite";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { expect } from "storybook/test";
import { statusQuery } from "../../api/queries";
import { StoryProviders, StubbedFetch, status } from "../../storybook/fixtures";
import { BuildLine } from "./BuildLine";

const meta = {
  title: "Features/Sync/Build Line",
  component: BuildLine,
  tags: ["autodocs"],
  decorators: [
    (Story) => (
      <div className="bg-[var(--base)] p-4">
        <Story />
      </div>
    ),
  ],
} satisfies Meta<typeof BuildLine>;

export default meta;
type Story = StoryObj<typeof meta>;

/** The deployed build the fixture status names a revision for. */
export const DeployedBuild: Story = {
  decorators: [
    (Story) => (
      <StoryProviders>
        <Story />
      </StoryProviders>
    ),
  ],
  play: async ({ canvas }) => {
    await expect(canvas.getByRole("link", { name: /^commit /i })).toBeInTheDocument();
  },
};

/** No commit stamped in, which is every local build. */
export const DevelopmentBuild: Story = {
  decorators: [
    (Story) => {
      const client = new QueryClient({
        defaultOptions: { queries: { retry: false, staleTime: Number.POSITIVE_INFINITY } },
      });
      const { build: _build, ...withoutBuild } = status;
      client.setQueryData(statusQuery().queryKey, withoutBuild);

      return (
        <QueryClientProvider client={client}>
          <Story />
        </QueryClientProvider>
      );
    },
  ],
  play: async ({ canvas }) => {
    await expect(canvas.getByRole("link", { name: "a development build" })).toBeInTheDocument();
  },
};

/**
 * Not knowing yet is not the same as knowing there is no revision: naming a
 * development build on a deployed service, even for one frame, is the exact
 * wrong answer to the question this line exists to settle.
 */
export const Pending: Story = {
  decorators: [
    (Story) => {
      const client = new QueryClient({ defaultOptions: { queries: { retry: false } } });

      return (
        // A request that never answers, so the line is asked what it says while
        // it still does not know.
        <StubbedFetch respond={() => new Promise<Response>(() => {})}>
          <QueryClientProvider client={client}>
            <Story />
          </QueryClientProvider>
        </StubbedFetch>
      );
    },
  ],
  play: async ({ canvas }) => {
    await expect(canvas.queryByText(/^Running/)).not.toBeInTheDocument();
  },
};
