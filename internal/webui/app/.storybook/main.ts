import type { StorybookConfig } from "@storybook/react-vite";

const config: StorybookConfig = {
  stories: ["../src/**/*.stories.@(ts|tsx)"],
  // Every story is tagged `autodocs`. Without this addon that tag produced
  // nothing at all: the index carried 118 stories and no docs entry, so the
  // page each component's stories were being written for did not exist.
  addons: [
    // Visual tests, the one question the suites here cannot answer: `addon-vitest`
    // plays every story and the sweep proves each page renders, but neither notices
    // a component that renders perfectly and looks wrong. Chromatic compares a
    // story's pixels against the last accepted ones, so it needs a baseline stored
    // across runs. Inert until a project is linked: with no token nothing uploads.
    "@chromatic-com/storybook",
    "@storybook/addon-docs",
    "@storybook/addon-mcp",
    "@storybook/addon-vitest",
  ],
  framework: "@storybook/react-vite",
};

export default config;
