/**
 * Design exploration only — not production code. Three ways to fold the
 * planner sidebar's route settings and per-row actions into collapsible
 * sections with an in-card sticky footer. See the task brief for the
 * complaints these answer.
 */

import type { Meta, StoryObj } from "@storybook/react-vite";
import type { ReactNode } from "react";
import { TooltipProvider } from "../../../components/ui/tooltip";
import { VariantG, VariantH, VariantI } from "./EvenMoreVariants";
import { SCENARIOS, type Scenario } from "./fixtures";
import { type LegShape, type LegStyle, VariantJ } from "./MixVariant";
import { VariantD, VariantE, VariantF } from "./MoreVariants";
import { VariantA } from "./VariantA";
import { VariantB } from "./VariantB";
import { VariantC } from "./VariantC";

function Frame({ children }: { children: ReactNode }) {
  return (
    <TooltipProvider delay={150}>
      <div className="flex gap-4 bg-[var(--base)] p-4">
        {/* A fake map strip so the card sits the way it does beside the real map. */}
        <div className="hidden h-[640px] w-64 shrink-0 rounded-2xl bg-[color-mix(in_oklab,var(--accent)_10%,var(--base))] lg:block" />
        {children}
      </div>
    </TooltipProvider>
  );
}

function scenarioOf(key: string): Scenario {
  const scenario = SCENARIOS.find((entry) => entry.key === key);
  if (!scenario) {
    throw new Error(`Unknown scenario: ${key}`);
  }
  return scenario;
}

interface SpikeArgs {
  scenario: string;
}

const meta: Meta<SpikeArgs> = {
  title: "Spikes/Planner Sidebar",
  argTypes: {
    scenario: {
      options: SCENARIOS.map((scenario) => scenario.key),
      control: {
        type: "select",
        labels: Object.fromEntries(SCENARIOS.map((s) => [s.key, s.label])),
      },
    },
  },
  args: { scenario: "draft3" },
};

export default meta;
type Story = StoryObj<SpikeArgs>;

function allStates(
  Variant: (props: { scenario: Scenario }) => ReactNode,
): NonNullable<Story["render"]> {
  return () => (
    <Frame>
      <div className="flex flex-wrap gap-4">
        {SCENARIOS.map((scenario) => (
          <Variant key={scenario.key} scenario={scenario} />
        ))}
      </div>
    </Frame>
  );
}

// --- Variant A: "⋯" row menu, two-button footer ---------------------------

export const A_RowMenu: Story = {
  name: "A — Row menu",
  render: ({ scenario }) => (
    <Frame>
      <VariantA scenario={scenarioOf(scenario)} />
    </Frame>
  ),
};

export const A_AllStates: Story = { name: "A — All states", render: allStates(VariantA) };

// --- Variant B: hover-revealed actions, one adaptive footer button --------

export const B_HoverActions: Story = {
  name: "B — Hover actions",
  render: ({ scenario }) => (
    <Frame>
      <VariantB scenario={scenarioOf(scenario)} />
    </Frame>
  ),
};

export const B_AllStates: Story = { name: "B — All states", render: allStates(VariantB) };

// --- Variant C: quick-toggle section headers, one status chip -------------

export const C_QuickToggles: Story = {
  name: "C — Quick toggles",
  render: ({ scenario }) => (
    <Frame>
      <VariantC scenario={scenarioOf(scenario)} />
    </Frame>
  ),
};

export const C_AllStates: Story = { name: "C — All states", render: allStates(VariantC) };

export const D_Timeline: Story = {
  name: "D — Timeline",
  render: ({ scenario }) => (
    <Frame>
      <VariantD scenario={scenarioOf(scenario)} />
    </Frame>
  ),
};

export const D_AllStates: Story = { name: "D — All states", render: allStates(VariantD) };

export const E_SettingsPopover: Story = {
  name: "E — Settings popover",
  render: ({ scenario }) => (
    <Frame>
      <VariantE scenario={scenarioOf(scenario)} />
    </Frame>
  ),
};

export const E_AllStates: Story = { name: "E — All states", render: allStates(VariantE) };

export const F_SelectionBar: Story = {
  name: "F — Selection bar",
  render: ({ scenario }) => (
    <Frame>
      <VariantF scenario={scenarioOf(scenario)} />
    </Frame>
  ),
};

export const F_AllStates: Story = { name: "F — All states", render: allStates(VariantF) };

export const G_ExpandingRows: Story = {
  name: "G — Expanding rows",
  render: ({ scenario }) => (
    <Frame>
      <VariantG scenario={scenarioOf(scenario)} />
    </Frame>
  ),
};

export const G_AllStates: Story = { name: "G — All states", render: allStates(VariantG) };

export const H_Legs: Story = {
  name: "H — Legs as rows",
  render: ({ scenario }) => (
    <Frame>
      <VariantH scenario={scenarioOf(scenario)} />
    </Frame>
  ),
};

export const H_AllStates: Story = { name: "H — All states", render: allStates(VariantH) };

export const I_StatsAndCommit: Story = {
  name: "I — Stats and one button",
  render: ({ scenario }) => (
    <Frame>
      <VariantI scenario={scenarioOf(scenario)} />
    </Frame>
  ),
};

export const I_AllStates: Story = { name: "I — All states", render: allStates(VariantI) };

export const J_Mix: StoryObj<
  SpikeArgs & { look: LegStyle; shape: LegShape; tall: boolean; deleting: boolean }
> = {
  name: "J — The mix",
  argTypes: {
    shape: { options: ["wave", "wide-wave", "s-curve", "zigzag"], control: { type: "select" } },
    tall: { control: { type: "boolean" } },
    deleting: { control: { type: "boolean" } },
    look: {
      options: ["hairline", "solid", "dashed", "dotted", "mixed", "dotted-mixed"],
      control: { type: "select" },
    },
  },
  args: { look: "dotted-mixed", shape: "s-curve", tall: false, deleting: false },
  render: ({ scenario, look, shape, tall, deleting }) => (
    <Frame>
      <VariantJ
        scenario={scenarioOf(scenario)}
        look={look}
        shape={shape}
        tall={tall}
        deleting={deleting}
      />
    </Frame>
  ),
};

export const J_AllStates: Story = { name: "J — All states", render: allStates(VariantJ) };

const LOOKS: LegStyle[] = ["hairline", "solid", "dashed", "dotted", "mixed"];

export const J_Lines: Story = {
  name: "J — Connecting lines",
  render: ({ scenario }) => (
    <Frame>
      <div className="flex flex-wrap gap-4">
        {LOOKS.map((look) => (
          <div key={look} className="flex flex-col gap-2">
            <p className="font-medium text-xs">{look}</p>
            <VariantJ scenario={scenarioOf(scenario)} look={look} />
          </div>
        ))}
      </div>
    </Frame>
  ),
};

export const J_LongList: StoryObj<
  SpikeArgs & { focus: number; neighbours: number; dragging: boolean }
> = {
  name: "J — Long list, collapsed",
  argTypes: {
    focus: { control: { type: "range", min: -1, max: 14, step: 1 } },
    neighbours: { control: { type: "range", min: 0, max: 3, step: 1 } },
    dragging: { control: { type: "boolean" } },
  },
  args: { scenario: "long15", focus: 7, neighbours: 1, dragging: false },
  render: ({ scenario, focus, neighbours, dragging }) => (
    <Frame>
      <VariantJ
        scenario={scenarioOf(scenario)}
        look="dotted-mixed"
        shape="s-curve"
        collapse
        focus={focus}
        neighbours={neighbours}
        dragging={dragging}
      />
    </Frame>
  ),
};
