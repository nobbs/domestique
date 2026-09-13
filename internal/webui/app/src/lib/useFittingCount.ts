/**
 * How many of a row's items fit beside the control that holds the rest.
 *
 * A row of named links has no width it can be asked to be: the names are as
 * long as they are, and a bar that lets them decide either wraps to two lines
 * or runs off the end of a phone. So the row keeps the items it has room for
 * and hands the remainder to one trailing control, and this is the arithmetic
 * that says where the line falls.
 *
 * It reads a second copy of the row rather than the row on screen, because the
 * row on screen is already the answer: an item hidden for want of room has no
 * width left to measure, so measuring the visible row would only ever confirm
 * the decision it is meant to revisit. The copy holds every item and the
 * trailing control at natural width, is laid out but never painted, and so
 * reports the same numbers whatever the split currently is — which is also what
 * keeps this from oscillating.
 *
 * Both elements are watched: the frame changes with the viewport, and the copy
 * changes when the web font swaps in and every name is suddenly a different
 * width than it was measured at.
 */

import { useLayoutEffect, useState } from "react";

/**
 * The frame's width against the copy's cumulative extents.
 *
 * The copy's children are the items in order followed by the trailing control,
 * so the gap between neighbours comes free: the extents are read from the left
 * edge of the copy, and the space the control needs is its own width plus the
 * gap in front of it.
 */
function fittingCount(frame: HTMLElement, measure: HTMLElement, total: number): number {
  const rects = [...measure.children].map((child) => child.getBoundingClientRect());
  const control = rects.at(-1);
  const last = rects.at(total - 1);

  // Mid-render the copy can hold a different number of items than the caller
  // has asked about; showing everything is the answer that never hides a
  // destination, and the next measurement corrects it.
  if (total === 0 || rects.length !== total + 1 || !control || !last) {
    return total;
  }

  // `getBoundingClientRect` rather than `clientWidth` on the frame: the frame's
  // width is fractional, and a rounded-down integer collapses a row that fits.
  const available = frame.getBoundingClientRect().width;
  const origin = measure.getBoundingClientRect().left;

  if (last.right - origin <= available) {
    return total;
  }
  const reserved = control.width + (control.left - last.right);

  for (let count = total - 1; count > 0; count--) {
    const end = rects.at(count - 1);

    if (end && end.right - origin + reserved <= available) {
      return count;
    }
  }

  return 0;
}

export interface FittingCount {
  /** The row the items have to fit inside. Its width is the budget. */
  frameRef: (element: HTMLElement | null) => void;
  /** The unpainted copy: every item, in order, then the trailing control. */
  measureRef: (element: HTMLElement | null) => void;
  /** How many of the first `total` items to show. */
  visible: number;
}

/**
 * Callback refs holding state rather than object refs, so measuring starts when
 * the elements arrive rather than at whatever a width of zero decides. An
 * environment without `ResizeObserver` — jsdom — measures once and stays there,
 * which in a suite that lays nothing out means everything fits.
 */
export function useFittingCount(total: number): FittingCount {
  const [frame, setFrame] = useState<HTMLElement | null>(null);
  const [measure, setMeasure] = useState<HTMLElement | null>(null);
  const [visible, setVisible] = useState(total);

  useLayoutEffect(() => {
    if (!frame || !measure) {
      setVisible(total);

      return;
    }
    const measureNow = () => setVisible(fittingCount(frame, measure, total));
    measureNow();

    if (typeof ResizeObserver === "undefined") {
      return;
    }
    const observer = new ResizeObserver(measureNow);
    observer.observe(frame);
    observer.observe(measure);

    return () => observer.disconnect();
  }, [frame, measure, total]);

  // The count is a render behind a change in `total` — an admin link arriving
  // with the configuration — and a stale larger number would ask for items the
  // caller does not have.
  return { frameRef: setFrame, measureRef: setMeasure, visible: Math.min(visible, total) };
}
