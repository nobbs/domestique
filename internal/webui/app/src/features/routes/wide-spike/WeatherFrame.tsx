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

function Caption({ children }: { children: ReactNode }) {
  return (
    <p
      className="text-[11px] leading-none text-[var(--ink-2)]"
      style={{ paddingLeft: PADDING.left }}
    >
      {children}
    </p>
  );
}

export function WeatherFrame({
  variant,
  caption,
  children,
}: {
  variant: WeatherFrameVariant;
  caption: string;
  children: ReactNode;
}) {
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
        <Caption>{caption}</Caption>
        {children}
      </div>
    );
  }

  if (variant === "card") {
    return (
      <div className="rounded-lg bg-[var(--base)] py-2">
        <div className="mb-1.5">
          <Caption>{caption}</Caption>
        </div>
        {children}
      </div>
    );
  }

  if (variant === "legend") {
    return (
      // A real fieldset, because the word sitting on the rule is what a legend
      // is, and the browser cuts the border for it without any arithmetic here.
      <fieldset className="rounded-lg border border-[var(--rule)] pt-1 pb-2">
        <legend className="ml-[2.25rem] px-1 text-[11px] text-[var(--ink-2)]">{caption}</legend>
        {children}
      </fieldset>
    );
  }

  return (
    <div className="overflow-hidden rounded-lg border border-[var(--rule)]">
      <div className="border-b border-[var(--rule)] bg-[var(--base)] py-1">
        <Caption>{caption}</Caption>
      </div>
      <div className="pt-1.5 pb-2">{children}</div>
    </div>
  );
}
