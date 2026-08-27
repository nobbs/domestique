import type { Meta, StoryObj } from "@storybook/react-vite";
import { StoryProviders } from "../storybook/fixtures";
import { PageShell } from "./Layout";

const meta = {
  title: "Components/Page Shell",
  component: PageShell,
  tags: ["autodocs"],
  // The shell carries the menu bar, which asks the service what synchronisation
  // is doing — so the shell needs a client and a router the same way a page
  // mounted inside it does.
  decorators: [
    (Story) => (
      <StoryProviders>
        <Story />
      </StoryProviders>
    ),
  ],
} satisfies Meta<typeof PageShell>;

export default meta;
type Story = StoryObj<typeof meta>;

export const Default: Story = {
  args: { children: <p className="text-sm">A readable page, outside the map workspace.</p> },
};
