/**
 * Escape, from anywhere on the page.
 *
 * The gestures that put a view into a state — a drag across the chart, a drag
 * along the route — are made with a pointer and leave focus wherever they began,
 * which is usually nowhere in particular. A way out that had to be focused first
 * would be a way out the reader has to go and find, so this listens on the
 * document and is registered only while there is something to leave.
 *
 * A key another handler has already dealt with is left alone: whoever called
 * `preventDefault` was closer to what the reader was doing.
 */

import { useEffect, useRef } from "react";

export function useEscapeKey(active: boolean, onEscape: () => void): void {
  // Held in a ref so an inline handler does not re-register the listener on
  // every render, and so the listener always calls the current one.
  const latest = useRef(onEscape);
  useEffect(() => {
    latest.current = onEscape;
  }, [onEscape]);

  useEffect(() => {
    if (!active) {
      return;
    }
    const onKeyDown = (event: KeyboardEvent) => {
      if (event.key === "Escape" && !event.defaultPrevented) {
        latest.current();
      }
    };
    document.addEventListener("keydown", onKeyDown);

    return () => document.removeEventListener("keydown", onKeyDown);
  }, [active]);
}
