/**
 * Buttons joined into one control, stacked.
 *
 * The frame belongs to the group rather than to each button: one edge, one
 * radius, one shadow, and a rule between the buttons instead of around them.
 * The buttons inside are `ghost` — they are already standing on this ground and
 * would otherwise draw their own edge inside its.
 *
 * Stacked only, because the map's zoom pair is the one thing joined this way.
 * A row is a `flex-row` and a `divide-x` away when something wants one.
 */

import type { ReactNode } from "react";
import { cn } from "@/lib/utils";

export interface ButtonGroupProps {
  children: ReactNode;
  className?: string;
}

export function ButtonGroup({ children, className }: ButtonGroupProps) {
  return (
    <div
      // The primitive squares off the corners of anything inside a group that
      // names itself this way, which is why the slot is spelled its way.
      data-slot="button-group"
      className={cn(
        // The edge is a ring rather than a border because a border would be
        // laid out as well as drawn: the buttons inside are 32 pixels wide and
        // carry their own edges within that, so a group that added two more
        // would stand two wider than the single button stacked above it.
        "flex w-fit flex-col divide-y divide-[var(--rule)] overflow-hidden rounded-lg bg-[var(--panel)] shadow-[var(--shadow)] ring-1 ring-[var(--rule)] ring-inset",
        className,
      )}
    >
      {children}
    </div>
  );
}
