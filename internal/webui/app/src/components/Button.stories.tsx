import type { Meta, StoryObj } from "@storybook/react-vite";
import {
  IconAdjustmentsHorizontal,
  IconAlertTriangle,
  IconRefresh,
  IconSearch,
  IconTrash,
  IconX,
} from "@tabler/icons-react";
import { MemoryRouter } from "react-router";
import { expect, fn, userEvent } from "storybook/test";
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
  args: { children: "Reprocess", onClick: fn() },
} satisfies Meta<typeof Button>;

export default meta;
type Story = StoryObj<typeof meta>;

export const Standard: Story = {};

export const Primary: Story = {
  args: { variant: "primary", children: "Run now" },
};

/** A glyph and a label together: the label is the children, the mark is `icon`. */
export const IconAndText: Story = {
  args: { icon: <IconRefresh stroke={2} />, children: "Reprocess" },
};

/** The square with no label, on the ground the map cannot show through. */
export const Panel: Story = {
  args: {
    variant: "panel",
    icon: <IconSearch stroke={1.6} />,
    children: undefined,
    "aria-label": "Search the route library",
  },
  play: async ({ canvas, args }) => {
    await userEvent.click(canvas.getByRole("button", { name: "Search the route library" }));
    await expect(args.onClick).toHaveBeenCalled();
  },
};

/** The accent edge a trigger wears while the control it opens holds something. */
export const PanelActive: Story = {
  args: {
    variant: "panel",
    icon: <IconAdjustmentsHorizontal stroke={1.6} />,
    active: true,
    children: undefined,
    "aria-label": "Show the library filters — filters are active",
  },
};

/** Inside something that already has a ground: it shows itself by filling. */
export const Ghost: Story = {
  args: {
    variant: "ghost",
    icon: <IconX stroke={2} />,
    children: undefined,
    "aria-label": "Close the route",
  },
};

/** A press that destroys something. Tinted, not filled: it is not the happy path. */
export const Danger: Story = {
  args: { variant: "danger", icon: <IconTrash stroke={2} />, children: "Delete them" },
};

/** The waiting tone, for an action held up by something the reader has to settle. */
export const Warning: Story = {
  args: { variant: "warning", icon: <IconAlertTriangle stroke={2} />, children: "Reconnect first" },
};

export const Disabled: Story = {
  args: { disabled: true, children: "Requesting…" },
};

/** Unmarked unless its caller has a mark in mind, which this one has. */
export const Link: Story = {
  render: () => (
    <div className="flex flex-wrap items-center gap-2">
      <ButtonLink to="/routes/12/2">Open route</ButtonLink>
      <ButtonLink to="/sync" icon={<IconRefresh stroke={2} />}>
        Sync
      </ButtonLink>
    </div>
  ),
};

/** The outbound one marks itself, and `icon={null}` takes the mark away. */
export const ExternalLink: Story = {
  render: () => (
    <div className="flex flex-wrap items-center gap-2">
      <ExternalButtonLink href="https://example.com">Open provider</ExternalButtonLink>
      <ExternalButtonLink href="https://example.com" icon={null}>
        Unmarked
      </ExternalButtonLink>
    </div>
  ),
};
