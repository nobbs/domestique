import type { Meta, StoryObj } from "@storybook/react-vite";
import { expect } from "storybook/test";
import { StoryProviders } from "../../storybook/fixtures";
import { SettingsPage } from "./SettingsPage";

function Page() {
  return (
    <StoryProviders>
      <SettingsPage />
    </StoryProviders>
  );
}

const meta = {
  title: "Features/Settings page",
  component: Page,
  tags: ["autodocs"],
} satisfies Meta<typeof Page>;

export default meta;
type Story = StoryObj<typeof meta>;

export const Preferences: Story = {};

export const ReturnsToTheMap: Story = {
  play: async ({ canvas }) => {
    await expect(canvas.getByRole("link", { name: "Atlas" })).toHaveAttribute("href", "/");
  },
};
