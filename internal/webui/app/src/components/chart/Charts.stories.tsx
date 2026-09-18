/**
 * The chart pieces on their own, each wired to one shared pointer so a key
 * named by one lights it in the others, as a page composing them would.
 */

import type { Meta, StoryObj } from "@storybook/react-vite";
import { useState } from "react";
import { DonutChart } from "./DonutChart";
import { HistogramChart } from "./HistogramChart";
import { LegendTable } from "./LegendTable";

const meta = {
  title: "Components/Charts",
  decorators: [
    (Story) => (
      <div className="max-w-xl rounded-xl bg-[var(--panel)] p-4 text-[var(--ink)] shadow-[var(--shadow)]">
        <Story />
      </div>
    ),
  ],
} satisfies Meta;

export default meta;
type Story = StoryObj<typeof meta>;

const GROUPS = [
  { key: 0, label: "Low", colour: "var(--grade-0)", value: 40 },
  { key: 1, label: "Middle", colour: "var(--grade-1)", value: 45 },
  { key: 2, label: "High", colour: "var(--grade-3)", value: 15 },
];

/** Thirty bars rising and falling, the first ten Low, the next fifteen Middle, the rest High. */
const BARS = Array.from({ length: 30 }, (_, index) => {
  const group = index < 10 ? 0 : index < 25 ? 1 : 2;
  return {
    value: Math.round(40 * Math.exp(-(((index - 14) / 7) ** 2))) + 1,
    colour: GROUPS[group]?.colour ?? "",
    group,
  };
});

function Linked() {
  const [active, setActive] = useState<number | null>(null);
  return (
    <div className="flex flex-col gap-4">
      <div className="flex flex-wrap items-center gap-6">
        <DonutChart segments={GROUPS} active={active} onActive={setActive}>
          <span className="font-semibold text-lg">
            {active === null ? "100" : GROUPS[active]?.value}
          </span>
        </DonutChart>
        <LegendTable
          className="min-w-60 flex-1"
          rows={GROUPS.map((group) => ({
            ...group,
            value: String(group.value),
            share: `${group.value}%`,
          }))}
          active={active}
          onActive={setActive}
        />
      </div>
      <HistogramChart
        label="Spread"
        bars={BARS}
        markers={[
          { edge: 10, label: "10" },
          { edge: 25, label: "25" },
        ]}
        unit="units"
        activeGroup={active}
        onActiveGroup={setActive}
        readout={(index) => `bar ${index}: ${BARS[index]?.value}`}
      />
    </div>
  );
}

/** Point at a ring segment, a row or a bar: the others follow. */
export const LinkedByPointer: Story = { render: () => <Linked /> };
