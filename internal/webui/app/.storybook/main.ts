import type { StorybookConfig } from "@storybook/react-vite";

const config: StorybookConfig = {
  stories: ["../src/**/*.stories.@(ts|tsx)"],
  // Every story is tagged `autodocs`. Without this addon that tag produced
  // nothing at all: the index carried 118 stories and no docs entry, so the
  // page each component's stories were being written for did not exist.
  addons: [
    "@storybook/addon-a11y",
    "@storybook/addon-docs",
    "@storybook/addon-mcp",
    "@storybook/addon-vitest",
  ],
  framework: "@storybook/react-vite",
};

export default config;
