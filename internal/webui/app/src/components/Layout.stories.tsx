import type { Meta, StoryObj } from "@storybook/react-vite";
import { useQueryClient } from "@tanstack/react-query";
import { type ReactNode, useState } from "react";
import { webUIConfigQuery } from "../api/queries";
import type { WebUIConfig } from "../api/types";
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

/** Marks the fixture's session as an admin viewing as a rider, before anything reads it. */
function AsImpersonated({ children }: { children: ReactNode }) {
  const client = useQueryClient();
  useState(() =>
    client.setQueryData<WebUIConfig>(webUIConfigQuery().queryKey, (config) =>
      config
        ? {
            ...config,
            identity: { display: "nina@example.test", admin: false, impersonating: true },
          }
        : config,
    ),
  );

  return children;
}

/** Either layout, while an admin views the service as a rider: a banner under the bar. */
export const Impersonating: Story = {
  render: () => (
    <AsImpersonated>
      <PageShell>
        <p className="text-sm">A readable page, seen as another rider.</p>
      </PageShell>
    </AsImpersonated>
  ),
};

export const ImpersonatingWorkspace: Story = {
  args: { ...Workspace.args },
  render: (args) => (
    <AsImpersonated>
      <Layout {...args} />
    </AsImpersonated>
  ),
};
