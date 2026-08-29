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
import { PADDING } from "../../../lib/plotAxis";

export type WeatherFrameVariant = "plain" | "capped" | "card" | "legend" | "header";

export const WEATHER_FRAMES: ReadonlyArray<{
  readonly variant: WeatherFrameVariant;
  readonly note: string;
}> = [
  { variant: "plain", note: "no frame — the band as one more lane" },
  {
    variant: "capped",
    note: "a caption only, letting the strip's own rounded border be the frame",
  },
  { variant: "card", note: "a filled inset, so the prediction sits on its own ground" },
  { variant: "legend", note: "a hairline box with the word on the rule, as a fieldset" },
  { variant: "header", note: "a bordered box under a titled strip" },
];

/**
 * The caption, and — where the band can be folded away — the control that does
 * it. One thing rather than two: a caption beside a separate chevron spends a
 * second target on the same idea, and the word is what a reader aims at anyway.
 */
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

export function WeatherFrame({
  variant,
  caption,
  controls,
  open = true,
  onOpenChange,
  children,
}: {
  variant: WeatherFrameVariant;
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
  const cap =
    controls === undefined ? (
      <Caption open={open} onOpenChange={onOpenChange}>
        {caption}
      </Caption>
    ) : (
      // Siblings, not nested: the caption is a button and so is half of the
      // departure control, and one cannot live inside the other.
      <div className="flex flex-wrap items-center justify-between gap-x-3 gap-y-1">
        <Caption open={open} onOpenChange={onOpenChange}>
          {caption}
        </Caption>
        {controls}
      </div>
    );
  const band = open ? children : null;

  if (variant === "plain") {
    return <>{children}</>;
  }

  if (variant === "capped") {
    // The band draws its own rounded border, so a box around it is a box
    // inside a box. All that is left to add is the word — which is the part
    // that was actually missing, since a border says "this is one thing" and
    // not "this one is a guess".
    return (
      <div className="grid gap-1.5">
        {cap}
        {band}
      </div>
    );
  }

  if (variant === "card") {
    return (
      <div className="rounded-lg bg-[var(--base)] py-2">
        <div className={open ? "mb-1.5" : ""}>{cap}</div>
        {band}
      </div>
    );
  }

  if (variant === "legend") {
    return (
      // A real fieldset, because the word sitting on the rule is what a legend
      // is, and the browser cuts the border for it without any arithmetic here.
      <fieldset className={`rounded-lg border border-[var(--rule)] pt-1 ${open ? "pb-2" : "pb-1"}`}>
        <legend className="ml-[2.25rem] px-1 text-[11px] text-[var(--ink-2)]">{caption}</legend>
        {band}
      </fieldset>
    );
  }

  return (
    <div className="overflow-hidden rounded-lg border border-[var(--rule)]">
      <div className={`bg-[var(--base)] py-1 ${open ? "border-b border-[var(--rule)]" : ""}`}>
        {cap}
      </div>
      {open ? <div className="pt-1.5 pb-2">{band}</div> : null}
    </div>
  );
}
