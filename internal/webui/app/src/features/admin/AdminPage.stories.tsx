import type { Meta, StoryObj } from "@storybook/react-vite";
import { Route, Routes } from "react-router";
import { StoryProviders } from "../../storybook/fixtures";
import { AdminPage } from "./AdminPage";

function Page({ path }: { path: string }) {
  return (
    <StoryProviders path={path}>
      <Routes>
        <Route path="admin/:section" element={<AdminPage />} />
      </Routes>
    </StoryProviders>
  );
}

const meta = {
  title: "Features/Admin page",
  component: Page,
  tags: ["autodocs"],
  parameters: { layout: "fullscreen" },
} satisfies Meta<typeof Page>;

export default meta;
type Story = StoryObj<typeof meta>;

export const Service: Story = { args: { path: "/admin/service" } };
export const Integrations: Story = { args: { path: "/admin/integrations" } };
export const Alerting: Story = { args: { path: "/admin/alerting" } };
export const MapTab: Story = { args: { path: "/admin/map" } };
export const Tasks: Story = { args: { path: "/admin/tasks" } };
