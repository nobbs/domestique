import type { Meta, StoryObj } from "@storybook/react-vite";
import type { ReactNode } from "react";
import type { TraceSummary } from "./planner";
import { TraceFinished, TraceStatus } from "./TraceStatus";

/** The chips are absolutely positioned over the map; this stands in for it. */
function ChipStage({ children }: { children: ReactNode }) {
  return (
    <div className="relative h-72 w-[28rem] overflow-hidden rounded-2xl bg-[color-mix(in_oklab,var(--ink-2)_10%,var(--panel))]">
      {children}
    </div>
  );
}

const CLEAN: TraceSummary = {
  waypoints: { seed: 14, peak: 38, final: 24 },
  rounds: 11,
  seconds: 18,
  followedShare: 0.996,
  strayedStretches: 0,
  planKm: 80.9,
  copiedKm: 81.2,
  outcome: "complete",
};

const STRAYED: TraceSummary = {
  waypoints: { seed: 16, peak: 41, final: 27 },
  rounds: 13,
  seconds: 22,
  followedShare: 0.981,
  strayedStretches: 2,
  planKm: 78.4,
  copiedKm: 79.6,
  outcome: "complete",
};

const STOPPED_SHORT: TraceSummary = {
  waypoints: { seed: 18, peak: 200, final: 200 },
  rounds: 16,
  seconds: 47,
  followedShare: 0.91,
  strayedStretches: 7,
  planKm: 95.4,
  copiedKm: 92.1,
  outcome: "stoppedShort",
};

const meta = {
  title: "Features/Planner/Trace status",
  component: TraceStatus,
  tags: ["autodocs"],
  args: {
    phase: "add",
    waypoints: 12,
    pause: null,
    onPause: () => {},
    onResume: () => {},
    onCancel: () => {},
  },
  decorators: [
    (Story) => (
      <ChipStage>
        <Story />
      </ChipStage>
    ),
  ],
} satisfies Meta<typeof TraceStatus>;

export default meta;
type Story = StoryObj<typeof meta>;

export const Matching: Story = {};

export const Trimming: Story = { args: { phase: "prune", waypoints: 27 } };

export const PausedByTheAdmin: Story = { args: { pause: "user", waypoints: 12 } };

export const EngineBusy: Story = { args: { pause: "busy", waypoints: 12 } };

export const FinishedComplete: Story = {
  render: () => <TraceFinished summary={CLEAN} onDismiss={() => {}} />,
};

export const FinishedStrayed: Story = {
  render: () => <TraceFinished summary={STRAYED} onDismiss={() => {}} />,
};

export const StoppedShort: Story = {
  render: () => <TraceFinished summary={STOPPED_SHORT} onDismiss={() => {}} />,
};
