import type { Meta, StoryObj } from "@storybook/react-vite";
import { StoryProviders } from "../storybook/fixtures";
import { Layout, PageShell } from "./Layout";

const meta = {
  title: "Components/Layout",
  component: Layout,
  tags: ["autodocs"],
  // Both layouts carry the menu bar, which asks the service what sync is
  // doing — so each needs a client and a router the same way a page mounted
  // inside it does.
  decorators: [
    (Story) => (
      <StoryProviders>
        <Story />
      </StoryProviders>
    ),
  ],
} satisfies Meta<typeof Layout>;

export default meta;
type Story = StoryObj<typeof meta>;

export const Workspace: Story = {
  args: {
    map: <div className="h-full bg-[linear-gradient(135deg,var(--base),var(--ground))]" />,
    children: (
      <div className="w-80 rounded-xl bg-[var(--panel)] p-4 text-sm shadow-[var(--shadow)]">
        Route library controls
      </div>
    ),
  },
};

/** The other layout this module exports: a readable page, outside the map workspace. */
export const Shell: Story = {
  render: () => (
    <PageShell>
      <p className="text-sm">A readable page, outside the map workspace.</p>
    </PageShell>
  ),
};
