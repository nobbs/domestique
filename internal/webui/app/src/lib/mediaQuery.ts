/**
 * Reading a CSS media query from React.
 *
 * Most of what the page does about a display preference it does in CSS, at the
 * queries in `index.css`. A few things cannot: the basemap is a style document
 * fetched over the network, the map's camera is animated by MapLibre in
 * JavaScript, and the elevation chart's height is a coordinate system rather
 * than a rule. All of them have to ask the same questions a stylesheet asks, so
 * the asking lives here rather than several times over.
 */

import { useCallback, useSyncExternalStore } from "react";

const REDUCED_MOTION = "(prefers-reduced-motion: reduce)";

/**
 * The one breakpoint, and the same value the `@media` block in `index.css` uses.
 *
 * There is exactly one, at 832 px: above it the panels float beside the map,
 * below it they dock to the bottom of it. Both copies must stay in step.
 */
const NARROW = "(max-width: 52rem)";

/**
 * A pointer that cannot hover and cannot be put down precisely: a finger.
 *
 * The chart offers the same two gestures to both kinds of pointer, but not in
 * the same way — a mouse arms a drag by pressing, a finger by holding — so the
 * hint above the plot has to say which of the two it is talking to.
 */
const COARSE_POINTER = "(pointer: coarse)";

/**
 * Whether a media query matches, re-rendering when that changes.
 *
 * `useSyncExternalStore` rather than an effect that seeds state: an effect runs
 * after the first paint, so the first render would always be the non-matching
 * answer and correct itself visibly.
 *
 * A fresh `MediaQueryList` per subscription rather than one held at module
 * scope: matching is stateless, the browser deduplicates the underlying
 * listener, and a module-level query would evaluate at import time, before any
 * test has had the chance to stub `matchMedia`.
 */
export function useMediaQuery(query: string): boolean {
  const subscribe = useCallback(
    (onChange: () => void) => {
      const list = window.matchMedia(query);
      list.addEventListener("change", onChange);

      return () => {
        list.removeEventListener("change", onChange);
      };
    },
    [query],
  );
  const matches = useCallback(() => window.matchMedia(query).matches, [query]);

  return useSyncExternalStore(subscribe, matches);
}

/**
 * Whether the reader has asked for less movement.
 *
 * The CSS side of this preference is the `prefers-reduced-motion` block in
 * `index.css`; this is the half of it a stylesheet cannot reach.
 */
export function usePrefersReducedMotion(): boolean {
  return useMediaQuery(REDUCED_MOTION);
}

/**
 * Whether the page is at its narrow layout.
 *
 * The layout itself is CSS. This is for the one thing a stylesheet cannot set:
 * the elevation chart is an SVG whose height is also its coordinate system, so
 * the shorter phone plot has to be a number rather than a rule.
 */
export function useNarrowViewport(): boolean {
  return useMediaQuery(NARROW);
}

/**
 * Whether the reader is pointing with a finger rather than with a mouse.
 *
 * Only for what is said, never for what is done: the chart decides how to treat
 * a gesture from the pointer that actually made it, because a laptop with a
 * touchscreen answers this yes and is still driven by its trackpad most of the
 * time.
 */
export function useCoarsePointer(): boolean {
  return useMediaQuery(COARSE_POINTER);
}
