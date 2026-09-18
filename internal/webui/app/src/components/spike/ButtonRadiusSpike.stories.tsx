/**
 * Button corner radii against the segmented control's, side by side.
 *
 * Storybook only: each row passes its radius through `className`, which the
 * button's class merge lets win over the primitive's `rounded-lg`.
 */

import type { Meta, StoryObj } from "@storybook/react-vite";
import { IconDownload, IconPlus, IconRefresh, IconTrash, IconX } from "@tabler/icons-react";
import { useState } from "react";
import { Button, type ButtonVariant } from "../Button";
import { Segmented } from "../Segmented";

const RADII = [
  { name: "A · Before", note: "rounded-lg, 14px on a 32px button", radius: "rounded-lg" },
  {
    name: "B · Track",
    note: "12px, the segmented control's outer track",
    radius: "rounded-[12px]",
  },
  {
    name: "C · Segment",
    note: "9px, the segments and thumb inside the track",
    radius: "rounded-[9px]",
  },
  { name: "D · Pill", note: "fully round ends", radius: "rounded-full" },
] as const;

const VARIANTS: ButtonVariant[] = [
  "default",
  "outline",
  "ghost",
  "destructive",
  "warning",
  "panel",
];

function Row({ radius }: { radius: string }) {
  const [range, setRange] = useState("365");
  return (
    <div className="flex flex-wrap items-center gap-3">
      <Segmented
        label="Range"
        size="sm"
        items={[
          { key: "90", label: "3 months" },
          { key: "365", label: "1 year" },
          { key: "all", label: "All" },
        ]}
        value={range}
        onChange={setRange}
      />
      {VARIANTS.map((variant) => (
        <Button
          key={variant}
          variant={variant}
          className={radius}
          icon={
            variant === "destructive" ? (
              <IconTrash />
            ) : variant === "default" ? (
              <IconPlus />
            ) : undefined
          }
        >
          {variant === "default" ? "New plan" : variant[0]?.toUpperCase() + variant.slice(1)}
        </Button>
      ))}
      <Button variant="outline" className={radius} icon={<IconRefresh />} aria-label="Refresh" />
      <Button variant="panel" className={radius} icon={<IconX />} aria-label="Close" />
      <Button variant="outline" className={radius} icon={<IconDownload />}>
        Export
      </Button>
    </div>
  );
}

const meta = {
  title: "Spikes/Button Radius",
  parameters: { layout: "fullscreen" },
} satisfies Meta;

export default meta;
type Story = StoryObj<typeof meta>;

export const AllRadii: Story = {
  render: () => (
    <div className="flex min-h-dvh flex-col gap-6 bg-[var(--base)] p-8 text-[var(--ink)]">
      {RADII.map(({ name, note, radius }) => (
        <section
          key={name}
          className="flex flex-col gap-3 rounded-2xl bg-[var(--panel)] p-5 shadow-[var(--shadow)]"
        >
          <span className="text-sm">
            <span className="font-semibold">{name}</span>{" "}
            <span className="text-[var(--ink-2)]">{note}</span>
          </span>
          <Row radius={radius} />
        </section>
      ))}
    </div>
  ),
};
