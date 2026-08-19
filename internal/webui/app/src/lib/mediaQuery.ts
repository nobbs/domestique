/**
 * Reading a CSS media query from React.
 *
 * Most of what the page does about a display preference it does in CSS, at the
 * queries in `index.css`. Two things cannot: the basemap is a style document
 * fetched over the network, and the map's camera is animated by MapLibre in
 * JavaScript. Both have to ask the same questions a stylesheet asks, so the
 * asking lives here rather than twice over in the modules that need it.
 */

import { useCallback, useSyncExternalStore } from "react";

const REDUCED_MOTION = "(prefers-reduced-motion: reduce)";

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
