/**
 * Five positions on the ride page's effort panel — today's flat grid plus
 * four variants — each rendered against the four ride shapes the data
 * allows, stacked in one column so a screenshot compares them at once.
 *
 * Storybook only: nothing here is imported by the application.
 */

import type { Meta, StoryObj } from "@storybook/react-vite";
import type { Activity } from "../../../api/types";
import { TrainingLoad } from "../TrainingLoad";
import {
  FoldedDiagnosticsPanel,
  GroupedPanel,
  LeanPanel,
  PrimaryDisclosurePanel,
  ShapeStack,
} from "./effortVariants";

const meta = {
  title: "Spikes/Effort Panel",
  parameters: { layout: "padded", chromatic: { disableSnapshot: true } },
} satisfies Meta;

export default meta;
type Story = StoryObj<typeof meta>;

function panel(
  description: string,
  Panel: (props: { ride: Activity }) => React.JSX.Element,
): Story {
  return {
    render: () => (
      <div className="mx-auto flex max-w-2xl flex-col gap-6 bg-[var(--base)] p-6">
        <p className="max-w-prose text-[var(--ink-2)] text-sm">{description}</p>
        <ShapeStack Panel={Panel} />
      </div>
    ),
    parameters: { docs: { description: { story: description } } },
  };
}

/** Today · every figure the ride allows in one flat three-column grid, equal weight, no groups. */
export const Baseline = panel(
  "Today: every figure in one flat grid beside the zone bar. Nothing groups readers, estimate diagnostics and load scales together; the panel grows to eighteen tiles for a full power-meter ride with an intense effort.",
  ({ ride }) => <TrainingLoad ride={ride} />,
);

/** A · Small headings split sensors, power, load and physiology; diagnostics stay as tiles under Power. */
export const Grouped = panel(
  "Bet: naming the four audiences (sensors, power, load, physiology) makes the panel scannable without hiding anything. Cost: still up to eighteen tiles, just under four headings instead of one grid — a rider skimming past Power still meets three diagnostic tiles.",
  ({ ride }) => <GroupedPanel ride={ride} />,
);

/** B · Speed, max speed, heart rate, max heart rate, cadence and power/estimate up front; everything else behind one closed disclosure. */
export const PrimaryDisclosure = panel(
  "Bet: most riders only ever read the six reader figures; everyone else is one click away. Cost: diagnostics, normalized power, TSS/hrTSS/TRIMP, decoupling and heat drift all sit behind 'More figures', closed by default — a rider who wants to judge the estimate's quality, or compare load scales, must know to open it first.",
  ({ ride }) => <PrimaryDisclosurePanel ride={ride} />,
);

/** B (opened) · The same layout with the disclosure open, to see what it costs a click to reach. */
export const PrimaryDisclosureOpened = panel(
  "The same variant with the disclosure open by default, so the hidden figures are visible for comparison. In the shipped panel it stays closed.",
  ({ ride }) => <PrimaryDisclosurePanel ride={ride} defaultOpen />,
);

/** C · The estimate's three diagnostics fold into the estimated-power tile as a caption and a quality badge; load figures grouped; no disclosure. */
export const FoldedDiagnostics = panel(
  "Bet: three tiles that only ever describe one other tile read better as that tile's caption — a rider glances at 'steady, low jitter, +2 W clamp bias' instead of parsing three separate labels. Cost: the exact per-figure values move from a tile to a hover title, so a reader who wants the raw jitter number must hover rather than read it outright.",
  ({ ride }) => <FoldedDiagnosticsPanel ride={ride} />,
);

/** D · Folded diagnostics plus the load scale the RideFigures headline already shows dropped from the panel. */
export const Lean = panel(
  "Bet: RideFigures already shows the ride's most specific load scale large; repeating it here is the panel's least informative tile. Cost: the panel now shows a different subset of load scales depending on which headline figure won, which a rider comparing two rides side by side could read as the panel being inconsistent rather than as the headline having already said one of them.",
  ({ ride }) => <LeanPanel ride={ride} />,
);
