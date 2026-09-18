/**
 * Layout pieces the Account and Admin spikes share: a section rail, top tabs,
 * and a titled group. Storybook only.
 */

import { type ReactNode, useState } from "react";
import { SegmentedTrack, SegmentLabel, segmentClass } from "../../../components/Segmented";
import { cn } from "../../../lib/utils";

export interface SpikeSection {
  key: string;
  label: string;
  icon?: ReactNode;
  content: ReactNode;
}

/** A page title over whatever layout follows, on the page's ground. */
export function SpikePage({ title, children }: { title: string; children: ReactNode }) {
  return (
    <div className="min-h-dvh bg-[var(--base)] px-6 py-8 text-[var(--ink)]">
      <div className="mx-auto flex w-full max-w-[1200px] flex-col gap-5">
        <h1 className="font-semibold text-2xl tracking-tight">{title}</h1>
        {children}
      </div>
    </div>
  );
}

function Segments({
  sections,
  value,
  onChange,
  vertical,
}: {
  sections: readonly SpikeSection[];
  value: string;
  onChange: (key: string) => void;
  vertical?: boolean;
}) {
  return (
    <SegmentedTrack
      active={value}
      orientation={vertical ? "vertical" : "horizontal"}
      className={vertical ? "w-full" : undefined}
    >
      <div role="tablist" className="contents">
        {sections.map((section) => (
          <button
            key={section.key}
            type="button"
            role="tab"
            data-segment={section.key}
            aria-selected={section.key === value}
            onClick={() => onChange(section.key)}
            className={cn(segmentClass(), vertical && "w-full justify-start")}
          >
            {section.icon}
            <SegmentLabel>{section.label}</SegmentLabel>
          </button>
        ))}
      </div>
    </SegmentedTrack>
  );
}

/** A vertical rail of sections beside the one chosen. */
export function SectionRail({ sections }: { sections: readonly SpikeSection[] }) {
  const [value, setValue] = useState(sections[0]?.key ?? "");
  return (
    <div className="grid items-start gap-6 md:grid-cols-[14rem_minmax(0,1fr)]">
      <div className="md:sticky md:top-6">
        <Segments sections={sections} value={value} onChange={setValue} vertical />
      </div>
      <div className="flex min-w-0 max-w-3xl flex-col gap-5">
        {sections.find((section) => section.key === value)?.content}
      </div>
    </div>
  );
}

/** Tabs under the title, one section at a time. */
export function TopTabs({ sections }: { sections: readonly SpikeSection[] }) {
  const [value, setValue] = useState(sections[0]?.key ?? "");
  return (
    <div className="flex flex-col gap-5">
      <Segments sections={sections} value={value} onChange={setValue} />
      <div className="flex max-w-3xl flex-col gap-5">
        {sections.find((section) => section.key === value)?.content}
      </div>
    </div>
  );
}

/** A titled run of cards, for grouping on one long page. */
export function Group({ title, children }: { title: string; children: ReactNode }) {
  return (
    <section className="flex flex-col gap-3">
      <h2 className="pt-2 font-semibold text-[var(--ink-2)] text-sm uppercase tracking-wide">
        {title}
      </h2>
      {children}
    </section>
  );
}
