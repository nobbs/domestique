import type { Meta, StoryObj } from "@storybook/react-vite";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { MemoryRouter } from "react-router";
import { expect, userEvent, within } from "storybook/test";
import { StoryProviders, StubbedFetch } from "../../../storybook/fixtures";
import { TaskRunFeed } from "./TaskRunFeed";

const meta = {
  title: "Features/Settings/Tasks/Task Run Feed",
  component: TaskRunFeed,
  tags: ["autodocs"],
} satisfies Meta<typeof TaskRunFeed>;

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
    const list = within(canvas.getByRole("list"));
    await expect(list.getByText(/sync:source/)).toBeInTheDocument();
    await expect(list.getByText(/sync:clear/)).toBeInTheDocument();
    // A skipped attempt's busy detail reads in words, not the wire category.
    await expect(list.getByText(/Something it needs was held by another run/)).toBeInTheDocument();
  },
};

/** The run history is still on its way. */
export const Pending: Story = {
  decorators: [
    (Story) => {
      const client = new QueryClient({ defaultOptions: { queries: { retry: false } } });

      return (
        <StubbedFetch respond={() => new Promise<Response>(() => {})}>
          <QueryClientProvider client={client}>
            <MemoryRouter>
              <Story />
            </MemoryRouter>
          </QueryClientProvider>
        </StubbedFetch>
      );
    },
  ],
  play: async ({ canvas }) => {
    await expect(canvas.getByLabelText("Loading task history")).toBeInTheDocument();
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
            <MemoryRouter>
              <Story />
            </MemoryRouter>
          </QueryClientProvider>
        </StubbedFetch>
      );
    },
  ],
  play: async ({ canvas }) => {
    await expect(
      await canvas.findByText("The service did not say what has happened."),
    ).toBeInTheDocument();
  },
};

/** Choosing a task in the filter carries it into the address, so the view is linkable. */
export const FiltersToOneTask: Story = {
  decorators: [
    (Story) => (
      <StoryProviders>
        <Story />
      </StoryProviders>
    ),
  ],
  play: async ({ canvas }) => {
    await userEvent.selectOptions(canvas.getByRole("combobox", { name: "Task" }), "sync:clear");

    await expect(canvas.getByRole("combobox", { name: "Task" })).toHaveValue("sync:clear");
  },
};
