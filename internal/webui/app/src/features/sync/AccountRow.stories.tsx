import type { Meta, StoryObj } from "@storybook/react-vite";
import { useState } from "react";
import { expect, fn, screen, userEvent } from "storybook/test";
import type { TargetStatus } from "../../api/types";
import { AccountRow } from "./AccountRow";

const connected: TargetStatus = {
  id: "rider-a",
  authorisation: "authorized",
  convergence: "current",
  stages: { current: 4, pending: 0 },
  lastRun: { completedAt: "2026-08-18T06:00:00Z", result: "succeeded" },
};

/**
 * Holds the delete-confirmation state, as `TargetConvergence` does — one
 * slot shared across every row, so only one row's dialog can be open. The
 * popover it opens renders through a portal into `document.body`, outside
 * this story's own canvas root, which is why the interactive stories below
 * read it back with `screen` rather than `canvas`.
 */
function Held({
  target = connected,
  reconciling = false,
  onReconcile = fn(),
}: {
  target?: TargetStatus;
  reconciling?: boolean;
  onReconcile?: () => void;
}) {
  const [open, setOpen] = useState(false);
  const [confirmation, setConfirmation] = useState("");

  return (
    <AccountRow
      target={target}
      reconciling={reconciling}
      onReconcile={onReconcile}
      clear={{
        open,
        onOpenChange: (next) => {
          setOpen(next);
          if (!next) {
            setConfirmation("");
          }
        },
        confirmation,
        onConfirmationChange: setConfirmation,
        pending: false,
        onConfirm: fn(),
      }}
    />
  );
}

const meta = {
  title: "Features/Sync/Account Row",
  component: AccountRow,
  tags: ["autodocs"],
  args: {
    target: connected,
    reconciling: false,
    onReconcile: fn(),
    clear: {
      open: false,
      onOpenChange: fn(),
      confirmation: "",
      onConfirmationChange: fn(),
      pending: false,
      onConfirm: fn(),
    },
  },
  decorators: [
    (Story) => (
      <ul className="w-[28rem] bg-[var(--panel)] p-2">
        <Story />
      </ul>
    ),
  ],
} satisfies Meta<typeof AccountRow>;

export default meta;
type Story = StoryObj<typeof meta>;

export const Connected: Story = { render: () => <Held /> };

export const Behind: Story = {
  render: () => (
    <Held target={{ ...connected, convergence: "lagging", stages: { current: 2, pending: 2 } }} />
  ),
};

export const NotConnected: Story = {
  render: () => (
    <Held
      target={{
        id: "rider-b",
        authorisation: "not_authorized",
        convergence: "unauthorized",
        stages: { current: 0, pending: 4 },
      }}
    />
  ),
};

export const LastWriteFailed: Story = {
  render: () => (
    <Held
      target={{
        ...connected,
        lastRun: { completedAt: "2026-08-18T06:00:00Z", result: "failed", failure: "destination" },
      }}
    />
  ),
};

export const ReconcileReportsItself: Story = {
  args: { onReconcile: fn() },
  play: async ({ canvas, args }) => {
    await userEvent.click(canvas.getByRole("button", { name: "Reconcile now: rider-a" }));

    await expect(args.onReconcile).toHaveBeenCalled();
  },
};

/** The confirmation asks for the account's own name, and disables until it matches. */
export const DeletionNeedsTheAccountName: Story = {
  render: () => <Held />,
  play: async ({ canvas }) => {
    await userEvent.click(canvas.getByRole("button", { name: "Delete all routes…" }));

    const confirm = screen.getByRole("button", {
      name: "Delete every Domestique route from rider-a",
    });
    await expect(confirm).toBeDisabled();

    await userEvent.type(screen.getByRole("textbox"), "rider-a");

    await expect(confirm).not.toBeDisabled();
  },
};
