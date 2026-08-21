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
 * strip, and the swatches repeat the map's own colours so the key works for the
 * map, the strip, and the chart at once.
 *
 * Proportions are of the whole stage, and unsurveyed ground is one of the
 * classes. A key that quietly renormalised over the surveyed part would report a
 * gravel third of a half-surveyed stage as two thirds gravel.
 *
 * Both lists are chips of one toggle group, which is what makes the key a single
 * tab stop with the arrow keys moving inside it: a reader reaches eleven classes
 * with one Tab rather than eleven, and picking a gradient band drops the surface
 * chip that was pressed because the group holds one selection across both lists.
 * The group is a `multiple` one holding at most a single value, deliberately.
 * Radix's `single` group is a radio group, and a radio group promises exactly one
 * choice that cannot be given back — which is the opposite of this key, whose
 * resting state is nothing picked and whose way back is a second press.
 */

import { ToggleGroup } from "radix-ui";
import type { SurfaceKind } from "../api/types";
import type { Highlight } from "../lib/highlight";
import { GRADIENT_BANDS } from "../lib/profile";
import type { SurfaceSummary } from "../lib/surface";
import { SURFACE_STYLES } from "../lib/surface";
import styles from "./RouteKey.module.css";

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

/**
 * What one chip is called within the group.
 *
 * The group speaks in strings, and both lists share its one selection, so the
 * class of the selection has to survive the trip: `gravel` and a band numbered
 * two must not be able to collide on their way through it.
 */
function chipValue(highlight: Highlight): string {
  return highlight.type === "surface" ? `surface:${highlight.kind}` : `band:${highlight.band}`;
}

export interface RouteKeyProps {
  /** Null for a route nobody has classified, which is said in words instead. */
  surface: SurfaceSummary | null;
  surfaceAbsence: string;
  /**
   * The bands this stage actually has, gentlest first.
   *
   * Of the whole stage rather than of the stretch on show, so zooming does not
   * reshuffle the key underneath the reader's hand — and taken from the stage's
   * one classification, so the key offers no class the chart has nothing to
   * light, and lists every class the chart can paint at any zoom level.
   */
  bands: number[];
  highlight: Highlight | null;
  onHighlightChange: (highlight: Highlight | null) => void;
}

export function RouteKey({
  surface,
  surfaceAbsence,
  bands,
  highlight,
  onHighlightChange,
}: RouteKeyProps) {
  // What each chip in the group stands for, built from the classes actually
  // offered. Reading the selection back out of this rather than parsing the
  // string means a value the key never offered cannot become a highlight.
  const offered = new Map<string, Highlight>();
  const surfaceHighlight = (kind: SurfaceKind): Highlight => ({ type: "surface", kind });
  const bandHighlight = (band: number): Highlight => ({ type: "band", band });
  for (const entry of surface?.shares ?? []) {
    offered.set(chipValue(surfaceHighlight(entry.kind)), surfaceHighlight(entry.kind));
  }
  for (const band of bands) {
    offered.set(chipValue(bandHighlight(band)), bandHighlight(band));
  }

  // At most one value travels in, and the group hands back the pressed chip
  // last: a press on an unpressed chip appends it to the one already there, and
  // a press on the pressed chip leaves nothing, which is the whole route back.
  const pick = (values: string[]) => {
    const picked = values.at(-1);
    onHighlightChange(picked ? (offered.get(picked) ?? null) : null);
  };

  return (
    <ToggleGroup.Root
      type="multiple"
      className={styles.key}
      aria-label="Route key"
      data-picked={highlight ? "true" : undefined}
      value={highlight ? [chipValue(highlight)] : []}
      onValueChange={pick}
    >
      {surface ? (
        <ul className={styles.list} aria-label="Surface classes">
          {surface.shares.map((entry) => {
            const style = SURFACE_STYLES[entry.kind];
            const share = formatShare(entry.share);

            return (
              <li key={entry.kind}>
                <ToggleGroup.Item
                  className={styles.chip}
                  value={chipValue(surfaceHighlight(entry.kind))}
                  // What the class means, for a key that has to explain
                  // "compacted" — spoken as part of the name, because a tooltip
                  // is nothing to a keyboard or a finger.
                  title={style.description}
                  aria-label={`${style.label}, ${style.description}, ${share} of the route`}
                >
                  <span className={styles.swatch} data-surface={entry.kind} aria-hidden="true" />
                  <span>{style.label}</span>
                  <span className={styles.share}>{share}</span>
                </ToggleGroup.Item>
              </li>
            );
          })}
        </ul>
      ) : (
        <p className={styles.absent}>{surfaceAbsence}</p>
      )}

      {bands.length > 0 ? (
        <ul className={styles.list} aria-label="Gradient bands">
          {bands.map((band) => (
            <li key={band}>
              <ToggleGroup.Item className={styles.chip} value={chipValue(bandHighlight(band))}>
                <span className={styles.swatch} data-band={band} aria-hidden="true" />
                <span>{GRADIENT_BANDS[band]?.label}</span>
              </ToggleGroup.Item>
            </li>
          ))}
        </ul>
      ) : null}
    </ToggleGroup.Root>
  );
}
