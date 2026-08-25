import type { Meta, StoryObj } from "@storybook/react-vite";
import { MemoryRouter } from "react-router";
import { Button, ButtonLink, ExternalButtonLink } from "./Button";

const meta = {
  title: "Components/Button",
  component: Button,
  tags: ["autodocs"],
  decorators: [
    (Story) => (
      <MemoryRouter>
        <Story />
      </MemoryRouter>
    ),
  ],
  args: { children: "Reprocess" },
} satisfies Meta<typeof Button>;

export default meta;
type Story = StoryObj<typeof meta>;

export const Standard: Story = {};

export const Primary: Story = {
  args: { variant: "primary", children: "Run now" },
};

export const Disabled: Story = {
  args: { disabled: true, children: "Requesting…" },
};

export const Link: Story = {
  render: () => <ButtonLink to="/routes/12/2">Open route</ButtonLink>,
};

export const ExternalLink: Story = {
  render: () => <ExternalButtonLink href="https://example.com">Open provider</ExternalButtonLink>,
};
