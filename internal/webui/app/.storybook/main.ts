import type { StorybookConfig } from "@storybook/react-vite";

const config: StorybookConfig = {
  stories: ["../src/**/*.stories.@(ts|tsx)"],
  // Every story is tagged `autodocs`. Without this addon that tag produced
  // nothing at all: the index carried 118 stories and no docs entry, so the
  // page each component's stories were being written for did not exist.
  addons: [
    // Visual tests, which is the one question the suites here cannot answer.
    // `addon-vitest` plays every story and asserts what it was told to assert,
    // and the sweep proves each page renders at all; neither notices a
    // component that renders perfectly and looks wrong. Chromatic compares a
    // story's pixels against the last accepted ones, which is a claim about
    // change rather than about correctness — so it wants a baseline stored
    // somewhere across runs, and that somewhere is Chromatic's own service.
    //
    // Inert until a project is linked: with no token the panel offers to set
    // one up and nothing is uploaded. `storybook build`, the Vitest project
    // and the sweep are unaffected either way, which is what keeps this
    // addition free for anyone who never signs in.
    "@chromatic-com/storybook",
    "@storybook/addon-a11y",
    "@storybook/addon-docs",
    "@storybook/addon-mcp",
    "@storybook/addon-vitest",
  ],
  framework: "@storybook/react-vite",
};

export default config;
