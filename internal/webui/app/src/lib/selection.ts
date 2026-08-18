/**
 * Picking a stretch of the route to look at.
 *
 * The chart and the map both offer the same gesture — drag across the ground you
 * want a closer look at — and they have to settle on the same stretch for it, or
 * a reader who drew two hundred metres on one instrument and the same two
 * hundred on the other would be shown two different rides. So the parts of the
 * gesture that are about the route rather than about the view live here: how far
 * a hand must travel before it means anything, how short a window may be, and
 * how a window too short to plot is grown.
 *
 * What each view keeps for itself is the reading of a position — the chart maps
 * a pixel along its axis to metres, the map takes the nearest point of the line
 * — because that is the one part of the gesture the two do genuinely differently.
 */

import type { DistanceWindow } from "./profile";

/**
 * How far the pointer must travel before a scrub becomes a selection.
 *
 * A hand resting on a trackpad moves a pixel or two and a finger never lands
 * still; treating that as a range would zoom every time somebody looked at the
 * page. Eight pixels is past the tremble and well under the shortest swipe
 * anybody makes on purpose — and it is a distance rather than a duration, so
 * nothing here waits on a clock to decide what a gesture was.
 */
export const MIN_DRAG_PIXELS = 8;

/**
 * The shortest stretch a drag may settle on.
 *
 * Gradient is measured over a hundred metres, so a window much shorter than a
 * couple of those is one measurement drawn three hundred times. A selection
 * under it is grown about its middle rather than refused: the reader asked to
 * look closer at somewhere, and the answer to "closer than the data goes" is
 * the closest the data goes, not nothing.
 */
export const MIN_WINDOW_METRES = 200;

/**
 * How near the route a pointer counts as on it, in pixels.
 *
 * The painted line is a few pixels wide, which is a pinpoint to aim at. Testing
 * against the projected position instead gives a hit area comfortably larger
 * than the mark, so following the route with the pointer actually works — and it
 * is the same distance that decides whether a drag is a selection or a pan, so
 * the ground that marks a position is the ground that can be dragged across.
 */
export const NEAR_ROUTE_PIXELS = 22;

/**
 * The same, for a fingertip.
 *
 * A finger covers a good deal more of the screen than it can aim with, and the
 * reader cannot see the line under it while it is down. Asking a hand for the
 * same accuracy a cursor gives would make the gesture something to attempt
 * rather than something to use — and the cost of the wider band is only that
 * the page is scrolled from beside the route rather than from on top of it.
 */
export const NEAR_ROUTE_TOUCH_PIXELS = 32;

/** A stretch between two positions on the route, in the order it is ridden. */
export function spanBetween(anchorMetres: number, metres: number): DistanceWindow {
  return {
    startMetres: Math.min(anchorMetres, metres),
    endMetres: Math.max(anchorMetres, metres),
  };
}

/**
 * A selection too short to plot is grown about its middle rather than refused,
 * and slid back inside the route rather than truncated — a window that ran off
 * the start would otherwise arrive shorter than the minimum it was grown to.
 */
export function widened(window: DistanceWindow, totalMetres: number): DistanceWindow {
  const span = window.endMetres - window.startMetres;
  if (span >= MIN_WINDOW_METRES) {
    return window;
  }
  const wanted = Math.min(MIN_WINDOW_METRES, totalMetres);
  const middle = (window.startMetres + window.endMetres) / 2;
  const start = Math.min(Math.max(middle - wanted / 2, 0), Math.max(totalMetres - wanted, 0));

  return { startMetres: start, endMetres: start + wanted };
}
