import type { Meta, StoryObj } from "@storybook/react-vite";
import { StoryProviders } from "../../storybook/fixtures";
import { SyncPage } from "./SyncPage";

const meta = { title: "Features/Sync page", tags: ["autodocs"] } satisfies Meta;

export default meta;
type Story = StoryObj<typeof meta>;

export const CurrentAndLaggingAccounts: Story = {
  render: () => (
    <StoryProviders>
      <SyncPage />
    </StoryProviders>
  ),
};
