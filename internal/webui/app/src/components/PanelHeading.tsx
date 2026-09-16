/**
 * A card's heading: a dark mark naming what the card is about, the title,
 * and what qualifies it in a quieter tone on the same line.
 */

import type { ReactNode } from "react";

export interface PanelHeadingProps {
  /** The glyph in the mark, drawn at 18 pixels. */
  icon: ReactNode;
  title: ReactNode;
  /** After the title, muted: what the title is measured against or projected from. */
  subtitle?: ReactNode;
  /** Whatever stands at the row's end: a switch, a count, an action. */
  aside?: ReactNode;
}

export function PanelHeading({ icon, title, subtitle, aside }: PanelHeadingProps) {
  return (
    <div className="flex min-h-9 items-center gap-3">
      <span
        aria-hidden="true"
        className="grid size-9 shrink-0 place-items-center rounded-md bg-[radial-gradient(circle_at_50%_35%,#6e6e6e,#3d3d3d_85%)] text-[var(--panel)]"
      >
        {icon}
      </span>
      <h2 className="min-w-0 font-semibold text-base leading-tight">
        {title}
        {subtitle ? (
          <span className="ml-1.5 font-normal text-[var(--ink-2)]">{subtitle}</span>
        ) : null}
      </h2>
      {aside ? <div className="ml-auto shrink-0">{aside}</div> : null}
    </div>
  );
}
