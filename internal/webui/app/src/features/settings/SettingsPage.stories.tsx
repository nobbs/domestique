import type { Meta, StoryObj } from "@storybook/react-vite";
import { useState } from "react";
import { expect, fn, userEvent } from "storybook/test";
import type { ThemeChoice } from "../../lib/theme";
import { StoryProviders, stubGlobal } from "../../storybook/fixtures";
import { SettingsPage } from "./SettingsPage";

/** A `localStorage` this story's own tab can read back, isolated from the real one. */
function stubStorage(): { entries: Map<string, string>; restore: () => void } {
  const entries = new Map<string, string>();
  const restore = stubGlobal("localStorage", {
    getItem: (key: string) => entries.get(key) ?? null,
    setItem: (key: string, value: string) => entries.set(key, value),
  });

  return { entries, restore };
}

function Page({
  onThemeChoiceChange = fn(),
}: {
  onThemeChoiceChange?: (choice: ThemeChoice) => void;
}) {
  const [theme, setTheme] = useState<ThemeChoice>("system");

  return (
    <StoryProviders>
      <SettingsPage
        themeChoice={theme}
        onThemeChoiceChange={(choice) => {
          setTheme(choice);
          onThemeChoiceChange(choice);
        }}
      />
    </StoryProviders>
  );
}

const meta = {
  title: "Features/Settings page",
  component: Page,
  tags: ["autodocs"],
  args: { onThemeChoiceChange: fn() },
} satisfies Meta<typeof Page>;

export default meta;
type Story = StoryObj<typeof meta>;

export const Preferences: Story = {};

export const SetsTheUnitPreference: Story = {
  play: async ({ canvas }) => {
    const storage = stubStorage();
    try {
      await userEvent.click(canvas.getByRole("radio", { name: "Imperial (mi)" }));

      await expect(storage.entries.get("domestique.units")).toBe("imperial");
    } finally {
      storage.restore();
    }
  },
};

export const PassesTheChosenThemeBack: Story = {
  play: async ({ canvas, args }) => {
    await userEvent.click(canvas.getByRole("radio", { name: "Dark" }));

    await expect(args.onThemeChoiceChange).toHaveBeenCalledWith("dark");
  },
};

export const ReturnsToTheMap: Story = {
  play: async ({ canvas }) => {
    await expect(canvas.getByRole("link", { name: "Map" })).toHaveAttribute("href", "/");
  },
};
