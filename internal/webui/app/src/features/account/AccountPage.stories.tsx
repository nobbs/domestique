import type { Meta, StoryObj } from "@storybook/react-vite";
import { Route, Routes } from "react-router";
import { StoryProviders } from "../../storybook/fixtures";
import { AccountPage } from "./AccountPage";

function Page({ path }: { path: string }) {
  return (
    <StoryProviders path={path}>
      <Routes>
        <Route path="account/:section" element={<AccountPage />} />
      </Routes>
    </StoryProviders>
  );
}

const meta = {
  title: "Features/Account page",
  component: Page,
  tags: ["autodocs"],
  parameters: { layout: "fullscreen" },
} satisfies Meta<typeof Page>;

export default meta;
type Story = StoryObj<typeof meta>;

export const Sync: Story = { args: { path: "/account/sync" } };
export const Accounts: Story = { args: { path: "/account/accounts" } };
export const RiderProfile: Story = { args: { path: "/account/profile" } };
export const DataSources: Story = { args: { path: "/account/sources" } };
