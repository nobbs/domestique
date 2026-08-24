/**
 * The one distance axis every chart drawn against a stage's geometry shares.
 *
 * The elevation profile maps distance to a pixel inside its own `geometry`
 * memo, and the forecast strip has to land its cells on exactly the same
 * pixels — a cell drawn from a second implementation of this arithmetic would
 * eventually disagree with the terrain under it by a rounding error nobody
 * could explain. So there is one function, and both charts call it.
 *
 * `PADDING` and `MIN_WIDTH` travel with it rather than staying private to the
 * elevation chart: the room reserved down the left for a metre axis is also
 * the room the strip's cells must not start inside, or its first cell would
 * sit under the profile's own tick labels rather than under the terrain they
 * describe.
 */

/**
 * The room around the plotted terrain: metre labels down the left, kilometre
 * labels along the foot, and enough at the top and right that a peak or a
 * cell touching the edge is not clipped by it.
 *
 * A shared measurement, not a shared layout — a chart that draws no tick
 * labels of its own still reserves the same left and right margin, which is
 * what keeps its horizontal axis lined up with the chart that does.
 */
export const PADDING = { top: 8, right: 8, bottom: 22, left: 40 } as const;

/** Below this, a card too narrow to measure yet is drawn at this width instead. */
export const MIN_WIDTH = 240;

/** The horizontal axis a chart plots distance against. */
export interface PlotAxis {
  /** The pixel width available for the terrain itself, padding already removed. */
  plotWidth: number;
  /** A distance along the stretch on show, as a pixel offset from its left edge. */
  x: (metres: number) => number;
}

/**
 * The axis a chart of `width` measured pixels draws the stretch from
 * `startMetres` to `endMetres` against.
 *
 * `x` is monotonic, maps `startMetres` to 0 and `endMetres` to `plotWidth`,
 * and a stretch of no length still gets one — dividing by zero would put every
 * mark at the same pixel rather than draw a stretch that has not loaded yet as
 * a stretch of no length.
 */
export function plotAxis(width: number, startMetres: number, endMetres: number): PlotAxis {
  const plotWidth = Math.max(width, MIN_WIDTH) - PADDING.left - PADDING.right;
  const shown = Math.max(endMetres - startMetres, 1);

  return {
    plotWidth,
    x: (metres: number) => ((metres - startMetres) / shown) * plotWidth,
  };
}
