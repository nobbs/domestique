import type { Meta, StoryObj } from "@storybook/react-vite";
import { expect, fn, userEvent } from "storybook/test";
import { SyncPhaseRow } from "./SyncPhaseRow";

const meta = {
  title: "Features/Sync/Sync Phase Row",
  component: SyncPhaseRow,
  tags: ["autodocs"],
  args: {
    phase: "targets",
    label: "Wahoo targets",
    lastRun: {
      lastCompletedAt: "2026-08-18T06:30:00Z",
      lastResult: "succeeded",
      sourceStages: 0,
      created: 3,
      updated: 12,
      deleted: 0,
    },
    enabled: true,
    scheduleDisabled: false,
    onToggle: fn(),
    running: false,
    onRun: fn(),
  },
  decorators: [
    (Story) => (
      <ul className="w-[28rem] bg-[var(--panel)] p-2">
        <Story />
      </ul>
    ),
  ],
} satisfies Meta<typeof SyncPhaseRow>;

export default meta;
type Story = StoryObj<typeof meta>;

export const Succeeded: Story = {};

export const NeverRun: Story = { args: { lastRun: undefined } };

/** A gate that held is stated, not styled as a failure — see `syncGuidance`. */
export const HeldByAGate: Story = {
  args: {
    lastRun: {
      lastCompletedAt: "2026-08-18T06:30:00Z",
      lastResult: "blocked",
      lastFailure: "deletion_limit",
      sourceStages: 0,
      created: 0,
      updated: 0,
      deleted: 0,
    },
  },
};

export const Running: Story = { args: { running: true } };

export const TogglesTheSchedule: Story = {
  play: async ({ canvas, args }) => {
    await userEvent.click(canvas.getByRole("switch", { name: "Hourly: Wahoo targets" }));

    await expect(args.onToggle).toHaveBeenCalled();
  },
};

export const RunsNow: Story = {
  play: async ({ canvas, args }) => {
    await userEvent.click(canvas.getByRole("button", { name: "Run now: Wahoo targets" }));

    await expect(args.onRun).toHaveBeenCalled();
  },
};
