/**
 * The key to the two encodings drawn along a route: the ground it is made of,
 * and how steeply it climbs.
 *
 * Two named sections, gradient first, each a proportion bar with the classes
 * that make it up listed underneath. The heading is what makes a row of
 * swatches answer the question it is the answer to: `6% 22%` under nothing at
 * all is a figure, and under `Gradient` it is a fact about the ride. Steepness
 * comes first because it is the thing a rider chooses a route by; the ground it
 * is made of decides which bike, which is the second question.
 *
 * Every entry is a control, not a caption. Clicking one asks where that class
 * is: its stretches stay lit on the map and in the chart while the rest of the
 * ride fades, which is the question a key has always implied and never answered.
 * Clicking it again gives the whole route back.
 *
 * Each section carries a bar of its own, one segment per class, in class order
 * rather than in ride order. It is deliberately not the strip along the foot of
 * the chart: that strip says *where*, this bar says *how much*, and the two
 * cannot be read for each other's question. The bar also survives the chart
 * being collapsed to a pill, which is when a reader has only this panel left.
 *
 * Every figure is text as well, so the split is readable without seeing a pixel
 * of either bar, and the swatches repeat the map's own colours so the key works
 * for the map, the strip, and the chart at once.
 *
 * Proportions are of the whole route, and unsurveyed ground is one of the
 * classes. A key that quietly renormalised over the surveyed part would report a
 * gravel third of a half-surveyed route as two thirds gravel.
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

import type { SurfaceKind } from "../../api/types";
import { ToggleGroup } from "../../components/ui/toggle-group";
import type { Highlight } from "../../lib/highlight";
import type { BandShare } from "../../lib/profile";
import { GRADIENT_BANDS } from "../../lib/profile";
import type { SurfaceSummary } from "../../lib/surface";
import { SURFACE_STYLES } from "../../lib/surface";
import { LegendChip } from "./LegendChip";
import { MixBar } from "./MixBar";

const paintClasses = {
  "band:0": "bg-[var(--grade-0)]",
  "band:1": "bg-[var(--grade-1)]",
  "band:2": "bg-[var(--grade-2)]",
  "band:3": "bg-[var(--grade-3)]",
  "band:4": "bg-[var(--grade-4)]",
  "surface:asphalt": "bg-[var(--surface-asphalt)]",
  "surface:paving": "bg-[var(--surface-paving)]",
  "surface:compacted": "bg-[var(--surface-compacted)]",
  "surface:gravel": "bg-[var(--surface-gravel)]",
  "surface:ground": "bg-[var(--surface-ground)]",
  "surface:unknown": "bg-[var(--surface-unsurveyed)]",
} as const;

/**
 * A share as a percentage, never rounded into a contradiction.
 *
 * A kilometre of gravel in a hundred-kilometre route is worth knowing about, and
 * "0%" beside a visible band on the map reads as a bug rather than as a small
 * number. The same holds at the other end: rounding the rest to "100%" while
 * another class is still listed beside it makes the key argue with itself.
 */
/**
 * A share as a bar segment's width.
 *
 * A tenth of a percent is finer than any bar this size can draw, and rounding
 * there keeps a segment from arriving in the DOM as `49.999999999911175%`.
 */
function segmentWidth(share: number): string {
  return `${(share * 100).toFixed(1)}%`;
}

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

export interface RouteLegendProps {
  /** Null for a route nobody has classified, which is said in words instead. */
  surface: SurfaceSummary | null;
  surfaceAbsence: string;
  /**
   * The bands this route actually has and their shares of it, gentlest first.
   *
   * Of the whole route rather than of the stretch on show, so zooming does not
   * reshuffle the key underneath the reader's hand — and taken from the route's
   * one classification, so the key offers no class the chart has nothing to
   * light, and lists every class the chart can paint at any zoom level.
   */
  bands: BandShare[];
  highlight: Highlight | null;
  onHighlightChange: (highlight: Highlight | null) => void;
}

export function RouteLegend({
  surface,
  surfaceAbsence,
  bands,
  highlight,
  onHighlightChange,
}: RouteLegendProps) {
  // What each chip in the group stands for, built from the classes actually
  // offered. Reading the selection back out of this rather than parsing the
  // string means a value the key never offered cannot become a highlight.
  const offered = new Map<string, Highlight>();
  const surfaceHighlight = (kind: SurfaceKind): Highlight => ({ type: "surface", kind });
  const bandHighlight = (band: number): Highlight => ({ type: "band", band });
  for (const entry of bands) {
    offered.set(chipValue(bandHighlight(entry.band)), bandHighlight(entry.band));
  }
  for (const entry of surface?.shares ?? []) {
    offered.set(chipValue(surfaceHighlight(entry.kind)), surfaceHighlight(entry.kind));
  }

  // At most one value travels in, and the group hands back the pressed chip
  // last: a press on an unpressed chip appends it to the one already there, and
  // a press on the pressed chip leaves nothing, which is the whole route back.
  const pick = (values: string[]) => {
    const picked = values.at(-1);
    onHighlightChange(picked ? (offered.get(picked) ?? null) : null);
  };

  return (
    <ToggleGroup
      className="flex w-full flex-col items-stretch gap-3 [&_[aria-pressed=false]]:data-[picked=true]:opacity-55"
      aria-label="Route key"
      data-picked={highlight ? "true" : undefined}
      value={highlight ? [chipValue(highlight)] : []}
      onValueChange={pick}
    >
      {bands.length > 0 ? (
        <div className="flex flex-col gap-1">
          <h3 className="text-[11px] font-semibold tracking-[0.07em] text-[var(--ink-2)] uppercase">
            Gradient
          </h3>
          <MixBar>
            {bands.map((entry) => (
              <span
                key={entry.band}
                className={`block min-w-px ${paintClasses[`band:${entry.band}` as keyof typeof paintClasses]}`}
                data-band={entry.band}
                style={{ width: segmentWidth(entry.share) }}
              />
            ))}
          </MixBar>
          <ul
            className="flex flex-wrap gap-x-2 gap-y-1 text-xs text-[var(--ink-2)] tabular-nums"
            aria-label="Gradient bands"
          >
            {bands.map((entry) => {
              const band = GRADIENT_BANDS[entry.band];
              const share = formatShare(entry.share);

              return (
                <LegendChip
                  key={entry.band}
                  value={chipValue(bandHighlight(entry.band))}
                  paintClassName={paintClasses[`band:${entry.band}` as keyof typeof paintClasses]}
                  title={band?.description}
                  ariaLabel={`${band?.label}, ${band?.description}, ${share} of the route`}
                  label={band?.label}
                  share={share}
                  swatch={{ band: entry.band }}
                />
              );
            })}
          </ul>
        </div>
      ) : null}

      <div className="flex flex-col gap-1">
        <h3 className="text-[11px] font-semibold tracking-[0.07em] text-[var(--ink-2)] uppercase">
          Surface
        </h3>
        {surface ? (
          <>
            <MixBar>
              {surface.shares.map((entry) => (
                <span
                  key={entry.kind}
                  className={`block min-w-px ${paintClasses[`surface:${entry.kind}` as keyof typeof paintClasses]}`}
                  data-surface={entry.kind}
                  style={{ width: segmentWidth(entry.share) }}
                />
              ))}
            </MixBar>
            <ul
              className="flex flex-wrap gap-x-2 gap-y-1 text-xs text-[var(--ink-2)] tabular-nums"
              aria-label="Surface classes"
            >
              {surface.shares.map((entry) => {
                const style = SURFACE_STYLES[entry.kind];
                const share = formatShare(entry.share);

                return (
                  <LegendChip
                    key={entry.kind}
                    value={chipValue(surfaceHighlight(entry.kind))}
                    paintClassName={
                      paintClasses[`surface:${entry.kind}` as keyof typeof paintClasses]
                    }
                    // What the class means, for a key that has to explain
                    // "compacted" — spoken as part of the name, because a
                    // tooltip is nothing to a keyboard or a finger.
                    title={style.description}
                    ariaLabel={`${style.label}, ${style.description}, ${share} of the route`}
                    label={style.label}
                    share={share}
                    swatch={{ surface: entry.kind }}
                  />
                );
              })}
            </ul>
          </>
        ) : (
          <p className="text-xs text-[var(--ink-2)]">{surfaceAbsence}</p>
        )}
      </div>
    </ToggleGroup>
  );
}
