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

export const Outline: Story = {
  play: async ({ canvas }) => {
    const button = canvas.getByRole("button", { name: "Reprocess" });
    // A shared control that could quietly become a submit button is a trap, so
    // the type is not among the props a call site can pass.
    await expect(button).toHaveAttribute("type", "button");
    // The weight `Default`'s own play compares itself against.
    await expect(button).toHaveClass("bg-background");
  },
};

export const Default: Story = {
  args: { variant: "default", children: "Run now" },
  play: async ({ canvas }) => {
    await expect(canvas.getByRole("button", { name: "Run now" })).toHaveClass("bg-primary");
  },
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
export const Destructive: Story = {
  args: { variant: "destructive", icon: <IconTrash stroke={2} />, children: "Delete them" },
};

/** The waiting tone, for an action held up by something the reader has to settle. */
export const Warning: Story = {
  args: { variant: "warning", icon: <IconAlertTriangle stroke={2} />, children: "Reconnect first" },
};

export const Disabled: Story = {
  args: { disabled: true, children: "Requesting…" },
  play: async ({ canvas }) => {
    // Disabled here only ever means "that request is already in flight", and
    // the whole point of it is that a second press cannot send a second one —
    // `pointer-events: none` is what a real browser enforces that for, which
    // is also why there is nothing left here to click and find not called.
    await expect(canvas.getByRole("button", { name: "Requesting…" })).toBeDisabled();
  },
};

/** A press with a class the feature placing it asked for, kept alongside the shared one. */
export const CustomClassName: Story = {
  args: { className: "border-2 border-dashed border-[var(--accent)]" },
  play: async ({ canvas }) => {
    await expect(canvas.getByRole("button", { name: "Reprocess" })).toHaveClass(
      "border-2",
      "border-dashed",
    );
  },
};

/** Unmarked unless its caller has a mark in mind, which this one has. */
export const Link: Story = {
  render: () => (
    <div className="flex flex-wrap items-center gap-2">
      <ButtonLink variant="default" to="/routes/12/2">
        Open route
      </ButtonLink>
      <Button variant="default">Run now</Button>
      <ButtonLink to="/sync" icon={<IconRefresh stroke={2} />}>
        Sync
      </ButtonLink>
    </div>
  ),
  play: async ({ canvas }) => {
    // A navigation that looks like an action is still a link: middle-click,
    // copy the address, and open in a new tab all have to keep working.
    const link = canvas.getByRole("link", { name: "Open route" });
    await expect(link).toHaveAttribute("href", "/routes/12/2");
    // The appearance is shared with the button of the same weight, and only
    // the appearance: the element is what makes it a link.
    await expect(link.className).toBe(canvas.getByRole("button", { name: "Run now" }).className);
  },
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
