/**
 * Four ways of saying where the forecast starts and stops.
 *
 * Stacked under a chart and a ribbon on the same axis, the filmstrip has no
 * edges of its own: a row of tinted tiles reads as another lane of the terrain
 * rather than as a different kind of claim about it. The terrain is measured
 * and the weather is *predicted*, and a panel that draws them alike is
 * inviting a reader to trust one as far as they trust the other.
 *
 * Since the band gained a rounded border of its own, three of these nest a box
 * inside a box, and the question has largely become whether anything beyond a
 * caption is still needed. They are kept so that the comparison is visible
 * rather than asserted.
 *
 * Whatever frames it must not indent it. The band lays its tiles on the shared
 * distance axis, and the whole argument for stacking is that a tile sits under
 * the ground it falls on — so every variant here borders without horizontal
 * padding, and lets the band's own reserved gutter do the insetting. Captions
 * are pushed in to where the plotted terrain starts, for the same reason.
 */

import { IconChevronsRight } from "@tabler/icons-react";
import type { ReactNode } from "react";
import { PADDING } from "../../lib/plotAxis";

function Caption({
  children,
  open,
  onOpenChange,
}: {
  children: ReactNode;
  open?: boolean | undefined;
  onOpenChange?: ((open: boolean) => void) | undefined;
}) {
  const style = { paddingLeft: PADDING.left };

  if (onOpenChange === undefined) {
    return (
      <p className="text-[11px] leading-none text-[var(--ink-2)]" style={style}>
        {children}
      </p>
    );
  }

  return (
    <button
      type="button"
      aria-expanded={open}
      onClick={() => onOpenChange(!open)}
      className="flex items-center gap-1 text-[11px] leading-none text-[var(--ink-2)] hover:text-[var(--ink)] focus-visible:outline-2 focus-visible:outline-offset-2 focus-visible:outline-[var(--accent)]"
      style={style}
    >
      <IconChevronsRight
        size={12}
        stroke={2}
        aria-hidden="true"
        className={open ? "rotate-90 transition-transform" : "transition-transform"}
      />
      {children}
    </button>
  );
}

export function ForecastFrame({
  caption,
  controls,
  open = true,
  onOpenChange,
  children,
}: {
  caption: string;
  /**
   * What sits at the end of the caption row — the departure, where the dock
   * makes it settable.
   *
   * Beside the caption rather than inside the band, because the band is what
   * folds away and the control that decides what it draws should not fold with
   * it. It also reads as one line: *forecast, from this moment*.
   */
  controls?: ReactNode;
  /** Whether the band is shown. `plain` has no caption, so it cannot fold. */
  open?: boolean | undefined;
  onOpenChange?: ((open: boolean) => void) | undefined;
  children: ReactNode;
}) {
  const cap = (
    <Caption open={open} onOpenChange={onOpenChange}>
      {caption}
    </Caption>
  );
  const band = open ? children : null;

  return (
    <div className="grid gap-1.5">
      {controls === undefined ? (
        cap
      ) : (
        // Siblings, not nested: the caption is a button and so is half the
        // departure control, and one cannot live inside the other.
        <div className="flex flex-wrap items-center justify-between gap-x-3 gap-y-1">
          {cap}
          {controls}
        </div>
      )}
      {band}
    </div>
  );
}
