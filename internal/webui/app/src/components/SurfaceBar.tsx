/**
 * The stage's surface split: a proportional bar above a legend that names each
 * class.
 *
 * The bar and the legend are one control, not a chart plus a key. The bar is
 * hidden from assistive technology and the legend carries every figure as text,
 * so the split is readable without seeing a single pixel of it — and the legend
 * doubles as the key to the bands drawn on the map, which is why the swatches
 * repeat the map's dash pattern rather than showing a flat chip.
 *
 * Proportions are of the whole stage, and unsurveyed ground is one of the
 * classes. A bar that quietly renormalised over the surveyed part would report a
 * gravel third of a half-surveyed stage as two thirds gravel.
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
      <div className="surface-bar__track" aria-hidden="true">
        {summary.shares.map((entry) => (
          <span
            key={entry.kind}
            className="surface-bar__slice"
            style={{
              width: `${entry.share * 100}%`,
              background: swatchBackground(entry.kind),
            }}
          />
        ))}
      </div>
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
