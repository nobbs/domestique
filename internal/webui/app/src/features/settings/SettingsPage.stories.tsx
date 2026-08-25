import type { Meta, StoryObj } from "@storybook/react-vite";
import { useState } from "react";
import type { ThemeChoice } from "../../lib/theme";
import { StoryProviders } from "../../storybook/fixtures";
import { SettingsPage } from "./SettingsPage";

const meta = { title: "Features/Settings page", tags: ["autodocs"] } satisfies Meta;

export default meta;
type Story = StoryObj<typeof meta>;

export const Preferences: Story = {
  render: () => {
    const [theme, setTheme] = useState<ThemeChoice>("system");

    return (
      <StoryProviders>
        <SettingsPage themeChoice={theme} onThemeChoiceChange={setTheme} />
      </StoryProviders>
    );
  },
};
