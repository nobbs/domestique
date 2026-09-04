/**
 * Four positions on how the dock should divide its lanes, one above the other.
 *
 * Storybook only. See `variants.tsx` for the bet each one makes.
 */

import type { Meta, StoryObj } from "@storybook/react-vite";
import { StoryProviders } from "../../../storybook/fixtures";
import {
  AnchoredDock,
  type DockState,
  RailDock,
  SplitDock,
  TabsDock,
  useDockState,
} from "./variants";

const meta = {
  title: "Spikes/Route Dock",
  tags: ["autodocs"],
  // The departure picker prints a day relative to the real clock.
  parameters: { layout: "fullscreen", chromatic: { disableSnapshot: true } },
} satisfies Meta;

export default meta;
type Story = StoryObj<typeof meta>;

const VARIANTS = [
  { name: "A · Tabs", note: "Every lane behind a tab, one at a time.", Dock: TabsDock },
  {
    name: "B · Anchored",
    note: "Profile stays; tabs choose what lies beneath it.",
    Dock: AnchoredDock,
  },
  {
    name: "C · Rail",
    note: "Rail on the left, icon over a word; a new lane costs one stop.",
    Dock: RailDock,
  },
  {
    name: "D · Split",
    note: "Lanes stay stacked; only the side column is tabbed.",
    Dock: SplitDock,
  },
] as const;

export const SideBySide: Story = {
  render: () => {
    // One state across all four, so a press in one shows in the others.
    const state = useDockState();

    return (
      <StoryProviders>
        <div className="grid gap-8 bg-[var(--ground)] p-6">
          {VARIANTS.map(({ name, note, Dock }) => (
            <div key={name} className="grid gap-2">
              <div>
                <h2 className="text-sm font-semibold">{name}</h2>
                <p className="text-xs text-[var(--ink-2)]">{note}</p>
              </div>
              <Dock {...state} />
            </div>
          ))}
        </div>
      </StoryProviders>
    );
  },
};

function only(Dock: (s: DockState) => React.JSX.Element): Story {
  return {
    render: () => {
      const state = useDockState();

      return (
        <StoryProviders>
          <div className="bg-[var(--ground)] p-6">
            <Dock {...state} />
          </div>
        </StoryProviders>
      );
    },
  };
}

export const TabsOnly = only(TabsDock);
export const AnchoredOnly = only(AnchoredDock);
export const RailOnly = only(RailDock);
export const SplitOnly = only(SplitDock);
