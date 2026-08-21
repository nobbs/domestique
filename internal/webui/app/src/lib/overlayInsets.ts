/**
 * How much of the map is under a panel.
 *
 * The panels float over the map rather than beside it, so the camera and the
 * furniture disagree about what "the map" is: framing a route against the whole
 * pane puts the first kilometres of it under the route panel and the last of it
 * under the elevation chart. What the reader can actually see is the pane minus
 * whatever is standing on it, and that is what the camera should frame.
 *
 * The panels are measured rather than assumed. Their sizes come from their
 * content — a search column grows with its results, the chart is a pill or a
 * panel depending on what was asked of it — so a table of widths kept here
 * would be a second, quietly wrong copy of the stylesheet.
 */

import { useEffect, useState } from "react";
import { useNarrowViewport } from "./mediaQuery";

/** Pixels of the map's own frame that are not visible, on each side. */
export interface Insets {
  top: number;
  right: number;
  bottom: number;
  left: number;
}

export const NO_INSETS: Insets = { top: 0, right: 0, bottom: 0, left: 0 };

/** The part of `DOMRect` this reads, so a test can hand over a plain object. */
export interface Box {
  top: number;
  right: number;
  bottom: number;
  left: number;
  width: number;
  height: number;
}

/** Where the panels live, and the one class the shell promises to put them in. */
const OVERLAY_SELECTOR = ".shell__overlay";

/**
 * Never more of the frame than this may be given away to the panels over it.
 *
 * A panel can cover most of the pane — the search column on a phone reaches the
 * top of the screen — and a camera asked to fit a route into the slot left over
 * would answer by flying to the far end of the zoom range. Past this share the
 * route is better shown partly covered than not shown at all.
 */
const MOST_OF_THE_FRAME = 0.6;

/**
 * The smallest box worth holding the camera out of, in pixels.
 *
 * The overlay's children are not all panels: the page's own heading is in there
 * too, a strip one pixel high that only a screen reader ever meets. Nothing
 * that thin is standing on the map.
 */
const LEAST_PANEL = 4;

/**
 * Which side each panel takes, and how far in it reaches.
 *
 * A panel is assigned to the edge it eats the least of, which is the edge it is
 * docked against: a column down the left takes its own width from the left
 * rather than its own height from the top, and the chart across the foot takes
 * its height from the bottom rather than most of the pane from the right.
 */
export function insetsFrom(frame: Box, panels: Box[]): Insets {
  const insets = { ...NO_INSETS };
  // Largest area first, so the column is read before the wordmark standing in
  // it: a panel already behind another panel's strip is asking for nothing the
  // map has not given up already.
  const standing = [...panels].sort(
    (one, other) => other.width * other.height - one.width * one.height,
  );

  for (const panel of standing) {
    if (panel.width < LEAST_PANEL || panel.height < LEAST_PANEL || behind(frame, insets, panel)) {
      continue;
    }
    const reach = {
      top: panel.bottom - frame.top,
      right: frame.right - panel.left,
      bottom: frame.bottom - panel.top,
      left: panel.right - frame.left,
    };
    const side = (Object.keys(reach) as Array<keyof Insets>).reduce((least, next) =>
      reach[next] < reach[least] ? next : least,
    );
    insets[side] = Math.max(insets[side], reach[side]);
  }

  return insets;
}

/** Whether a panel falls entirely within ground the map has already given up. */
function behind(frame: Box, insets: Insets, panel: Box): boolean {
  return (
    panel.right <= frame.left + insets.left ||
    panel.left >= frame.right - insets.right ||
    panel.bottom <= frame.top + insets.top ||
    panel.top >= frame.bottom - insets.bottom
  );
}

/**
 * The room to leave around the bounds, once the panels have had their share.
 *
 * `width` and `height` are the pane's, and nought for a pane that has not been
 * laid out: there is nothing to hold the padding against then, so it is passed
 * through as asked rather than scaled against a frame of no size.
 */
export function framePadding(
  gutter: number,
  insets: Insets,
  width: number,
  height: number,
): Insets {
  const [left, right] = held(gutter + insets.left, gutter + insets.right, width);
  const [top, bottom] = held(gutter + insets.top, gutter + insets.bottom, height);

  return { top, right, bottom, left };
}

/**
 * One axis of `framePadding`, scaled down together when the pair asks for more
 * of the frame than it may have. Both sides shrink in proportion, so the
 * framing keeps the bias the panels gave it: a route beside a left column stays
 * right of centre rather than being re-centred over it.
 */
function held(start: number, end: number, extent: number): [number, number] {
  const asked = start + end;
  const allowed = extent * MOST_OF_THE_FRAME;
  if (extent <= 0 || asked <= allowed) {
    return [start, end];
  }
  const scale = allowed / asked;

  return [start * scale, end * scale];
}

/** Whether two measurements say the same thing, so a render can be skipped. */
function same(one: Insets, other: Insets): boolean {
  return (
    one.top === other.top &&
    one.right === other.right &&
    one.bottom === other.bottom &&
    one.left === other.left
  );
}

/**
 * The insets of the shell's overlay, kept current as its panels come and go.
 *
 * The overlay covers exactly the pane the map is drawn in, so its own box is
 * the frame and its children are what stands on it. Both are watched: a panel
 * changing size is a resize, and a panel opening or closing is a mutation, and
 * either changes what the reader can see of the map.
 */
export function useOverlayInsets(): Insets {
  const [insets, setInsets] = useState<Insets>(NO_INSETS);
  /*
   * Only at the wide layout. Below the breakpoint the panels dock across the
   * foot of the pane and take most of it — which is the design, not an
   * accident — and holding the camera out from under a column that grows as the
   * reader types would fly it on every keystroke.
   */
  const narrow = useNarrowViewport();

  useEffect(() => {
    if (narrow) {
      setInsets(NO_INSETS);

      return;
    }
    const overlay = document.querySelector<HTMLElement>(OVERLAY_SELECTOR);
    if (
      !overlay ||
      typeof ResizeObserver === "undefined" ||
      typeof MutationObserver === "undefined"
    ) {
      return;
    }
    const panels = () => Array.from(overlay.children);
    const measure = () => {
      const next = insetsFrom(
        overlay.getBoundingClientRect(),
        panels().map((panel) => panel.getBoundingClientRect()),
      );
      setInsets((previous) => (same(previous, next) ? previous : next));
    };

    const sizes = new ResizeObserver(measure);
    const watch = () => {
      sizes.disconnect();
      sizes.observe(overlay);
      for (const panel of panels()) {
        sizes.observe(panel);
      }
    };
    const arrivals = new MutationObserver(() => {
      watch();
      measure();
    });

    watch();
    measure();
    arrivals.observe(overlay, { childList: true });

    return () => {
      sizes.disconnect();
      arrivals.disconnect();
    };
  }, [narrow]);

  return insets;
}
