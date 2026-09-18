/** Chrome shared by the three variants: the real sidebar's card, but with a footer pinned inside it. */

import { IconChevronDown, IconRoute } from "@tabler/icons-react";
import type { ReactNode } from "react";
import { Collapsible, CollapsibleContent, CollapsibleTrigger } from "@/components/ui/collapsible";
import { cn } from "@/lib/utils";
import { PanelMark } from "../../../components/PanelHeading";

/** The card's own width in the real sidebar rail. */
export function CardShell({
  title,
  aside,
  children,
  footer,
}: {
  title: ReactNode;
  aside?: ReactNode;
  children: ReactNode;
  footer: ReactNode;
}) {
  return (
    <section className="flex h-[640px] w-[22.5rem] min-w-0 flex-col rounded-2xl bg-[var(--panel)] shadow-[var(--shadow)]">
      <div className="flex min-h-9 shrink-0 items-center gap-3 px-5 pt-5 pb-3">
        <PanelMark>
          <IconRoute size={18} stroke={1.8} />
        </PanelMark>
        <h2 className="min-w-0 flex-1 truncate font-semibold text-base leading-tight">{title}</h2>
        {aside ? <div className="shrink-0">{aside}</div> : null}
      </div>
      <div className="flex min-h-0 flex-1 flex-col overflow-y-auto px-5">{children}</div>
      <div className="shrink-0 border-[var(--rule)] border-t p-3">{footer}</div>
    </section>
  );
}

/** A section that opens/closes, its header carrying a summary badge instead of a dead empty body. */
export function Section({
  title,
  summary,
  defaultOpen,
  children,
}: {
  title: string;
  summary: ReactNode;
  defaultOpen: boolean;
  children: ReactNode;
}) {
  return (
    <Collapsible
      defaultOpen={defaultOpen}
      className="border-[var(--rule)] border-b py-1 last:border-b-0"
    >
      <CollapsibleTrigger className="group flex w-full items-center gap-2 py-2 text-left font-semibold text-sm">
        <IconChevronDown
          aria-hidden="true"
          size={14}
          className="shrink-0 text-[var(--ink-2)] transition-transform group-data-[panel-open]:rotate-180"
        />
        <span className="flex-1">{title}</span>
        <span className="font-normal text-[var(--ink-2)] text-xs">{summary}</span>
      </CollapsibleTrigger>
      <CollapsibleContent className="pb-2">{children}</CollapsibleContent>
    </Collapsible>
  );
}

/** A section with nothing in it: one quiet line, no header to expand and no dead space beneath it. */
export function EmptySection({ children }: { children: ReactNode }) {
  return (
    <p className="border-[var(--rule)] border-b py-3 text-[var(--ink-2)] text-xs last:border-b-0">
      {children}
    </p>
  );
}

export function statusDot(colourVar: string, pulse: boolean) {
  return (
    <span
      aria-hidden="true"
      className={cn("size-2 shrink-0 rounded-full", pulse ? "animate-pulse" : undefined)}
      style={{ background: `var(${colourVar})` }}
    />
  );
}
