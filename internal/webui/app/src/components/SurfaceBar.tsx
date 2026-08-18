/**
 * The stage's surface split, as a key: every class present, what share of the
 * stage it covers, and the mark it wears.
 *
 * It carries no bar of its own. The proportions are drawn along the foot of the
 * elevation chart, in the order they are ridden, which says everything a
 * summary bar said and also says *where* — so a second bar beside it would be
 * the same figures twice, one of them in the wrong order.
 *
 * Every figure is text, so the split is readable without seeing a pixel of the
 * strip, and the swatches repeat the map's colour and dash pattern so the key
 * works for the map, the strip, and the legend at once.
 *
 * Proportions are of the whole stage, and unsurveyed ground is one of the
 * classes. A legend that quietly renormalised over the surveyed part would
 * report a gravel third of a half-surveyed stage as two thirds gravel.
 */

import type { SurfaceSummary } from "../lib/surface";
import { SURFACE_STYLES, swatchBackground } from "../lib/surface";

/**
 * A share as a percentage, never rounded into a contradiction.
 *
 * A kilometre of gravel in a hundred-kilometre stage is worth knowing about, and
 * "0%" beside a visible band on the map reads as a bug rather than as a small
 * number. The same holds at the other end: rounding the rest to "100%" while
 * another class is still listed beside it makes the legend argue with itself.
 */
function formatShare(share: number): string {
  const percent = share * 100;
  if (percent > 0 && percent < 1) {
    return "<1%";
  }
  if (percent > 99 && percent < 100) {
    return ">99%";
  }

  return `${Math.round(percent)}%`;
}

export function SurfaceBar({ summary }: { summary: SurfaceSummary }) {
  return (
    <div className="surface-bar">
      <ul className="surface-bar__legend">
        {summary.shares.map((entry) => (
          <li key={entry.kind}>
            <span
              className="surface-bar__swatch"
              style={{ background: swatchBackground(entry.kind) }}
              aria-hidden="true"
            />
            <span>{SURFACE_STYLES[entry.kind].label}</span>
            <span className="surface-bar__share">{formatShare(entry.share)}</span>
          </li>
        ))}
      </ul>
    </div>
  );
}
