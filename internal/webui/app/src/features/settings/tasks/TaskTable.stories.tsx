import type { Meta, StoryObj } from "@storybook/react-vite";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { expect, screen, userEvent } from "storybook/test";
import { StoryProviders, StubbedFetch } from "../../../storybook/fixtures";
import { TaskTable } from "./TaskTable";

const meta = {
  title: "Features/Settings/Tasks/Task Table",
  component: TaskTable,
  tags: ["autodocs"],
} satisfies Meta<typeof TaskTable>;

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
    await expect(canvas.getByText("sync:source")).toBeInTheDocument();
    await expect(canvas.getByText("Hourly")).toBeInTheDocument();
    // sync:clear, surface:annotate and ridemodel:predict are nothing this build schedules.
    await expect(canvas.getAllByText("On demand")).toHaveLength(3);
  },
};

/** The registered task list is still on its way. */
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
    await expect(canvas.getByLabelText("Loading tasks")).toBeInTheDocument();
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
      await canvas.findByText("The service did not say what it runs."),
    ).toBeInTheDocument();
  },
};

/**
 * `sync:clear` deletes every owned route from one target slot, and a run with
 * none named does nothing — so its Run control has to ask which slot before
 * it can ask to confirm, and stays disabled until both are given.
 */
export const ClearNeedsASlotAndConfirmation: Story = {
  decorators: [
    (Story) => (
      <StoryProviders>
        <Story />
      </StoryProviders>
    ),
  ],
  // The dialog renders through a portal into document.body, outside this
  // story's own canvas root — see TargetRow.stories.tsx for the same reason
  // `screen` is used here instead of `canvas` once it is open.
  play: async ({ canvas }) => {
    await userEvent.click(canvas.getByRole("button", { name: "Run now: sync:clear" }));

    const confirm = screen.getByRole("button", { name: "Delete them" });
    await expect(confirm).toBeDisabled();

    await userEvent.click(screen.getByRole("radio", { name: "rider-a" }));
    await expect(confirm).toBeDisabled();

    await userEvent.type(screen.getByLabelText(/Type/), "rider-a");
    await expect(confirm).not.toBeDisabled();
  },
};
