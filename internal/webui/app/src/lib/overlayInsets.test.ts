/**
 * What the panels take, and what the camera is left to frame in.
 *
 * The geometry is asked in pixels rather than through a layout: jsdom lays
 * nothing out, so the boxes are handed over directly. What is being tested is
 * the rule for reading them — which side a panel is docked against, and how
 * much of the frame it may have.
 */

import { describe, expect, it } from "vitest";
import type { Box, Insets } from "./overlayInsets";
import { framePadding, insetsFrom, NO_INSETS } from "./overlayInsets";

/** The pane, at a comfortable desktop size. */
const FRAME: Box = { top: 0, right: 1280, bottom: 800, left: 0, width: 1280, height: 800 };

function box(left: number, top: number, width: number, height: number): Box {
  return { top, right: left + width, bottom: top + height, left, width, height };
}

/** The column of panels down the left, as the wide layout places it. */
const COLUMN = box(20, 20, 404, 620);

/** The chart across the foot, beside that column. */
const CHART = box(444, 620, 816, 160);

describe("insetsFrom", () => {
  it("takes nothing for a map nothing is standing on", () => {
    expect(insetsFrom(FRAME, [])).toEqual(NO_INSETS);
  });

  // A column eats its own width from the left, not its own height from the top.
  it("gives a column the side it is docked against", () => {
    expect(insetsFrom(FRAME, [COLUMN])).toEqual({ ...NO_INSETS, left: 424 });
  });

  it("gives a panel across the foot the bottom rather than most of the pane", () => {
    expect(insetsFrom(FRAME, [CHART])).toEqual({ ...NO_INSETS, bottom: 180 });
  });

  it("reads a whole layout at once", () => {
    expect(insetsFrom(FRAME, [COLUMN, CHART])).toEqual({ ...NO_INSETS, left: 424, bottom: 180 });
  });

  // The collapsed chart is a pill in the bottom-right corner: short and narrow,
  // and closer to the foot than to the right edge.
  /*
   * The card the atlas opens a route into: wider than it is tall, in the
   * top-left corner. Judged by reach alone it looks shallower from the top and
   * is handed the whole width of the pane for it — which frames every route
   * into whatever band is left between it and the dock on the foot.
   */
  it("gives a corner card the side that costs the least map", () => {
    expect(insetsFrom(FRAME, [box(12, 12, 470, 340)])).toEqual({ ...NO_INSETS, left: 482 });
  });

  /*
   * The same card once its two lists are folded away: 368 wide and barely 200
   * tall, in the corner, with the dock already across the foot. Judged against
   * the whole pane the top band is the cheaper of the two — but the dock has
   * shortened the pane, and against what is left the card is a column again.
   *
   * Charged against the whole frame instead, this returns `top: 216`, and the
   * camera frames every route into the strip between the card and the dock.
   */
  it("charges a panel against the map its neighbours have left", () => {
    expect(insetsFrom(FRAME, [box(12, 12, 368, 204), CHART])).toEqual({
      ...NO_INSETS,
      left: 380,
      bottom: 180,
    });
  });

  it("gives a pill in a corner the nearer of its two edges", () => {
    expect(insetsFrom(FRAME, [box(1060, 736, 200, 44)])).toEqual({ ...NO_INSETS, bottom: 64 });
  });

  // The wordmark stands inside the column, not beside it: it is behind ground
  // the map has already given up, and asking for a strip off the top as well
  // would hold the camera out of a corner nothing is covering.
  it("asks for nothing more for a panel behind another one's strip", () => {
    expect(insetsFrom(FRAME, [COLUMN, box(20, 20, 404, 60)])).toEqual({
      ...NO_INSETS,
      left: 424,
    });
  });

  it("keeps the deepest reach when two panels share a side", () => {
    expect(insetsFrom(FRAME, [box(20, 20, 300, 620), COLUMN])).toEqual({
      ...NO_INSETS,
      left: 424,
    });
  });

  // A panel that has not been laid out has no box worth reading, and one that
  // is measured before its content arrives would otherwise claim an edge.
  it("ignores a panel with no size", () => {
    expect(insetsFrom(FRAME, [box(0, 0, 0, 0)])).toEqual(NO_INSETS);
  });

  // The page's own heading sits in the overlay beside the panels: a strip a
  // pixel high that only a screen reader ever meets, and nothing the map has to
  // hold its camera out of.
  it("ignores the hidden heading standing among the panels", () => {
    expect(insetsFrom(FRAME, [box(20, 20, 404, 1)])).toEqual(NO_INSETS);
  });
});

describe("framePadding", () => {
  it("adds the gutter to every side of an empty pane", () => {
    expect(framePadding(56, NO_INSETS, 1280, 800)).toEqual({
      top: 56,
      right: 56,
      bottom: 56,
      left: 56,
    });
  });

  it("holds the camera out from under the panels", () => {
    const insets: Insets = { ...NO_INSETS, left: 424, bottom: 180 };

    expect(framePadding(56, insets, 1280, 800)).toEqual({
      top: 56,
      right: 56,
      bottom: 236,
      left: 480,
    });
  });

  /*
   * A panel can cover most of the pane. Fitting a route into the slot left over
   * would answer by flying to the far end of the zoom range, so past this share
   * the route is better shown partly covered than not shown at all.
   */
  it("refuses to give the panels more than most of the frame", () => {
    const padding = framePadding(56, { ...NO_INSETS, bottom: 700 }, 1280, 800);

    expect(padding.top + padding.bottom).toBeCloseTo(480);
    // Scaled together, so the framing keeps the bias the panels gave it.
    expect(padding.bottom / padding.top).toBeCloseTo(756 / 56);
  });

  // Before the first layout there is nothing to hold the padding against, and a
  // share of no frame is no padding at all.
  it("passes the padding through for a pane that has no size yet", () => {
    expect(framePadding(56, NO_INSETS, 0, 0)).toEqual({
      top: 56,
      right: 56,
      bottom: 56,
      left: 56,
    });
  });
});
