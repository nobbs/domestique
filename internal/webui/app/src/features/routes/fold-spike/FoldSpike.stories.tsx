/**
 * Five ways the dock's fold control could replace the top-edge pill.
 *
 * Storybook only. See `variants.tsx` for the bet each one makes.
 */

import type { Meta, StoryObj } from "@storybook/react-vite";
import { StoryProviders } from "../../../storybook/fixtures";
import { CornerFold, GripFold, InlineFold, RailFold, TabFold } from "./variants";

const meta = {
  title: "Spikes/Dock Fold",
  tags: ["autodocs"],
  // The departure picker and the "back hh:mm" readout print a clock-relative time.
  parameters: { layout: "fullscreen", chromatic: { disableSnapshot: true } },
} satisfies Meta;

export default meta;
type Story = StoryObj<typeof meta>;

const VARIANTS = [
  { name: "A · Corner", note: "A square button in the panel's corner.", Fold: CornerFold },
  { name: "B · Rail stop", note: "The fold is a third rail item.", Fold: RailFold },
  { name: "C · Grip", note: "A full-width drag grip on the seam.", Fold: GripFold },
  {
    name: "D · In the line",
    note: "Beside the profile stop's ⓘ; the pill stays.",
    Fold: InlineFold,
  },
  { name: "E · Tab", note: "A short tab hanging from the seam.", Fold: TabFold },
] as const;

export const SideBySide: Story = {
  render: () => (
    <StoryProviders>
      <div className="grid gap-8 bg-[var(--ground)] p-6">
        {VARIANTS.map(({ name, note, Fold }) => (
          <div key={name} className="grid gap-2">
            <div>
              <h2 className="text-sm font-semibold">{name}</h2>
              <p className="text-xs text-[var(--ink-2)]">{note}</p>
            </div>
            <Fold />
          </div>
        ))}
      </div>
    </StoryProviders>
  ),
};

function only(Fold: () => React.JSX.Element): Story {
  return {
    render: () => (
      <StoryProviders>
        <Fold />
      </StoryProviders>
    ),
  };
}

export const Corner = only(CornerFold);
export const RailStop = only(RailFold);
export const Grip = only(GripFold);
export const InLine = only(InlineFold);
export const Tab = only(TabFold);
