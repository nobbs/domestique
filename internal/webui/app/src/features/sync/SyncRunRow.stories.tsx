import type { Meta, StoryObj } from "@storybook/react-vite";
import type { SyncRun } from "../../api/types";
import { SyncRunRow } from "./SyncRunRow";

const succeeded: SyncRun = {
  reference: "1a2b3c4d5e6f",
  phase: "targets",
  completedAt: "2026-08-18T06:30:00Z",
  result: "succeeded",
  sourceStages: 0,
  created: 3,
  updated: 12,
  deleted: 1,
};

const meta = {
  title: "Features/Sync/Sync Run Row",
  component: SyncRunRow,
  tags: ["autodocs"],
  args: { run: succeeded, label: "Wahoo accounts" },
  decorators: [
    (Story) => (
      <ul className="w-96 bg-[var(--panel)] p-2">
        <Story />
      </ul>
    ),
  ],
} satisfies Meta<typeof SyncRunRow>;

export default meta;
type Story = StoryObj<typeof meta>;

export const Succeeded: Story = {};

/** A run held by a gate reads apart from one that broke outright. */
export const HeldByAGate: Story = {
  args: {
    run: { ...succeeded, result: "blocked", failure: "deletion_limit" },
  },
};

export const Failed: Story = {
  args: {
    run: { ...succeeded, result: "failed", failure: "destination" },
  },
};

/** No reference: a run recorded before the migration that named them. */
export const NoReference: Story = {
  args: { run: { ...succeeded, reference: "" } },
};

export const SourceRead: Story = {
  args: {
    run: { ...succeeded, phase: "source", sourceStages: 47, created: 0, updated: 0, deleted: 0 },
    label: "VeloPlanner",
  },
};
