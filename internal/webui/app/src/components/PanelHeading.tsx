/**
 * A card's heading: a dark mark naming what the card is about, the title,
 * and what qualifies it in a quieter tone on the same line.
 */

import { type ReactNode, useId } from "react";

export interface PanelHeadingProps {
  /** The glyph in the mark, drawn at 18 pixels. */
  icon: ReactNode;
  title: ReactNode;
  /** After the title, muted: what the title is measured against or projected from. */
  subtitle?: ReactNode;
  /** Whatever stands at the row's end: a switch, a count, an action. */
  aside?: ReactNode;
  /** The heading's level in the page outline; 2 unless the card sits under another heading. */
  level?: 2 | 3;
  id?: string;
}

export function PanelHeading({ icon, title, subtitle, aside, level = 2, id }: PanelHeadingProps) {
  const Heading = level === 3 ? "h3" : "h2";
  return (
    <div className="flex min-h-9 items-center gap-3">
      <PanelMark>{icon}</PanelMark>
      <Heading id={id} className="min-w-0 font-semibold text-base leading-tight">
        {title}
        {subtitle ? (
          <span className="ml-1.5 font-normal text-[var(--ink-2)]">{subtitle}</span>
        ) : null}
      </Heading>
      {aside ? <div className="ml-auto shrink-0">{aside}</div> : null}
    </div>
  );
}

/** The dark square a heading or a headline figure carries its glyph in. */
export function PanelMark({ children }: { children: ReactNode }) {
  return (
    <span
      aria-hidden="true"
      className="grid size-9 shrink-0 place-items-center rounded-md bg-[radial-gradient(circle_at_50%_35%,#6e6e6e,#3d3d3d_85%)] text-[var(--panel)]"
    >
      {children}
    </span>
  );
}

/** The card every page section sits in: its marked heading, then whatever answers it. */
export function Panel({
  children,
  className,
  ...heading
}: PanelHeadingProps & { children: ReactNode; className?: string }) {
  const fallback = useId();
  const id = heading.id ?? fallback;
  return (
    <section
      aria-labelledby={id}
      className={`flex min-w-0 flex-col gap-4 rounded-2xl bg-[var(--panel)] p-5 shadow-[var(--shadow)] ${className ?? ""}`}
    >
      <PanelHeading {...heading} id={id} />
      {children}
    </section>
  );
}
