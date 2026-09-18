/**
 * The hand-built controls that do not go through `Button`, today's corners beside 9px.
 *
 * Storybook only: static copies of each control's classes, the radius the one change.
 */

import type { Meta, StoryObj } from "@storybook/react-vite";
import {
  IconArrowDown,
  IconDots,
  IconInfoCircle,
  IconLayoutSidebarRightCollapse,
  IconStairs,
  IconX,
} from "@tabler/icons-react";
import type { ReactNode } from "react";
import { Button } from "../Button";

const HOVER = "bg-[var(--base)]";
const PILL = "bg-[var(--muted)] px-2.5 py-1 text-[11px] font-medium text-[var(--ink-2)]";

function RoutePanelControls({ radius }: { radius: string }) {
  const cell = "grid h-7 w-8 place-items-center text-[var(--ink-2)]";
  return (
    <div className={`flex w-fit overflow-hidden bg-[var(--muted)] ${radius}`}>
      <span className={cell}>
        <IconLayoutSidebarRightCollapse size={16} stroke={2} />
      </span>
      <span className={`${cell} border-[var(--rule)] border-l`}>
        <IconDots size={16} stroke={2} />
      </span>
      <span className={`${cell} border-[var(--rule)] border-l`}>
        <IconX size={16} stroke={2} />
      </span>
    </div>
  );
}

function DockPills({ radius }: { radius: string }) {
  return (
    <div className="flex items-center gap-2">
      <span className={`${PILL} ${radius}`}>Whole route</span>
      <span className={`flex items-center gap-1 ${PILL} ${radius}`}>
        <IconStairs size={13} stroke={2} />
        Climbs
      </span>
      <span
        className={`flex items-center gap-1 bg-[var(--panel)] px-2.5 py-1 font-medium text-[11px] text-[var(--ink)] shadow-[var(--shadow)] ${radius}`}
      >
        <IconStairs size={13} stroke={2} />
        Climbs open
      </span>
      <span
        className={`grid size-7 place-items-center bg-[var(--muted)] text-[var(--ink-2)] ${radius}`}
      >
        <IconInfoCircle size={16} stroke={1.8} />
      </span>
      <span className="flex items-center gap-1.5 text-sm">
        Forecast
        <span
          className={`bg-[var(--rule)] px-1.5 py-px text-[10px] text-[var(--ink)] tabular-nums ${radius}`}
        >
          12
        </span>
      </span>
    </div>
  );
}

function SortHeaders({ small, large }: { small: string; large: string }) {
  return (
    <div className="flex items-center gap-4">
      <span
        className={`inline-flex items-center gap-0.5 px-1 py-0.5 font-semibold text-xs ${HOVER} ${small}`}
      >
        Distance
        <IconArrowDown size={12} stroke={2.2} />
      </span>
      <span className={`flex items-center gap-1 px-3 py-2 font-semibold text-sm ${HOVER} ${large}`}>
        Ascent
        <IconArrowDown size={14} stroke={2} />
      </span>
      <span className="text-[var(--ink-2)] text-xs">(hover wash shown)</span>
    </div>
  );
}

function RegionChip({ radius }: { radius: string }) {
  return (
    <span className="inline-flex items-center gap-1.5 rounded-full bg-[var(--muted)] py-0.5 pr-1 pl-2.5 text-xs">
      germany/hessen
      <span className={`grid size-5 place-items-center bg-[var(--rule)] ${radius}`}>
        <IconX className="size-3" />
      </span>
    </span>
  );
}

function Row({ label, before, after }: { label: string; before: ReactNode; after: ReactNode }) {
  return (
    <div className="flex flex-col gap-2 border-[var(--rule)] border-t pt-4 first:border-0 first:pt-0">
      <span className="font-medium text-sm">{label}</span>
      <div className="grid grid-cols-[3rem_1fr] items-center justify-items-start gap-3">
        <span className="text-[var(--ink-2)] text-xs">Today</span>
        {before}
        <span className="text-[var(--ink-2)] text-xs">9px</span>
        {after}
      </div>
    </div>
  );
}

const meta = {
  title: "Spikes/Control Radius",
  parameters: { layout: "fullscreen" },
} satisfies Meta;

export default meta;
type Story = StoryObj<typeof meta>;

export const BeforeAndAfter: Story = {
  render: () => (
    <div className="min-h-dvh bg-[var(--base)] p-8 text-[var(--ink)]">
      <div className="flex max-w-3xl flex-col gap-4 rounded-2xl bg-[var(--panel)] p-6 shadow-[var(--shadow)]">
        <Row
          label="Beside a Button (now 9px)"
          before={<Button variant="outline">Export</Button>}
          after={<Button variant="outline">Export</Button>}
        />
        <Row
          label="Route panel controls"
          before={<RoutePanelControls radius="rounded-full" />}
          after={<RoutePanelControls radius="rounded-[9px]" />}
        />
        <Row
          label="Dock pills and count"
          before={<DockPills radius="rounded-full" />}
          after={<DockPills radius="rounded-[9px]" />}
        />
        <Row
          label="Catalogue sort headers"
          before={<SortHeaders small="rounded" large="rounded-md" />}
          after={<SortHeaders small="rounded-[9px]" large="rounded-[9px]" />}
        />
        <Row
          label="Region chip remove"
          before={<RegionChip radius="rounded-full" />}
          after={<RegionChip radius="rounded-[9px]" />}
        />
      </div>
    </div>
  ),
};
