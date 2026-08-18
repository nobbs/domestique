/**
 * The key to the two encodings drawn along a stage: the ground it is made of,
 * and how steeply it climbs.
 *
 * One row, both keys, sharing a line wherever the pane is wide enough for them.
 * They explain marks that share an axis — the surface strip runs along the foot
 * of the elevation chart, directly under the terrain it belongs to — and keys
 * for one instrument stacked on separate lines read as two instruments. Short of
 * room the steepness key drops beneath the surface key rather than shrinking
 * either, because a key that wraps is still read and a key set in six-point type
 * is not.
 *
 * Every entry is a control, not a caption. Clicking one asks where that class
 * is: its stretches stay lit on the map and in the chart while the rest of the
 * ride fades, which is the question a key has always implied and never answered.
 * Clicking it again gives the whole route back.
 *
 * The surface key carries no bar of its own. The proportions are drawn along the
 * foot of the chart, in the order they are ridden, which says everything a
 * summary bar said and also says *where* — so a second bar beside it would be
 * the same figures twice, one of them in the wrong order.
 *
 * Every figure is text, so the split is readable without seeing a pixel of the
 * strip, and the swatches repeat the map's colour and dash pattern so the key
 * works for the map, the strip, and the chart at once.
 *
 * Proportions are of the whole stage, and unsurveyed ground is one of the
 * classes. A key that quietly renormalised over the surveyed part would report a
 * gravel third of a half-surveyed stage as two thirds gravel.
 */

import type { Highlight } from "../lib/highlight";
import { sameHighlight } from "../lib/highlight";
import { GRADIENT_BANDS } from "../lib/profile";
import type { SurfaceSummary } from "../lib/surface";
import { SURFACE_STYLES, swatchBackground } from "../lib/surface";

/**
 * A share as a percentage, never rounded into a contradiction.
 *
 * A kilometre of gravel in a hundred-kilometre stage is worth knowing about, and
 * "0%" beside a visible band on the map reads as a bug rather than as a small
 * number. The same holds at the other end: rounding the rest to "100%" while
 * another class is still listed beside it makes the key argue with itself.
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

export interface StageKeyProps {
  /** Null for a stage nobody has classified, which is said in words instead. */
  surface: SurfaceSummary | null;
  surfaceAbsence: string;
  /**
   * The bands this stage actually has, gentlest first.
   *
   * Of the whole stage rather than of the stretch on show, so zooming does not
   * reshuffle the key underneath the reader's hand — and taken from the profile,
   * so the key offers no class the chart has nothing to light.
   */
  bands: number[];
  highlight: Highlight | null;
  onHighlightChange: (highlight: Highlight | null) => void;
}

export function StageKey({
  surface,
  surfaceAbsence,
  bands,
  highlight,
  onHighlightChange,
}: StageKeyProps) {
  // A second click on the pressed entry is the way back, which is why these are
  // toggles rather than a set of radio buttons: there is a state with nothing
  // picked, and it is the one the page opens in.
  const pick = (next: Highlight) => onHighlightChange(sameHighlight(highlight, next) ? null : next);

  return (
    <div className="stage-key" data-picked={highlight ? "true" : undefined}>
      {surface ? (
        <ul className="stage-key__list" aria-label="Surface classes">
          {surface.shares.map((entry) => {
            const style = SURFACE_STYLES[entry.kind];
            const share = formatShare(entry.share);

            return (
              <li key={entry.kind}>
                <button
                  type="button"
                  className="stage-key__chip"
                  aria-pressed={sameHighlight(highlight, { type: "surface", kind: entry.kind })}
                  // What the class means, for a key that has to explain
                  // "compacted" — spoken as part of the name, because a tooltip
                  // is nothing to a keyboard or a finger.
                  title={style.description}
                  aria-label={`${style.label}, ${style.description}, ${share} of the stage`}
                  onClick={() => pick({ type: "surface", kind: entry.kind })}
                >
                  <span
                    className="stage-key__swatch"
                    style={{ background: swatchBackground(entry.kind) }}
                    aria-hidden="true"
                  />
                  <span>{style.label}</span>
                  <span className="stage-key__share">{share}</span>
                </button>
              </li>
            );
          })}
        </ul>
      ) : (
        <p className="stage-key__absent">{surfaceAbsence}</p>
      )}

      {bands.length > 0 ? (
        <ul className="stage-key__list" aria-label="Gradient bands">
          {bands.map((band) => (
            <li key={band}>
              <button
                type="button"
                className="stage-key__chip"
                aria-pressed={sameHighlight(highlight, { type: "band", band })}
                onClick={() => pick({ type: "band", band })}
              >
                <span className="stage-key__swatch" data-band={band} aria-hidden="true" />
                <span>{GRADIENT_BANDS[band]?.label}</span>
              </button>
            </li>
          ))}
        </ul>
      ) : null}
    </div>
  );
}
