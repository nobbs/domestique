/**
 * The one class a reader has picked out, and the ground it covers.
 *
 * The legend beneath the chart is not only a caption: a class can be clicked, and
 * the stretches of route made of it stay lit while the rest of the ride fades.
 * The question it answers — "where is the gravel", "where does it hit twelve
 * percent" — is asked of the map and of the chart at once, so the highlight is
 * held above both and turned into ground here, once, in the two units the two
 * views address ground in: metres along the route for the chart, and point
 * indices for the map.
 *
 * One class at a time, deliberately. A surface and a gradient asked together
 * would mean the ground that is both, which is a different question and usually
 * an empty answer — and a reader who could not tell which of the two had
 * emptied it would have learnt nothing about either.
 */

import type { Position, SurfaceKind, SurfaceRange } from "../api/types";
import type { CoordinateRange, DistanceWindow } from "./profile";
import { cumulativeMetres, GRADIENT_BANDS, gradientRanges } from "./profile";
import type { SurfaceSummary } from "./surface";
import { SURFACE_STYLES } from "./surface";

/** A gradient band, or a surface class. Never both, and never several. */
export type Highlight = { type: "band"; band: number } | { type: "surface"; kind: SurfaceKind };

/** Whether two highlights mean the same class, so the legend can press its own chip. */
export function sameHighlight(left: Highlight | null, right: Highlight | null): boolean {
  if (!left || !right) {
    return left === right;
  }
  if (left.type === "band" && right.type === "band") {
    return left.band === right.band;
  }

  return left.type === "surface" && right.type === "surface" && left.kind === right.kind;
}

/**
 * What the picked class is called.
 *
 * One name, taken from the palette that draws it, so the chart's spoken summary,
 * the collapsed overview's hint, and the pressed chip in the legend cannot come
 * to call the same highlight three different things.
 *
 * A band is named here by the span it covers rather than by the chip's own
 * label: the chips are read as a row and can be terse about it, but "the 3%
 * stretches are lit" in the middle of a sentence would be a different claim
 * from the one the legend is making.
 */
export function highlightLabel(highlight: Highlight): string {
  return highlight.type === "surface"
    ? SURFACE_STYLES[highlight.kind].label
    : (GRADIENT_BANDS[highlight.band]?.description ?? "");
}

/** A stretch of the route measured in metres, as the chart's marks carry it. */
interface Span {
  startMetres: number;
  endMetres: number;
}

/**
 * The ground a stretch of chart does *not* cover, within the stretch on show.
 *
 * The chart fades what was not asked for rather than lighting what was: the
 * terrain is drawn as columns of band colour, edge and all, and brightening
 * a column would change the colour that is the whole point of it. Veiling the
 * gaps leaves every mark exactly the colour it means and simply takes the light
 * off the rest — which is also what the map does outside a zoomed stretch, so
 * the two views go on saying one thing one way.
 *
 * Spans are expected in route order; touching or overlapping ones are absorbed
 * rather than left to produce a gap of negative width.
 */
export function gapsOutside(
  spans: readonly Span[],
  startMetres: number,
  endMetres: number,
): DistanceWindow[] {
  const gaps: DistanceWindow[] = [];
  let cursor = startMetres;

  for (const span of spans) {
    if (span.startMetres > cursor) {
      gaps.push({ startMetres: cursor, endMetres: Math.min(span.startMetres, endMetres) });
    }
    cursor = Math.max(cursor, span.endMetres);
    if (cursor >= endMetres) {
      return gaps.filter((gap) => gap.endMetres > gap.startMetres);
    }
  }
  gaps.push({ startMetres: cursor, endMetres });

  return gaps.filter((gap) => gap.endMetres > gap.startMetres);
}

/**
 * A run of segments as the points that bound it.
 *
 * Both the surface classification and the steepness runs address the ground
 * between two points by the index of the segment it starts at, so the last point
 * of a run is one past its `endIndex` — and is the first point of the run that
 * takes over from it, which is what makes neighbouring stretches meet.
 */
function pointsOf(
  range: { startIndex: number; endIndex: number },
  lastIndex: number,
): CoordinateRange {
  const startIndex = Math.min(Math.max(range.startIndex, 0), lastIndex);

  return {
    startIndex,
    endIndex: Math.min(Math.max(range.endIndex + 1, startIndex), lastIndex),
  };
}

/**
 * Where on the route the selected class is, as stretches of the coordinates.
 *
 * Empty when the stage has none of it, which the map draws as a route with
 * nothing lit — the honest answer to asking for ground that is not there.
 */
export function highlightRanges(
  coordinates: Position[],
  surfaceRanges: readonly SurfaceRange[],
  highlight: Highlight,
): CoordinateRange[] {
  const lastIndex = coordinates.length - 1;
  if (lastIndex < 1) {
    return [];
  }
  const ranges =
    highlight.type === "surface"
      ? surfaceRanges.filter((range) => range.kind === highlight.kind)
      : gradientRanges(coordinates).filter((range) => range.band === highlight.band);

  return ranges
    .map((range) => pointsOf(range, lastIndex))
    .filter((range) => range.endIndex > range.startIndex);
}

/**
 * The ground both restrictions agree on, as disjoint stretches in route order.
 *
 * A zoomed chart and a picked class restrict the map for different reasons, and
 * what stays lit is what satisfies both: a reader looking at two kilometres of
 * route who asks for gravel means the gravel in those two kilometres.
 */
export function intersectRanges(
  left: readonly CoordinateRange[],
  right: readonly CoordinateRange[],
): CoordinateRange[] {
  const overlaps: CoordinateRange[] = [];
  for (const first of left) {
    for (const second of right) {
      const startIndex = Math.max(first.startIndex, second.startIndex);
      const endIndex = Math.min(first.endIndex, second.endIndex);
      if (endIndex > startIndex) {
        overlaps.push({ startIndex, endIndex });
      }
    }
  }

  return overlaps.sort((first, second) => first.startIndex - second.startIndex);
}

/**
 * The stretches of route drawn at full strength, or null when that is all of it.
 *
 * Null rather than one range covering everything, because "nothing is dimmed" is
 * a different state from "everything happens to be lit": it is what lets the map
 * skip the paint expression entirely, and what keeps the halo off a route nobody
 * has asked a question of.
 */
export function litRanges(
  window: CoordinateRange | null,
  highlighted: CoordinateRange[] | null,
): CoordinateRange[] | null {
  if (!highlighted) {
    return window ? [window] : null;
  }

  return window ? intersectRanges(highlighted, [window]) : highlighted;
}

/** Where the selected class is, as metre spans in route order. */
export function highlightSpans(
  coordinates: Position[],
  surface: SurfaceSummary | null,
  highlight: Highlight,
): DistanceWindow[] {
  if (highlight.type === "surface") {
    return (surface?.bands ?? [])
      .filter((band) => band.kind === highlight.kind)
      .map((band) => ({ startMetres: band.startMetres, endMetres: band.endMetres }));
  }

  const lastIndex = coordinates.length - 1;
  if (lastIndex < 1) {
    return [];
  }
  const distances = cumulativeMetres(coordinates);
  const lastDistance = distances[distances.length - 1] as number;

  return gradientRanges(coordinates)
    .filter((range) => range.band === highlight.band)
    .map((range) => ({
      startMetres: distances[range.startIndex] as number,
      endMetres: distances[Math.min(range.endIndex + 1, distances.length - 1)] ?? lastDistance,
    }));
}

/**
 * The span after the one nearest the current window, wrapping past the last.
 *
 * "Nearest middle" rather than "next span starting after the window" — the
 * window on show came out of `widened()`, which can shift or clamp its edges,
 * so a start-based comparison can land back on the span already selected.
 */
export function nextSpan(
  spans: readonly DistanceWindow[],
  window: DistanceWindow | null,
): DistanceWindow | null {
  if (spans.length === 0) {
    return null;
  }
  if (!window) {
    return spans[0] as DistanceWindow;
  }

  const middleOf = (span: DistanceWindow) => (span.startMetres + span.endMetres) / 2;
  const target = middleOf(window);
  let nearest = 0;
  let nearestDistance = Number.POSITIVE_INFINITY;
  spans.forEach((span, index) => {
    const distance = Math.abs(middleOf(span) - target);
    if (distance < nearestDistance) {
      nearest = index;
      nearestDistance = distance;
    }
  });

  return spans[(nearest + 1) % spans.length] as DistanceWindow;
}
