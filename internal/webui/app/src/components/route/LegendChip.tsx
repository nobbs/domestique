import type { ReactNode } from "react";
import type { SurfaceKind } from "../../api/types";
import { ToggleGroupItem } from "../ui/toggle-group";

export interface LegendChipProps {
  /** What this chip is called within the toggle group it presses in. */
  value: string;
  /** The swatch's fill — one of `RouteLegend`'s `paintClasses` entries. */
  paintClassName: string;
  title: string | undefined;
  ariaLabel: string;
  label: ReactNode;
  share: string;
  /** Which list the chip is in, and the swatch's own data attribute for it. */
  swatch: { band: number } | { surface: SurfaceKind };
}

/**
 * One chip in the route legend: a swatch, a class name, and the share of the
 * route it covers. A control, not a caption — see the module doc on
 * `RouteLegend` for why it presses as a toggle rather than sitting there as
 * text, and why it has to live inside a `ToggleGroup` to do that.
 */
export function LegendChip({
  value,
  paintClassName,
  title,
  ariaLabel,
  label,
  share,
  swatch,
}: LegendChipProps) {
  return (
    <li>
      <ToggleGroupItem
        className="-m-0.5 h-7 min-w-0 gap-1 rounded-md border border-transparent bg-transparent px-1.5 text-xs font-normal text-[var(--ink-2)] hover:border-[var(--rule)] hover:bg-transparent hover:text-[var(--ink)] focus-visible:ring-[var(--accent)] aria-pressed:border-[var(--accent)] aria-pressed:bg-[color-mix(in_srgb,var(--accent)_14%,transparent)] aria-pressed:text-[var(--ink)]"
        value={value}
        // The chips are read as a row of five and are terse about it, so the
        // span each one covers is spoken rather than written: "6%" on the
        // chip, "6 to 9%" in the name.
        title={title}
        aria-label={ariaLabel}
      >
        <span
          className={`size-4 shrink-0 rounded-sm border border-transparent forced-colors:border-[CanvasText] forced-colors:forced-color-adjust-none ${paintClassName}`}
          {...("band" in swatch
            ? { "data-band": swatch.band }
            : { "data-surface": swatch.surface })}
          aria-hidden="true"
        />
        <span>{label}</span>
        <span className="text-[var(--ink)]">{share}</span>
      </ToggleGroupItem>
    </li>
  );
}
